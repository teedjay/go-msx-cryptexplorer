# CLAUDE.md: native Go port of a 16KB MSX1 cartridge

This project ports one MSX1 cartridge ROM to native Go. It follows the plan
distilled from four earlier ports (three 16KB MSX1 cartridges and one 32KB,
all fully ported and playable); the plan is reproduced below, adapted to this
ROM. Every rule in it exists because something went wrong without it, or
because it is the thing that actually caught the bug. The scars are kept on
purpose.

**This ROM shares its framework with the fourth port** (`../go-msx-warriorquest`,
same publisher, a year later): the same handler shape, busy flag, VDP
register table, input routine, text-stream writer and inline-table
trampoline, at different addresses. The generic tooling was copied from
there and retargeted. Reuse that port's solutions as *leads*, never as
facts: read this ROM's bytes before porting anything.

---

## This ROM

### Identity

| | |
| --- | --- |
| File the user supplies | `King's Valley 1 (1985) Konami [RC-727] [8553].rom` (keep it in the repo root, git-ignored) |
| Size | 16,384 bytes (`$4000`), plain cartridge, no mapper, no SCC |
| CRC32 | `b1b3543e` |
| MD5 | `c5698102c7e452b03fc7b9bb80a3768a` |
| SHA-1 | `dda0d1bf4da8b159d08add8b7f4dc75da3e6e242` |
| SHA-256 | `01bd3179540c886ab658626ca0460c329d167327fabd43eb615883375e9e9a3e` |
| Region / refresh | Japanese release, 60Hz: use the `C-BIOS_MSX1_JP` machine |

Every tool that opens the ROM must check the size and SHA-256 and warn on a
mismatch (`internal/rom` does). Every address in these notes is meaningless
against another dump.

### Naming

- Repo / module path: `go-msx-cryptexplorer` (local-only, bare repo name).
  It is generic on purpose; keep it that way.
- Do not put the original title or publisher in the module path, package
  names, window title, README headings or commit subjects. The ROM filename
  above (which the user must look for) and the tracing notes are the only
  places they appear.

### Memory map

- The cartridge occupies **`$4000-$7FFF`** (page 1). File offset = CPU
  address − `$4000`. Header: INIT = `$406C`; STATEMENT, DEVICE, TEXT all
  `$0000`.
- The ROM has **no `$FF` fill at the end**: the last bytes are data. Do not
  assume any region is padding until a reference proves it.
- Game RAM: INIT clears **`$E000-$E6FF`** (LDIR of `$06FF` bytes after
  seeding `$E000`) and puts the stack at **`$E700`**. The listing already
  references `$E700`, `$E701`, `$E760`, `$E761` above the stack, so the
  game uses RAM INIT does not clear (fourth-port lesson). Traces record
  `$E000-$EFFF`; seed the port's uncleared RAM from the trace's tick 0 and
  exclude the stack's reach (top of `$E6xx`; measure it) from the
  comparator. Treat any access outside `$E000-$EFFF` as a surprise: `wr`
  panics on it until a trace shows the original does it.
- `romByte(addr)` serves `$4000-$7FFF` from Raw manifest entries only, and
  `wr` into `$4000-$7FFF` is a no-op, as on hardware.

### Boot, interrupt and tick (read from the bytes; Medium until traced)

- INIT `$406C`: `DI / IM 1`, installs **`H.KEYI`** (`$FD9A`) = `JP $401A`
  (the hook runs before the BIOS reads the VDP status, so the game
  acknowledges VBlank itself), `LD SP,$E700`, clears RAM, sets `$E005`=1,
  calls the VDP/PSG set-up at `$459B`, clears `$E005`, `RDVDP` at `$4096`,
  **`EI` at `$4099`**, and parks in **`JR $` at `$409A`**. The whole game
  runs inside the interrupt.
- Handler `$401A`: `CALL RDVDP`, `DI`, `CALL $7B36` (**per-interrupt work**,
  the sound driver: it walks three 14-byte channel blocks at `$E010`,
  `$E01E`, `$E02C` and calls `WRTPSG`; runs before the busy check, so nested
  interrupts run it too). Busy flag **`$E005` bit 0** at `$4021`; if set,
  skip to `$4034`. Otherwise `INC ($E005)`, `EI`, **tick body starts at
  `$402A`**: `CALL $45C8` (input), `CALL $40A3` (state dispatch); then
  `XOR A / LD ($E005),A`.
- Exit path `$4034`: `CALL RDVDP / OR A / DI / CALL M,$7B36 / EI / RET`: a
  VBlank pending on the way out runs the sound work inline. Model it (the
  fourth port saw it once in 59,000 ticks); the comparator must refuse
  `$7B36` return addresses it does not model (`$4021` handler, `$403C` exit
  path).
- `scripts/refdump.tcl`'s tick-start breakpoint goes on **`$402A`**. The
  per-interrupt work is `$7B36`; count its entries between tick starts.
- INIT's BIOS calls (`WRTPSG`, VRAM fill through `$409C` → `$44AB`) may
  re-enable interrupts: record INIT-time interrupts in the trace header and
  have the port's boot run them where they happened.

### State dispatch (`$40A3`, High from the bytes)

`INC ($E003)` (frame counter), `BC = ($E000)` (C = state, B = sub-state),
and unless `$E002` bit 6 is set, `PUSH $4644` as a continuation (so every
state handler's `RET` lands in `$4644`, not back in `$40A3`). Then
`JUMP_A` over 11 states (`$40D0 … $429C`). Sub-states are switched by
`DJNZ` chains on B (an inefficient switch on the current value, not a
countdown).

### VRAM layout (register table at `$45C0`, written by `$45AF`)

| Reg | Value | Meaning |
| --- | --- | --- |
| R0 | `$02` | Screen 2 (Graphic 2) |
| R1 | `$E2` | 16K, display on, VBlank IRQ on, 16x16 sprites, unmagnified |
| R2 | `$0E` | name table `$3800` |
| R3 | `$7F` | colour table `$0000` |
| R4 | `$07` | pattern table `$2000` |
| R5 | `$76` | sprite attribute table `$3B00` |
| R6 | `$03` | sprite pattern table `$1800` |
| R7 | `$E4` | backdrop colour 4 |

The same non-default layout as every earlier port. Confirm it by rendering
a trace before trusting it, and watch for later `WRTVDP` callers.

### Other leads (byte search; verify in the listing)

- **No `LD A,R`** anywhere: no entropy source from the refresh register.
  Any randomness is the game's own RNG in RAM, which the comparator checks
  like any other byte.
- No direct `IN`/`OUT`: VRAM goes through the BIOS (`WRTVRM`, `RDVRM`,
  `SETWRT`, `WRTVDP`), the PSG through `WRTPSG`/`RDPSG`.
- Input `$45E8` (same as the fourth port's): PSG R15=`$8F`, joystick A via
  R14, keyboard rows 7 (SELECT) and 8 (cursor keys, SPACE) through
  `SNSMAT` into one byte (bit 0 up, 1 down, 2 left, 3 right, 4 trigger A /
  SPACE, 5 trigger B / SELECT). `$E009` held, `$E008` newly pressed.
  `SNSMAT` is also called at `$4B5F` and `$4C19`: find out why.
- Demo input: when `$E000` = 5 and `$E002` bit 6 is clear, input comes from
  `$4621`, which reads (value, duration) pairs from the pointer at `$E080`
  (set to `$4ACE` by state 2), ending at `$FF`.
- `JR $` also at `$4A09`: read it (hang trap or data).
- `$409C`: `NOP x4 / JP $44AB` and `$403E`: `LD HL,$0000 / LD A,($404C)
  / NOP …/ JP $45E8` look like development leftovers patched over, as in
  the fourth port. Port what executes, not what the leftovers suggest.

### Where things are

- `docs/analysis.md`: the analysis doc (open issues first, checksums,
  boot, state machine, VRAM map, assets, verification log, corrections).
- `docs/entries.txt`: what the disassembler starts from (addresses and
  names only). `JUMP_A` at `$404B` is the inline-table trampoline.
- `docs/listing.asm`: generated by `go run ./cmd/disasm -o docs/listing.asm`,
  git-ignored.
- Tools: see the table in Phase 1 below; the ones that exist are in `cmd/`.
- A fresh clone needs `go run ./cmd/extract` before `go test ./...` can
  exercise the assets.

### Environment

- Go 1.27 (`/opt/homebrew/bin/go`), openMSX (`/opt/homebrew/bin/openmsx`)
  with the bundled C-BIOS machines. macOS: no `timeout`, so run openMSX in
  the background with a watchdog loop (`scripts/refdump.sh`).
- Reference traces, VRAM dumps, audio recordings and the listing stay out
  of the repo (traces under the session scratchpad or `/tmp`).

---

# The plan

**The goal is a native Go port, not an emulator.** No Z80 code runs at
runtime. Each routine is read, understood, and rewritten as ordinary Go
against a small hardware-abstraction interface. That distinction drives
everything else: the legal position, the verification strategy, and what
"done" means for a routine.

**The second port's headline lesson: verify against the original tick by
tick, from the first ported routine.** An emulator (openMSX with the free
C-BIOS) used *offline* as a reference oracle records the original's state at
every game tick, and a comparator runs the port in lockstep against it. In
the second project this reached a byte-for-byte match over tens of thousands
of ticks of real play. It found bugs that static reading and screenshots
never would have, and it named each one's address. See Phase 1, "The
reference oracle", and Phase 5.

**The third port's headline lessons:** place nested interrupts where the
original took them (counted in VRAM writes), or long ticks will never
match; when the asset lookup panics, ask the oracle before "fixing"
anything, because four times out of four it was genuine original
behaviour; and verify every layer you ship, audio output included, against
the original, not just the layer the comparator covers. The third port
matched byte for byte over a 41,821-tick game through stage 7.

**The fourth port's headline lessons** (32KB, eight stages, eight bosses,
an ending): a match says the port reproduces the original, not that the
original played the game you meant, so read the original's own timeline
(stage, lives, state) in every trace; an entropy source (`LD A,R`) breaks
input found on the port, and the cure is the oracle in the loop (record,
cut before the first divergence in outcome, replay the original's own
values, search on); and the interrupt handler's own BIOS code is part of
the machine (it ran the sound work from an exit path without an
interrupt, and it restored a register the game read stale). The fourth
port matched byte for byte over a 188,307-tick game: all stages, every
boss, the ending and the second loop's first stage.

---

## Phase 0: repo hygiene and legal, before any code

Do this on day one. The first project retrofitted it at the end, and it cost
a full day of migration plus a careful audit, because by then ~3,000 bytes of
ROM data had accumulated inside committed Go source.

1. **`.gitignore` the ROM, the extracted assets and the generated
   disassembly in the very first commit:** `*.rom`, `assets/**/*.bin`,
   `docs/listing.asm`. All three are the rights holder's data, decompressed
   or decoded but not yours. Reference traces and VRAM dumps are ROM-derived
   too, so keep them outside the repo (for example `/tmp`). Screenshots of
   your own running engine are fine.
2. **No ROM bytes in committed Go source.** This is the rule that is easiest
   to break by accident and most expensive to undo. A data table longer than
   a few bytes goes through the extract tool (Phase 1) and is reached via
   `//go:embed`, not pasted in as `[217]byte{0x..., ...}`. Short constants
   (a 2-byte tile pair, a palette index, an 8-byte VDP register table) are
   fine; use judgement, but when in doubt extract it.
3. **Name the repo for what it is, not what it plays.** Use a generic,
   descriptive name. Do not put the original title or publisher in the
   repository name, the module path, the window title, or any heading.
   Reference them only where technically necessary: the ROM filename a user
   must actually look for, and the tracing notes.
4. **Add `NOTICE` and a README "Legal" section up front.** Say the project
   is unofficial and non-commercial, and not affiliated with or endorsed by
   the rights holder. Say what is and is not distributed, and that the
   engine executes no original code. Invite rights holders to open an issue.
   Add a `LICENSE` too if the README is going to call the project open
   source.
5. **Decide the module path now.** Renaming later is mechanical but touches
   every import. A local-only repo can simply use the bare repo name.

## Phase 1: the tooling spine

Build these before porting anything. Each one paid for itself many times
over. Roughly in this order:

| Tool | Why it has to exist |
| --- | --- |
| `internal/z80dis` + `cmd/disasm` | Recursive-descent disassembler with **byte-level coverage tracking**, so a misaligned decode can't quietly corrupt already-confirmed code. Needs special handling for computed-jump trampolines; find them early. Give it `-gaps` (unclaimed ranges *with the instructions that reference into each*) and `-peek A-B` (linear decode without claiming). |
| `cmd/extract` | Rebuilds every asset from the ROM via a **manifest** (address, length, and method: raw range, compressed stream, or derived). This is the thing that lets you ship no ROM data. Give it a `-check` mode that re-derives everything and diffs without writing. |
| `internal/<compression>` + decoder cmd | The graphics may well be compressed. Decode to flat buffers for extraction. **Check each block**: in the second project the font was stored raw while everything around it was RLE. |
| `cmd/gfxpreview`, `cmd/vramview` | Render byte ranges or raw VRAM dumps as tile and sprite sheets, through the shared renderer. |
| `internal/msxrender` + `cmd/screenshot` | **The most valuable visual tool.** See "One renderer" below. |
| `scripts/refdump.tcl` + `cmd/refcompare` | **The most valuable tool overall.** See "The reference oracle" below. Build it as soon as the boot path is ported. |
| `cmd/sndpreview` | Once you reach audio: render a sound id to WAV through the ported sequencer and a PSG model, so you can check sounds without a window. Give it a `-trace` mode that renders the game's audio over a reference trace's interrupts, for comparison with the original's (Phase 5, "Audio"). |
| `cmd/disasm -reach A,B,...` | Lists every routine reachable from a set of roots, with the instruction count. Use it to scope a porting step ("boot through title: 40 routines, 779 instructions") and to check nothing is left. |
| `cmd/keygen`, `cmd/keysearch`, `cmd/tracecov` | Scripted-input schedules, a search that finds good play on the port, and a coverage report over a set of traces. See Phase 5, "Coverage". |
| `cmd/audiocmp` + `scripts/refaudio.tcl` | The original's audio recorded by openMSX, compared window by window with the port's. See Phase 5, "Audio". |

### Disassembler lessons from the second and third projects

- **Grow inline jump tables one entry at a time.** For a `CALL trampoline`
  followed by `DW` entries, add a single entry, run the full descent to
  completion, then try the next. Code discovered from the earlier entries
  then stops the table from overrunning into it. Report such tables as
  "heuristic" until you confirm the length from tight packing (the table
  ends exactly at its first target) or from the index range.
- **Treat `LD rr,nn` / `PUSH rr` as a code reference.** The pushed value is a
  continuation that a later `RET` jumps to. Both top-level dispatchers in the
  second ROM pushed a continuation before dispatching, so **every handler's
  `RET` landed in a third routine, not back at the caller**. Missing this
  would have left two whole routines unported. It is another disguise of the
  routine-boundary trap in Phase 4.
- **Callbacks passed in a register** (`LD DE,routine` then a helper doing
  `EX DE,HL / JP (HL)`) are invisible to call-following. Search for the
  helper, list the values loaded into DE before it, and add them as entries.
- **Format immediates and call targets separately.** `LD DE,$0000` printed
  as `LD DE,CHKRAM` (a BIOS label) misleads. Only call and jump targets get
  BIOS names; memory operands get system-variable names.
- A "data reference" in the gap report may be a VRAM address the VDP masks
  (`LD DE,$7800` meant `$3800`) feeding a fall-through into a fill routine.
  Read the code around a surprising reference before classifying the target.
- *(Third project)* **The `PUSH` continuation rule has false positives.**
  `LD HL,table / PUSH HL / ... / POP HL` pushes a data pointer, and
  following it as code decoded a run of graphics as instructions until it
  collided with real code. Decide by simulating the stack forward from the
  `PUSH` (count pushes and pops, follow unconditional jumps): popped before
  any `RET` or `JP (rr)`, it is data; still on the stack there, it is a
  continuation; anything else (`EX (SP),HL`, `LD SP`), report it for a
  human.
- *(Third project)* **A `JP (HL)` whose target is `table + A` may jump into
  code, not through pointers.** `$49B5` jumped to `$49B6 + 0/4/8/12`: four
  4-byte stubs (`LD A,n / JR common`) that the gap report showed as 19
  bytes of data. Peek every gap next to a `JP (HL)`.
- **Check "dead" code is dead.** Unreferenced code (no `CALL`/`JP`) may
  still be reached through a pointer. Search the whole ROM for its address
  as a 16-bit word before calling it dead; the third project had three
  genuinely dead routines, all development leftovers.

### One renderer, shared, with a headless way to use it

Ebitengine (or any windowed toolkit) may not run in a sandboxed shell, which
means you cannot see what you just built. The trap is to write a throwaway
compositor inside a test to inspect VRAM. **Do not.** That copy silently
drifts from what the game actually renders, and then you end up debugging a
rendering difference that does not exist in the real program.

Instead:

- Put the compositor in its own package (`internal/msxrender`): VRAM in,
  `image.RGBA` out.
- The game draws every frame through it.
- A headless `cmd/screenshot` drives a real console instance (attract
  sequence → menu → into a stage, holding the accelerator) and writes PNGs
  through *the same* package.

A still then cannot show something the running game would not. This also
gives you a regression test in Phase 5.

### The reference oracle (lockstep traces)

The original runs in openMSX purely as an offline reference. The port never
executes Z80 code. Set it up as soon as the boot path is ported:

- **Record at the start of every game tick.** For a VBlank-driven game, put
  a breakpoint on the first instruction of the tick body (after any
  re-entrancy guard). Dump game RAM, all 16KB of VRAM, VDP registers R0-R7,
  and the interrupt count so far. Use the Tcl `debug read_block` and
  `debug read "VDP regs"` commands. A breakpoint that has a command does not
  pause emulation.
- **Compare in lockstep.** `cmd/refcompare` boots the port and, for each
  recorded tick, runs the per-interrupt work (sound) as many times as the
  reference took interrupts. It compares RAM, VRAM and registers, runs the
  tick, and stops at the **first** differing byte, with ref/port PNGs of
  that tick. The first differing address almost always points at the
  routine that owns it. Exclude the Z80 stack area of RAM.
- **Prove the comparator can fail.** Flip one byte in a copy of a trace and
  check that it is reported at the right tick and address. A comparator that
  always passes is worse than none.
- **Feed input by tick.** The trace script takes a schedule
  (`tick:row:mask:down|up` for `keymatrixdown/up`), applied at the tick-start
  breakpoint, before the game reads input. The comparator replays the same
  schedule into the port. Use the keyboard modes for traced sessions: the
  key matrix is easy to drive and the joystick is not. Give the playable
  frontend a `-record` flag that writes this format, so a human's session
  can be replayed into the original and diffed.
- **Script games for coverage.** A long pseudo-random input schedule
  started from the menu covered most player states in the second project.
  Measure coverage from the traces (which state values occurred), and list
  what has not been exercised as an open issue. Rare paths (stage clear,
  game over, 2 players) need a human.
- **When a byte differs and you cannot see why, ask the oracle who wrote
  it.** `debug set_watchpoint write_mem <addr>` with a command that logs
  `[reg PC]` and `$::wp_last_value`. In the second project, `$E130` differed
  after stage setup. The watchpoint named an `RR (IX+0)` in a load-time
  sprite-mirroring loop that used RAM as a scratch buffer, and one byte of
  that buffer survived later initialisation. The port had only reproduced
  the loop's VRAM result. **Derived assets can have RAM side effects: port
  those too.**
- openMSX practicalities: C-BIOS machines ship with it (`C-BIOS_MSX1_JP` for
  60Hz). `screenshot` with `set throttle off` gave black images, so dump VRAM
  and render it with your own renderer instead, which is better anyway.
  macOS has no `timeout`, so run openMSX in the background with a watchdog
  loop. Several instances can record in parallel.
- *(Third project)* **Interrupts can fire during INIT.** C-BIOS routines
  (`WRTVDP`, `SETWRT` and the game's own VRAM fill) re-enable interrupts,
  so the handler ran its per-interrupt work twice before INIT finished.
  Write a header with the interrupt count at INIT's final `EI`, and have
  the port's boot run those interrupts where they can have happened.
- *(Third project)* **Place nested interrupts where the original took
  them, counted in VRAM writes.** Replaying a long tick's nested
  interrupts after the tick broke at the real-game start: a 14-frame load
  ended by starting a tune, so in the original the sound driver ran 13
  times on the old state, and in the port 13 times on the new one. VRAM
  write sequences run with interrupts disabled and re-enable them right
  after their last write, so "VRAM writes so far this tick" (a watchpoint
  on `write_io 0x98`) pins every nested interrupt between the right two
  writes. The port counts its own writes and runs each interrupt
  immediately after the write that reaches its count. This also
  reproduces, for free, the original's VDP-address corruption when an
  interrupt's sprite writes land inside an RLE stream that runs with
  interrupts enabled.
- *(Third project)* **openMSX Tcl traps that fail silently:** in a
  `read_mem` watchpoint command, `$::wp_last_value` is unset, so the
  command errors and logs nothing; instruction fetches also trigger
  `read_mem` watchpoints; and globals named `::keys` or `::t` broke a
  script without a word. Prefix every global. **Validate a probe on a
  known positive first**: a probe that reports nothing is not evidence.
  One "the original never reads this" conclusion was drawn from a broken
  probe and was wrong. Logging registers from breakpoints is the most
  reliable probe.
- *(Third project)* Make the comparator recover engine panics and report
  them as "tick N: panic", with the game package's stack frames.
- *(Fourth project)* **The handler has more than one way to run the
  per-interrupt work.** Its exit path re-read the VDP status and, if a
  VBlank had arrived while interrupts were off at the very end of the
  tick, ran the sound work inline (`CALL M,$49C0`): no interrupt taken,
  but a frame consumed. And a tick can *start* inside the previous tick's
  exit path (an interrupt during the BIOS `RDVDP` there), nested in that
  handler. Log the return address of every per-interrupt-work entry and
  the busy flag with every interrupt, and make the comparator refuse
  return addresses it does not model rather than guess. The first case
  appeared once in 59,000 ticks.
- *(Fourth project)* **Place interrupts by sound events as well as VRAM
  writes.** An interrupt after a tick's last VRAM write but before a later
  sound start cannot be placed by the write count. Count sound starts and
  the game's direct reads of the sound driver's RAM too, record both per
  interrupt, and poll before each.
- *(Fourth project)* **The BIOS saves and restores registers around the
  handler, and stale registers see that.** C-BIOS's interrupt handler
  pushes IY and pops it on the way out, so a tick started from the idle
  loop always saw IY = `$012C` (left by INIT's BIOS calls), whatever the
  previous tick did, while a tick started in the exit path kept the
  previous tick's IY. A boss routine stored through that stale IY: on the
  original into BIOS ROM (no effect), in the port into the boss itself.
  A breakpoint logging the register at tick start settled it in one run.
  Such values are BIOS-specific; say so in the notes.
- *(Fourth project)* **openMSX unthrottled is fast: 188,307 ticks recorded
  in 80 s.** Recording is cheap enough to iterate with (see Phase 5,
  "Coverage"). A full-state trace is about 20KB a tick (3.9GB for that
  game), so delete old traces as you go.
- *(Fourth project)* **A match is not the outcome you wanted.** A
  133,000-tick recording matched byte for byte, but in it the original
  had lost every life at a boss and gone through three game overs; the
  input search had planned a different game. After every recording,
  print the original's state timeline (state, stage, lives, boss phase)
  from the trace before claiming what it verified.

## Phase 2: map the ROM before porting

1. Generate the listing. Re-run `cmd/disasm` whenever you confirm new code
   addresses: they feed back into its jump-table heuristics and recover more
   of the ROM automatically.
2. Record the ROM's checksums (CRC32/MD5/SHA-1/SHA-256) and size in the
   analysis doc, and have every tool warn on a mismatch. Every address in
   your notes is meaningless against a different dump.
3. **Derive the VRAM layout from the ROM's own VDP register writes.** Do not
   assume the MSX BASIC default. Both projects used the same non-default
   layout (colour table at `$0000`, patterns at `$2000`, name table at
   `$3800`, sprites at `$1800`/`$3B00`). Assuming the textbook layout
   mislabelled every pattern/colour pair backwards in the first project:
   "pattern" files that were colour data and vice versa. Confirm the
   derivation by compositing one pattern+colour pair and checking you get
   readable text or a recognisable shape.
4. Find the top-level state machine and the per-frame tick early. Everything
   else hangs off them. Check whether the game runs entirely inside the
   VBlank interrupt (INIT ending in `JR $`) and whether it has a busy flag:
   then some ticks take several frames (see Phase 5, "Tick pacing").
5. **Spare VRAM is working storage.** The second ROM kept its stage map in
   VRAM above the sprite attribute table and read it back through BIOS
   `RDVRM`. Do not assume VRAM holds only display data.
6. *(Fourth project)* **Measure the game's RAM; INIT's clear is not its
   extent.** INIT cleared `$E000-$E7FF` and put the stack at `$E800`, but
   play used `$E800-$EFFF` above the stack, which holds whatever the
   machine left there (under C-BIOS/openMSX a fixed pattern). Record the
   whole page from the first trace, seed the port with the power-on
   contents, and widen the trace format before you need it rather than
   after.
7. *(Fourth project)* **A 32KB cartridge changes little.** INIT maps page
   2 itself (`RSLREG`, `ENASLT`), which ports to a documented no-op; code
   and data cross `$8000` freely. Do not split anything by page. Expect
   about twice the tables, triplers and stream directives, and more
   manifest gaps on the first comparator runs.

## Phase 3: assets

- **Check whether a block is compressed before extracting it.** Grep the
  listing for a `LD DE,<addr>` (or `LD HL,<addr>`) feeding a call into the
  decompressor. A raw render that "looks roughly like tiles" is not
  confirmation; that is exactly how a font shipped with subtly corrupted
  glyphs.
- **Render the entire decoded output and check it is a complete, coherent
  set.** Not a spot check. "It terminates cleanly" and "most tiles look
  fine" were both satisfied by a decoder with its two run kinds swapped.
- **Tight packing is the strongest cheap signal**: a correctly sized
  extraction ends exactly where the next known thing begins. In the second
  project every stream ended exactly at the next stream or at code.
- **Verify provenance empirically; never trust a filename.** In the first
  project two assets were *named* for a ROM address that really is a valid
  compressed stream. Decoding from it succeeds and yields plausible,
  completely wrong data. They were actually load-time mirrors of slices of
  other assets. Confirm each manifest entry by extracting it and diffing
  against the file you already have.
- Assets the original computes at load time (mirrored tiles, derived
  colours) belong in the manifest as derived entries, computed from their
  source asset, not as separate captured blobs.
- **Store multi-segment streams as flat segments, with the VRAM addresses in
  the manifest.** Have the extractor fail if a decode does not produce
  exactly the declared segments. The manifest then doubles as a checked
  record.
- **Register every data gap the code references as a Raw entry**, even small
  tables, and give the engine one `romByte(addr)` lookup that serves any
  address inside a Raw entry and **panics on anything else**. Routines that
  walk pointer tables can then be ported with their original address
  arithmetic. A read of something never extracted fails loudly instead of
  returning a plausible zero. In the second project that panic is what
  turned a silent run-off-the-map bug into an obvious crash with an address.
- *(Third project)* **When `romByte` panics, ask the oracle before you
  change anything.** It happened four times, and each was genuine
  original behaviour: the demo input reader ran past its table into two
  code bytes; a collision scan indexed a 5-entry table with any RAM byte
  (through a pointer bug), reading code up to 510 bytes on; after a game
  over the demo reader read the BIOS at `$0000`; and the last sound's stop
  byte was the first byte of the `$FF` fill. Confirm each with a
  breakpoint that logs the pointer, then extend the manifest with a
  documented entry. When the index can be any byte, declare the whole
  reachable range at once rather than one panic at a time.
- *(Third project)* **Streams share tails and are reused mirrored.** One
  stream is decoded both normally and bit-reversed to a second
  destination, and several entry points start inside another stream and
  decode its tail. Tight packing still holds; record each entry point.
- Overlaps are allowed when the original does them. The second ROM copied
  `$180` bytes of font into VRAM, the last `$28` of which were its text
  table. The asset keeps the full length; the text is a separate entry.
- Runtime-decoded data stays runtime-decoded. Object shapes whose rows were
  RLE streams read at draw time are ported as the decoder routine reading
  raw ROM data, not pre-decoded.

## Phase 4: port the engine

Work outward from the state machine, one routine at a time, running the
lockstep comparison as you go.

### Conventions (keep these strict)

- Every ported function's doc comment names the Z80 address range it came
  from (`// Ported from $4685-$46A8`), so anyone can jump back to the
  listing and check the translation.
- **Say what is confirmed vs inferred, inline.** Use a three-level key: High
  (traced instruction by instruction, and/or visually or trace confirmed),
  Medium (control flow clear, meaning inferred), Low (plausible guess). A
  confident-sounding function name must not imply more certainty than the
  tracing supports. When the role is unclear, name the function by its
  address (`upd6B53`) and put the guess in the comment.
- **Prefer an honest stub with `// TODO: not yet ported, see $XXXX`, which
  sets a `Console.Unported` marker, over a guessed implementation.** The
  comparator then reports "ticks 0-N match, tick N reached unported $XXXX".
  That is a clean progress measure.
- Address unconfirmed RAM the way the original did (`c.RAM[0x0NN]` for
  `$E0NN`). Promote it to a named accessor only once the byte's role is
  confirmed, and note what confirmed it.
- **Keep CPU-address helpers** (`rd`, `wr`, `rd16`, `ldir`, `lddr`, `inc`,
  `dec` on 16-bit addresses) so pointer-walking routines translate line for
  line. `wr` to the cartridge range is a no-op, as on hardware. The second
  ROM wrote to its own ROM in three places, all development leftovers with
  no effect, and the port must not "fix" them. `wr` to an unmapped address
  panics.
- Keep the analysis doc append-only for corrections: add a correction note
  rather than editing history away. The record of *how* something was got
  wrong is worth as much as the right answer.

### The one failure mode that will cost you the most

**Assuming where a routine starts and ends, instead of reading it.**

This single mistake, in many disguises, caused nearly every real bug in the
first port, and kept turning up in the second:

- A shared helper was a *tripler*: it wrote three copies, 2KB apart, for the
  three Screen 2 thirds. It was read as a single write because the call site
  looked like one.
- A terminator byte in a data stream was really a *gap marker*; the stream
  continued past it.
- A "row repeat" copier actually advanced its source pointer each row, so
  rows walked forward through ROM.
- `JP L70E5` landed in the *middle* of another routine, not at the start of
  the one whose `CALL` sat at that address. Control continued through three
  more calls that the port never ran.
- A scoring routine looked like it ended at its `RET`; the no-carry branch
  three instructions earlier fell through into the high-score update, which
  therefore never ran at all.
- A state handler was read starting three instructions too late, dropping
  the two `CALL`s that cleared the screen.
- *(Second project)* Dispatchers `PUSH`ed a continuation before jumping
  through a table, so each handler's `RET` went somewhere else entirely.
- *(Second project)* A screen-clear routine set up its arguments and fell
  through into the fill routine below it, with no `CALL` or `JP` to show it.
- *(Fourth project)* "Kill all enemies" ended in `POP IX / JR $6974`,
  falling into the enemy-bullet clear; an item timer's last `CALL` was
  followed by a fall-through that printed the timer once more. The
  comparator named both within seconds; static reading had missed them.

The rules that would have prevented them:

1. **Read from the actual branch target, not from the nearest label.** When
   something jumps to `$411A`, start reading at `$411A`.
2. **Follow every fall-through.** A `RET` is not the end of a routine if a
   conditional branch above it skips past to more code. Trace each exit, and
   check whether the routine's last instruction is a jump at all.
3. **Read the callee before modelling a call.** Never infer what a shared
   routine does from how one call site uses it.
4. **For loops, the jump target tells you the semantics.** In the first ROM
   the two compression run-kinds are distinguished *only* by whether `DJNZ`
   targets the byte-read or the instruction after it. One re-emits without
   advancing (repeat), the other advances (literal). The control-byte ranges
   alone look symmetric and will mislead you.
5. **Track the stack.** Any `PUSH` of a constant, `POP` of a return address
   or `EX (SP),HL` changes where control goes.
6. When a bug makes no sense, re-read the raw bytes of the routine rather
   than re-reading your Go, or ask the oracle (a watchpoint) who wrote the
   byte.
7. *(Third project)* **Pass the stale register explicitly.** Helpers that
   `SET`/`RES` bits in `(HL)` were called with whatever `HL` the caller
   last loaded: four different addresses across call sites. Give such
   helpers an `hl` parameter rather than a fixed address. Likewise a
   routine whose exit path decides an `HL` that its caller then writes
   through must return it.
8. *(Third project)* **Review your translation against the listing before
   compiling.** One pass over freshly written Go found three bugs:
   `LD (HL),0 / LDIR` ported as the LDIR first (the clear idiom needs the
   seed byte stored first), and an `RRA / JR NC` with its sense reversed.

### Z80 and MSX semantics that bit us

- **`DJNZ` decrements a register, not RAM.** A dispatch that loads a state
  byte into `B` each frame and `DJNZ`es on it is an *inefficient switch over
  the current value*, not a persistent countdown. Modelling it as a RAM
  countdown broke the main loop for a long time.
- **8-bit wrap is load-bearing.** `ADD A,A` wraps at 8 bits, and widening to
  `uint16` before doubling changes behaviour. Model the width the original
  used: `3*A` index arithmetic is 8-bit, `L+5` without carry into H is
  low-byte only, and `ADD HL,BC` does carry.
- **BCD via `DAA`**: scores are packed BCD. Model `DAA` exactly (the
  correction from the H and C flags and the result, for both add and
  subtract) rather than writing a "BCD add" that assumes valid operands.
  `binary → BCD` loops (`ADD HL,HL` / `ADC A,A` / `DAA`) depend on it.
- **16-bit loads read two bytes** (`LD BC,(E000h)` fetches `$E000` *and*
  `$E001`), and overlapping 16-bit stores can touch three addresses (the
  second ROM set a hi-score of 999999 with two overlapping stores).
- **Chained `SBC HL` carries a borrow.** Two `SBC HL,rr` in a row feed the
  first borrow into the second. In the second ROM that meant ending a jump
  *higher* than it started counted as a long fall.
- **Flags survive across instructions that don't touch them**, and `EX
  AF,AF'` can carry a comparison result through a whole loop. Follow which
  instruction last set the flag that a branch tests.
- **Stale registers.** Occasionally a branch tests a register the routine
  only sets on some paths (`BIT 2,E` where E was loaded only if the enemy
  moved). Do not invent a value silently. Pick the most plausible one, mark
  it Medium in the code, list it as an open issue, and let a trace that
  exercises that path decide.
- **TMS9918A specifics:** colour index 0 is transparent and shows the
  backdrop (not black). Sprite Y `$D0` terminates the sprite list for all
  following entries. The displayed sprite row is Y+1. A 16x16 sprite's
  pattern number has its low 2 bits ignored and the table is addressed in
  8-byte units, not 32. At most four sprites show on one line.
- **The same 32 bytes have multiple 16x16 layouts in one ROM.** Hardware
  sprites are four 8x8 quadrants in TL/BL/TR/BR order; "big font"
  background characters are TL/TR/BL/BR; neither is 16 interleaved rows.
  All three produce *something* plausible from the same bytes. Justify the
  layout from the destination address and which hardware table it targets,
  never from "it rendered as something".
- **Entropy sources (`LD A,R`) have no equivalent in a native port.** Read
  the value through a pluggable source: a real RNG (values 0-127, since R's
  bit 7 stays 0 unless the game does `LD R,A`) in normal play, and in
  lockstep runs the values the trace script logged at each `LD A,R` site.
  Have the comparator flag a read at a different site or tick, because that
  alone pinpoints a control-flow difference. Document the deviation rather
  than faking the source. *(Fourth project)* It also breaks every input
  schedule found on the port wherever the value matters: see Phase 5,
  "Coverage", for what worked.
- **Sound state is game state.** The game logic may read the sound driver's
  RAM (for example, waiting for a tune to end before changing state). Then
  the sequencer must be ported faithfully and early, long before you have
  audio output. Port it as register writes into a PSG register array, and
  add a separate chip model (tones, noise, mixer, volume table, envelope)
  later for output.
- *(Third project)* **`RRA` four times is a 9-bit rotate**, not a shift by
  four: the carry goes into bit 7 each time. It only mattered for unused
  bits there, but model it with an explicit carry.
- *(Third project)* **Confirm what a variable means by measuring it, not
  from one routine.** The speed variable was a period (lower is faster),
  so "up" accelerated. Its comment said the opposite. The comparator
  matched either way, but the input tools built on the wrong meaning
  held the brake in every scripted game, and none reached a stage clear
  until the meaning was measured.
- **The BIOS port-number bytes** at `$0006`/`$0007` may be used swapped
  relative to the documentation. It is harmless on real machines, where both
  are `$98`; just don't let it confuse your VDP reading.

### Go pitfalls when transliterating

- **Operator precedence differs from what Z80 thinking suggests.** In Go,
  `&^`, `<<`, `>>`, `&` and `*` bind tighter than `|`, `^`, `+` and `-`,
  and `|` and `+` share a level. `col|0x08&^0x80` is `col|(0x08&^0x80)`,
  and `pc>>1|pc<<7+0x88` only worked by luck. Parenthesise every mixed
  bitwise/arithmetic expression. A review pass caught one real bug of
  exactly this kind in the second project.
- Byte-typed arithmetic wraps like the Z80 does; compute in `byte`, not
  `int`, unless you are deliberately testing for a borrow.

## Phase 5: make it playable, then keep it that way

Getting it on screen early is what surfaces bugs; static analysis does not.

**Verification, run before every commit:**

```bash
go build ./... && go vet ./... && go test ./...
go run ./cmd/extract -check                       # every asset still matches the ROM
go run ./cmd/refcompare -trace /tmp/attract.trace # port matches the original, tick by tick
go run ./cmd/refcompare -trace /tmp/play.trace -keys /tmp/play.keys
go run ./cmd/screenshot -out /tmp/shots && \
  for f in screenshots/*.png; do cmp "$f" "/tmp/shots/$(basename "$f")"; done
```

- Run `-check` after touching the manifest, the assets package, or how any
  asset is produced. A wrong address fails loudly instead of shipping
  plausible, wrong data.
- **Run `refcompare` after every engine change.** Keep at least one attract
  trace (no input) and a few scripted-input game traces. They must keep
  matching through every ported state. This is the strongest test you will
  have: it checks every byte of RAM and VRAM on every tick.
- Run the screenshot byte-diff after any engine change *not* meant to alter
  behaviour (a refactor, moving data into assets, a rename). It is the
  cheap smoke test when no trace is at hand. When a change *is* meant to
  alter what is drawn, regenerate the stills, commit them, and say so in the
  message.

**Porting order that worked:** title and attract first (verified tick by
tick), then the whole game tick at once, then fix diffs from the first
divergent byte. Porting a tick's subsystems one at a time and verifying each
in isolation does not work with whole-tick traces, because a partial tick
diverges immediately. Read everything the tick calls, port it all, and let
the comparator walk you through the mistakes. In the second project the
complete gameplay port needed one fix to match the 1700-tick demo.

**Tick pacing.** In a VBlank-driven game with a busy flag, a tick that takes
longer than a frame makes the following interrupts run sound only. The
trace's interrupt counts show this: in the second ROM a stage load spanned
106 frames and some gameplay ticks spanned 2. A port that runs one tick per
frame plays those moments faster. Decide deliberately how to model it, and
document the choice as an open issue until you do.
*(Fourth project)* The two-term model did not fit a game that also
decodes RLE into RAM, copies with `LDIR`, mirrors tiles bit by bit and
calls `WRTVRM`/`RDVRM` per byte. A least-squares fit over eight per-tick
counters (address set-ups, writes, mirrored bytes, `LDIR` bytes, `FILVRM`
bytes, per-byte BIOS calls, RAM-RLE and VRAM-RLE bytes; `cmd/pacefit`)
mispredicted 2 of 42,396 ticks. Count every kind of per-byte work in the
port from the start, so the fit has the terms it needs.
*(Third project)* A cost model fitted to the traces settled it:
**cycles ≈ 200 per VRAM address set-up + 66 per VRAM write**, against
59,659 cycles per frame, predicted the frame count of 76,899 of 76,918
recorded ticks. Writes alone did not fit, because a routine that sets the
address for every byte through the BIOS costs several times more per byte
than a streamed copy. The frontend keeps the busy flag set for the frames
a tick owes and runs the per-interrupt work in them, so loads take their
time and the music keeps playing.

**Coverage, and reaching what random play cannot.** *(Third project)*
Write a coverage report over a set of traces: states and sub-states seen,
plus a list of RAM conditions (jumping, fallen, knocked, wind, later
stages, time out). Random input covered falls and knocks but never cleared
a stage. **Search for good input on the port, then verify it on the
original**: try random 32-tick input chunks on a copy of the console and
keep the one that covers the most distance without falling. It found
schedules through stage 7 in seconds; recorded on the original, all 41,821
ticks matched. The port is only the search engine, never the judge.

*(Fourth project)* A harder game needed more from the search:

- **Use the game's own cheat, and allow deaths.** A no-death search
  stalled for hours at single spots. The game's hidden start cheat (26
  lives) plus "accept a lost life after 30 failures" got through. A
  verified schedule without the cheat was reused by splicing the cheat's
  key presses in front of it, after checking on the port that the two
  runs differed only in the lives bytes, then on the original.
- **Back up from the furthest point reached**, not from where the path
  ends, or repeated failures drift ever further back; shrink the chunks
  and raise the tries while stuck.
- **When it stalls, render the spot and read the code.** Three stalls,
  three game mechanics: a river spanning the screen crossed only on a
  bridge revealed by shooting an invisible object five times; a "stay
  high on the screen" preference that pinned the player above a boss it
  could only shoot upwards; and a last boss whose damage is counted per
  eye, not in the usual counter, so the progress score never moved.
  Each fix was a few lines in the progress score once the code was read.
- **`LD A,R` broke the schedules on the original.** A boss's mine fuses
  read R; the original's values differ from the port's, a different fuse
  frees bullet slots at different times, and the original's bullet
  pattern became a different fight: the player died there 23 times.
  Searching over several random R "worlds" at once still died on the
  original. What worked: **the oracle in the loop.** Record the schedule
  (seconds), find the original's first death, cut the schedule some
  ticks before it, replay the cut schedule on the port with the
  original's own logged R values (`keysearch -rtrace`), so the port is in
  the original's true state, and search on from there. The second round
  got through the boss on the original without losing a life. Any other
  replay past an R site (screenshots, probes) must also be fed the
  recorded values, or it shows a game that never happened.

**Audio.** *(Third project)* The comparator proves the PSG registers, not
what reaches the speakers. The first frontend applied the registers from
the audio thread once per `Read`, which the toolkit calls in large chunks:
the chip saw the music a few times a second, the music was garbled and
short sound effects vanished. The user heard it at once. Render each game
frame's samples on the game thread from that frame's registers, through
one function shared by the window and a headless renderer. Then **check
the output against the original's**: openMSX records audio
(`record start -audioonly`), and a comparison per 1/60 s window (aligned
by loudness envelope) of "sounding or not" and "is each side's strongest
frequency present in the other's spectrum" agreed on 98.5% of windows.
Compare presence, not the single peak: in a chord at equal volumes the
loudest note depends on each output stage's filtering.

*(Fourth project)* **The hand-off to the audio player is its own layer;
measure it in the real window.** The headless renderer matched the
original at 99.8%, and the user still heard broken music. The window's
`Read` padded every short read with silence; the player reads ahead, so
its buffer filled with silence and the ring dropped the game's samples
as stale: 66% of the music dropped. A blocking `Read` instead deadlocked
(Ebitengine calls it holding a lock the game loop needs), and
`SetBufferSize` had been given a byte count where it takes a duration.
`Read` now returns what is ready and only a ~1 ms sliver of silence when
nothing is. The check that counts: tap what `Read` hands the player in
the real window during a game, record the keys with `-record`, record the
original's audio with the same keys, and compare: 99.8% of windows (48.1%
before, on the same test). Also render at the real frame rate (NTSC
59.92 Hz, not 60) and give the chip model a DC blocker: the output is
AC-coupled, and a silent channel still outputs its volume.

**Make the frontend checkable headless too.** *(Third project)* Give the
playable frontend `-press frame:KEY,...` (scripted keys) and `-shot
file.png -shotframe N` (save the composited window and quit). Overlays and
cheats can then be checked from a still of the real window.

**Commit after each logical change**, once build, vet and tests are clean.
Never commit ROM-derived files. If `git status` offers them, `.gitignore` is
broken.

**Write throwaway probes as real programs, delete them after.** Use
temporary `_test.go` files in the engine package or a scratch `cmd/`
directory, and remove them before committing. Measuring a change ("off-road
object-frames fell from 175 to 20 over 12,000 frames") beats asserting it.

## Phase 6: only once it plays

Extensions (a second renderer, new modes, cheats, tooling) should sit *over*
the port, never inside it. The first project added a 3D chase-camera mode as
a separate package that reads the VRAM the engine already wrote and
reinterprets it. The engine itself was untouched, so the 3D mode shows the
real simulation and cannot desync from it. Keep that boundary.

When an extension must change game state (a level-select cheat):

- Put it in its own file, hook the engine at **one** documented point, and
  make it inactive unless triggered, so every trace still matches.
- **A shortcut skips state set-up that the normal path performs.** List every
  variable the skipped path establishes, not just the obvious ones. The
  second project's cheat set the stage index, number and colour. But the
  "bottom of map" marker is normally set when you finish the previous stage,
  so on a cheat-started stage a fall scrolled off the end of the map and
  crashed. Derive such values from the ROM data itself (here, the stage's
  first map record) rather than hard-coding them, and test the derivation
  against what the game actually builds.
- Reproduce a user-reported extension bug headlessly with a failing test
  first, then fix.
- *(Third project)* **Verify a derived shortcut against the real thing.**
  A level-select cheat must reproduce what playing the skipped stages
  leaves behind. Everything a stage set-up clears is rebuilt anyway, so
  list only what lies outside that range (stage number, course index, a
  wind-schedule index advanced by every wind on the way). Derive them from
  the ROM's own tables, and compare with the values the original actually
  had at each real stage start in a long trace.
- *(Third project)* **Check the feature exists before building it.** A
  request for a "flashing flag" bonus came from another version of the
  game; the ported code showed this cartridge has no such thing. Say so and
  offer the faithful option and the new-extension option.
- *(Fourth project)* **Let cheats call the game's own routines.** Item
  cheats (shield, invisibility, time stop, warp, smart bomb) went through
  one small exported file in the engine that calls the ported effect
  routines, so sounds, music changes, timers and warnings were the real
  ones for free; the frontend calls it between frames. One effect kept
  its state in the object that gave it (a timer in a "?" block's slot):
  the cheat had to lend it a free slot, and say why that is safe.
- *(Fourth project)* **Compare a level select's start with the real
  starts, RAM and VRAM, and classify every difference.** At the first
  play tick of each stage in the full-game trace, the cheat start differed
  in timing, score, lives and carried items (expected), and in leftovers
  of earlier play: a byte *below* the new-game clear range that persists
  across games and gates hidden warps (the cheat must set it), counters
  whose real value depends on how long an earlier boss lasted, tiles
  never shown in those stages, and four bytes drawn past the last name
  row. Only a byte-by-byte comparison finds the first kind.
- *(Fourth project)* **Start a level select through the game's own start
  path** (press the start key for the player, then write the stage
  variables before the first intro tick), so new-game set-up and the
  start cheats run as normal.

---

## Working with the user

- **The user playing the game is your best bug detector** for paths the
  traces have not covered. Ask them to play after each milestone, and offer
  `-record` so their session can be replayed into the original and diffed.
- **When the user says it's still wrong, believe them.** On one bug in the
  first project I twice concluded "this is genuine ROM behaviour" and was
  wrong both times; a real-hardware screenshot they supplied settled it.
  Both times the error was the same: combining two true statements into a
  false one without reading the routine they both depended on.
- **A real-hardware (or reference-emulator) screenshot or trace is worth a
  day of tracing.** With the oracle you can produce your own; do so whenever
  behaviour is disputed.
- **Report measurements, not impressions.** "All 12,000 ticks match", "all
  seven stills byte-identical" and "longest off-road stretch 86 frames → 2"
  are checkable; "looks right" is not. Say plainly which paths are verified
  and which are only translated.
- Distinguish, out loud, between a bug in the port and faithful reproduction
  of an original quirk. Both are common. Say which you think it is and why.
- *(Third project)* **When the user reports a symptom in a layer you have
  not verified against the original, look there first.** "The music is
  wrong" came with every PSG register byte proven correct; the fault was
  in the frontend's audio thread, the one layer the comparator never saw.
- *(Fourth project)* **It happened again, one layer further out.** The
  fourth port verified the rendered audio against the original and the
  user still reported wrong music: the samples were right, the hand-off to
  the speakers was not. "Verified" covers exactly the code path that was
  measured; say which path that was.
- *(Fourth project)* **Long searches run in the background; report from
  their logs, not from memory or from a notification stream that lags.**
  When asked for status, read the log and the latest state, and say what
  is verified on the original and what only on the port.

## Documentation hygiene

- Keep one analysis doc with:
  - an **Open issues** list at the top: what is actually open, including
    unverified paths and approximations, with nothing resolved left in it
  - the checksums
  - the state machine table, with what verified each state
  - the confidence-rated asset inventory
  - the VRAM map
  - the compression format
  - a verification log (which traces, how many ticks, what matched)
  - dated write-ups of every bug and correction
- **Sweep for stale claims periodically.** Doc comments rot fast. Near the
  end of the first project a sweep found four comments still saying "not
  yet ported" about work long since finished, an instruction telling future
  sessions to commit files that must never be committed, and a layout
  section describing a directory structure that had changed. Each had been
  true when written.
- When you move a resolved item out of the open list, move the *write-up*
  into the body rather than deleting it.
- *(Fourth project)* **After a milestone trace, re-measure every open
  issue.** Once the full game was recorded, half the list was stale:
  stream segments "unverified" were all verified (one only in the pause
  trace), the "partly read" input path was long ported, and "two players"
  had come from this plan's template: the game has no second player.

---

## Suggested first week

1. Repo, `.gitignore`, `NOTICE`, README skeleton, module path. *(Phase 0)*
2. Disassembler with coverage tracking, `-gaps` and `-peek`; listing; ROM
   checksums recorded. *(Phases 1-2)*
3. VRAM layout derived from the ROM's own register writes and confirmed by
   compositing one pattern+colour pair.
4. Compression format traced (if any); decoder; first assets extracted and
   **fully rendered** for inspection.
5. `cmd/extract` with a manifest and `-check`; `internal/msxrender`;
   `cmd/screenshot`. Nothing ROM-derived committed, ever.
6. Boot path, frame tick and state machine ported far enough to show the
   title screen. **Then `scripts/refdump.tcl` and `cmd/refcompare`**, with
   the attract sequence matching tick by tick. Commit the first screenshot
   as the regression baseline.
7. Read the whole game tick, port it, and drive the comparator to a match
   on the demo, then on scripted-input games. Then hand it to the user to
   play. *(Third project: the whole tick, about 2,100 instructions, matched
   the 4000-tick attract trace as soon as two manifest gaps were filled.)*
8. Scripted-input traces with the nested-interrupt placement, a coverage
   report, input search for the stages random play cannot reach, then the
   frontend with tick pacing and audio checked against the original's.
   *(Fourth project: the whole tick matched the attract trace after a few
   manifest gaps; the hard part was reaching the late game. Budget for
   the input search, the oracle in the loop at any `LD A,R`, and audio
   measured through the real window.)*
