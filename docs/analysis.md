# Analysis notes

The native Go port of a 16KB MSX1 cartridge (see `CLAUDE.md` for the
plan and the ROM's identity). Confidence key: High (traced instruction by
instruction and/or confirmed by the lockstep comparator), Medium (control
flow clear, meaning inferred), Low (plausible guess).

This file is append-only for corrections: a wrong claim gets a dated
correction note, not an edit.

## Open issues

The game code is complete: every reachable routine is ported (5,429
instructions in 244 routines; no unported stub is left). What is open is
verification, one frontend feature and documented approximations.

Not yet verified against the original (ported and playable, but no trace
has exercised it):

- **Clearing pyramids 4, 9, 10, 12, 13 and 14.** The input search entered
  and played each and gave up at 3-5 of their jewels (search limits, see
  the verification log); play up to that point matches. Their exits and
  the rest of their layouts are unchecked. Cleared and verified: 1, 2, 3,
  5, 6, 7, 8, 11 and 15.
- **The second loop beyond its first intro.** The ending and the start of
  loop 2 match, but no play in it. The loop count `$E058` raises the enemy
  speed and colour tier (`$6D0F`: type + `$E058`, at most 4), so the
  faster enemies of loop 2 are untested.
- **Enemy state 8** (an enemy stuck too long vanishes and restarts from
  its place in the pyramid data, `$7261`): reviewed against the listing,
  never seen in a trace.
- **Code coverage is not measured.** Coverage so far is judged by game
  states (`cmd/tracecov`), not by which branches of the Go code ran in a
  verified trace. Measuring it (Go coverage over the comparator runs)
  would show whether the items above hide untested code, and which paths
  to steer the search or a human player to.

Frontend:

- **No joystick or gamepad input.** The original reads joystick port A
  (PSG R14, `$45E8`) and the engine models it (`Console.JoyA`), but the
  window maps only the keyboard.

Known approximations (documented, measured):

- **Frame pacing in the window.** The paced `Machine` mispredicts 11 of
  39,488 traced ticks by one frame, and runs the interrupts a long tick
  owes after it rather than between its VRAM writes, so sound can be a
  frame off the original (the comparator places them exactly). Gameplay
  is unaffected: recordings are indexed by tick and replay exactly.
  Audio measured against the original: see "Audio".
- **Unreachable-by-design values not modelled**: `LDIR` with BC = 0 in the
  enemy copy at `$6B88` (a pyramid with no enemies would copy 64KB) and
  the do-while list loops (`$57FF`, `$6532`, `$6CE7`, `$6653`) with a
  count of 0. Every pyramid has at least one enemy, jewel and dagger (the
  builder ran for all 15 without a panic), so these are documented, not
  ported for count 0, except `$6653`, which is ported as written.
- **BIOS-specific values.** The bytes at `$F000-$F37F` that the map probes
  can read, and the I register (0), are C-BIOS's as measured; a real MSX
  BIOS would leave other bytes above `$F000`.
- The roles of several RAM bytes are still Medium (see the RAM map).

## Checksums

| | |
| --- | --- |
| File | `King's Valley 1 (1985) Konami [RC-727] [8553].rom` |
| Size | 16,384 bytes |
| CRC32 | `b1b3543e` |
| MD5 | `c5698102c7e452b03fc7b9bb80a3768a` |
| SHA-1 | `dda0d1bf4da8b159d08add8b7f4dc75da3e6e242` |
| SHA-256 | `01bd3179540c886ab658626ca0460c329d167327fabd43eb615883375e9e9a3e` |

## Disassembly

`cmd/disasm` from `docs/entries.txt`: 5,429 instructions in 244
routines, all reachable from INIT `$406C` and the hook `$401A`; 9,818
bytes of code, 3,188 of claimed data (streams, tables), 3,272 unclaimed
(data referenced by `LD rr,nn` and covered by the manifest). One
overlapped instruction: `JR Z,$4F34` at `$4F31` lands on the operand of
`CP $20`, which decodes as a `JR NZ` that is never taken there. Dead code
(no reference, no 16-bit word in the ROM): `$4501` (a tripler), `$4542`
(SETRD helper), the `LD A,n` alternatives at `$4D40`, `$4DB9`, `$7173`
skipped by `JR`, and `SCF / RET` at `$511B`.

Seven inline jump tables follow `CALL $404B` (`JUMP_A`), all tight. Five
dispatchers push a continuation before dispatching (`$40B2`, `$4C59`,
`$5803`, `$65F1`, `$6814`, `$6F79`), and several handlers `POP HL` it away
(`$4BE2`, `$4C2F`, `$6742`, `$717E`, `$71DD`, `$7280`): ported as a
returned flag ("skip the rest of the caller").

## Boot, interrupt, tick (High, trace-confirmed)

- INIT `$406C` hooks `H.KEYI` with `JP $401A`, clears `$E000-$E6FF`, sets
  the busy flag, runs `$459B` (mixer `$B8`, sound `$20`, 16KB VRAM clear
  through `$409C → $44AB`, R0-R7 from `$45C0`), clears the flag, `EI` at
  `$4099`, `JR $` at `$409A`.
- 19 interrupts happen during INIT (the first right after the mixer's
  `WRTPSG`, the rest during the VRAM clear), placed by VRAM write count.
- The handler runs the sound driver `$7B36` on every interrupt, then the
  tick (`$45C8` input, `$40A3` dispatch) unless busy. The exit path's
  `CALL M,$7B36` occurred once (the p8 trace) and matched; the comparator
  refuses return addresses other than `$4021`/`$403C`.

## State machine (`$E000`, table at `$40BA`)

| State | Address | Role | Verified by |
| --- | --- | --- | --- |
| 0 | `$40D0` | logo scroll, wait, title letters | attract |
| 1 | `$410D` | title (256 ticks) | attract |
| 2 | `$4115` | demo set-up | attract |
| 3 | `$429F` | game start ("PLAY START" flashing) | play1, ks1 |
| 4 | `$4141` | pyramid intro (odd `$E051`) / map screen set-up (even) | attract, play1, ks1 |
| 5 | `$41D5` | play (`$4B0C`) / map screen (`$77C3`) | attract, play1, ks1 |
| 6 | `$41F5` | life lost; game over text | play1 |
| 7 | `$4243` | game over wait | play1 |
| 8 | `$4266` | next phase (`$E051`++) | ks1 |
| 9 | `$427B` | screen flip (scroll) | p2, p6, p8 and others |
| 10 | `$429C` | ending | p15 |

`$E051` counts phases: odd inside a pyramid, even on the map screen
between pyramids. `$E002` bit 6 is "in a game"; with it clear, every
state returns through the continuation `$4644` (a key press goes to the
title, or starts a game from it).

## VRAM layout (High)

R0-R7 = `02 E2 0E 7F 07 76 03 E4`: Screen 2, 16x16 sprites, names
`$3800`, colours `$0000`, patterns `$2000`, sprite attributes `$3B00`,
sprite patterns `$1800`. Confirmed by rendering the traces.

## Formats

- **RLE** (`$451A`, with address `$4514`, tripled `$44F1`; High): 0 end,
  `$80` new VRAM address, `$01-$7F` repeat the next byte, `$81-$FF` copy
  n&$7F bytes. Same as the previous port's.
- **Text** (`$4055`, `$4051` erases; High): address word, bytes, `$FE`
  new address, `$FF` end.
- **Sound** (`$7AA9`, driver `$7B36`; High): see `internal/engine/sound.go`.
  Sound 1 (falling) is built in RAM at `$E03B-$E03E` from the template
  at `$7B8D` and swept by the driver.
- **Pyramid data** (`$6D70`, builder `$6AA3`; High): up to four section
  bytes (template set, sub-template; a `$3x` ends early), each a 16x22
  bit-packed brick layout (44 bytes); four door records (Y, X|screen,
  destination pyramid/door) or `$FF`; enemies (count, 3 bytes each);
  jewels (count, colour|1, Y, X|screen); daggers; picks; sliding walls;
  traps; stairs (start points, built upwards through empty cells).
- **Map** (`$E700-$EFFF`): 96x24 cells, `$60` per row, one byte per
  cell; the class is the high nibble, the tile comes from `$5D8B`.

## RAM map (main variables)

| Address | Role | Confidence |
| --- | --- | --- |
| `$E000/$E001` | state / sub-state | High |
| `$E002` | bit 6: in a game | High |
| `$E003` | frame counter | High |
| `$E004` | state timer | High |
| `$E005` | busy flag | High |
| `$E008/$E009` | new / held input | High |
| `$E010-$E039` | sound channels (3 x 14) | High |
| `$E03A` | PSG mixer copy | High |
| `$E043-$E045` | hi-score (BCD) | High |
| `$E049-$E04B` | score (BCD) | High |
| `$E050` | lives (REST) | High |
| `$E051` | phase | High |
| `$E052` | next extra-life threshold | High |
| `$E053` | alive (0 ends the life / the demo) | High |
| `$E054/$E055/$E056/$E057` | pyramid / next / entry door / exit door | High |
| `$E058` | loop count | Medium |
| `$E062/$E063` | pyramids cleared, a bit each | High |
| `$E080/$E082` | demo input pointer / count | High |
| `$E0B0-$E12F` | sprite attributes in RAM | High |
| `$E130` | door sequence step | High |
| `$E134-$E14D` | player (see `player.go`) | High |
| `$E14E` | enemy start records | High |
| `$E162/$E164/$E165` | enemy IX / count / index | High |
| `$E166-$E169` | stair scan results | High |
| `$E16A` | enemies, 22 bytes each | High |
| `$E1C5` | doors, 7 bytes each | High |
| `$E1F2-$E1F5` | jewels collected / count / index / 9-byte list | High |
| `$E261-$E264` | daggers (held / index / count / 17-byte list) | High |
| `$E2CA-$E2CC` | picks | High |
| `$E31C-$E31E` | sliding walls | High |
| `$E3FD-$E3FF` | traps | High |
| `$E700-$EFFF` | the map | High |
| `$F000-$F37F` | not the game's, but read by the map probes | High (oracle) |

## Assets

`assets/manifest.go`: 73 entries, all re-derived by `cmd/extract -check`.
Streams are exact (decoded with their declared segments). `stagedata`
(`$5D8B-$6517`) and `sound` (`$7CE9-$7FFF`) are large blocks to be split
when their tables are fully mapped (Medium extent, High content).
`$6300`, referenced as `LD HL,$6300` at `$4831`, is a VRAM address
(`$2300`), not data: not in the manifest.

## Verification log

| Date | Trace | Ticks | Result |
| --- | --- | --- | --- |
| 2026-10-12 | attract (no input) | 3,000 | ticks 0-597 match, then unported `$4FEE` |
| 2026-10-12 | attract | 3,000 | match after two fixes (below) |
| 2026-10-12 | attract | 12,000 | all match (several demo cycles) |
| 2026-10-12 | play1 (keygen seed 1, random) | 6,000 | all match: deaths, game over, a second game |
| 2026-10-12 | ks1 (keysearch: pyramid 1 cleared) | 3,400 | all match: exit door, map screen, pyramid 2 built |
| 2026-10-12 | comparator flip test | | a flipped RAM byte (tick 1500) and VRAM byte (tick 1700) reported at the right tick and address |
| 2026-10-12 | machinecheck (paced Machine) | attract 12,000, play1 6,000 | RAM, VRAM and frame timing all match |
| 2026-10-12 | p3, p7, p8, p6, p2 (keysearch, cheat start by pokes) | 3,551 / 4,432 / 5,112 / 8,264 / 6,134 | all match: each pyramid cleared, exit, map screen, next pyramid |
| 2026-10-12 | p5, p11 | 6,574 / 4,131 | all match |
| 2026-10-12 | keysearch gave up (1.5M rounds or 80,000 cells) | | pyramids 4 (4/6 jewels), 9 (3/4), 10 (5/6), 12 (4/6), 13 (3/5), 14 (4/6) |
| 2026-10-12 | p15 | 4,742 | all match: pyramid 15 cleared, map screen, **the ending** (state 10), extra life and bonus, second loop starts at pyramid 1 |

## Audio

`scripts/refaudio.tcl` records the original's audio in openMSX over a key
schedule; `cmd/sndpreview` renders the port's through the same session
code the window uses; `cmd/audiocmp` compares them per 1/60 s window
(aligned by loudness envelope): both sounding or both silent, and each
side's strongest frequency present in the other's spectrum.

| Run | Windows | Sounding agrees | Frequencies agree (both sounding) |
| --- | --- | --- | --- |
| attract, headless | 3,066 | 97.8% | 94.2% of 378 |
| pyramid 1 game (ks1), headless | 3,464 | 98.9% | 97.2% of 2,671 |
| real window, tapped at the player's `Read` (`cmd/play -tap`), against the original over the window's own `-record` | 2,622 | 99.0% | 99.2% of 845 |

The window's hand-off drops nothing (the previous port's failure mode).

## Pacing

`cmd/pacefit` fits cycles = base + coefficients x the engine's per-tick
counters (VRAM address set-ups and writes, mirrored bytes, LDIR bytes,
fill bytes, BIOS WRTVRM/RDVRM calls, RLE bytes, map cell-to-tile lookups,
map probes, enemies updated) to the frame counts of the traces. The first
fit (17,998 ticks, no screen flips) mispredicted none; the screen flip
(704 writes, each through the `$5D75` tile lookup) then took 4 frames
where 1 was predicted, until the lookups were counted. Over 39,488 ticks,
11 are now off by one frame. A tick near a frame boundary can overrun
through game logic alone (an interrupt landed inside `$5CFE`, after the
tick's last VRAM write); the probe and enemy counters stand in for it.

## Cheat starts against real arrivals

The pyramid-select cheat (`internal/session/cheat.go` over
`engine.StartPyramid`) goes through the game's own start and writes only
`$E055` (pyramid) and `$E056` (entry door, derived from the pyramid data's
door records). `cmd/tracediff` compared the first play tick of pyramid 8
reached for real (the p7 trace, tick 4374) with a cheat start of it (p8,
tick 449), every byte of RAM and VRAM:

- expected: the held input `$E009`, score and hi-score digits (RAM and
  name table), sound channel 3 and the falling-tone bytes `$E02D-$E03E`,
  the sprite rotation `$E061` and enemy sprite bytes (timing), the phase
  `$E051` (3 against 1; only its parity is used), `$E062` (pyramid 7
  cleared), the stack (`$E6B6-$E6D3`);
- leftovers of earlier play that the pyramid does not use: `$E057` (the
  door left by, used only by the map screen), `$E05F` (a stair flag both
  sides rewrite before reading), the map screen's marker sprites
  (`$E0FD-$E103`, hidden), the map screen's tile patterns and colours
  (`$0600-$073F`, `$2600-$273F` in each third), and dagger sprite frames
  past the no-item player patterns (`$18C0-$193E`, mirrored at
  `$1BC1-$1C3E`).

No difference in state the pyramid reads. A cheat start has no pyramid
marked cleared, so every pyramid's jewels are there and its clear bonus
is paid.

## Bugs found by the comparator, and other corrections

- **2026-10-12, tick 598** (`$4CFA`): the player's sprite patterns take
  +`$60` when facing *left* (`RRA / JR NC` skips the add when bit 0 is
  clear). The port had the sense reversed.
- **2026-10-12, tick 833** (`$511D`): `LD BC,$0810` before `CALL $50E1`
  is 8 across and 16 down; the port had them swapped, so the player left
  a stair one step early.
- **2026-10-12, review** (`$6AAD`): `INC A / LD ($E053),A` after the map
  fill stores 1, not `$15`: the fill does `LD (HL),$14`, which leaves A
  (0 from the `XOR A` before) alone. Caught reading the translation
  against the listing before any trace reached it.
- **2026-10-12, search** (`$50E1`): the map probe's row is
  `(Y + offset) >> 3 & $1F`, so an object near the top of the screen
  reads rows 24-31, past the map, up to `$F31F`. The port panicked on
  `$F2B7`; the oracle (a dump of `$F000-$FFFF` at three ticks) showed the
  power-on pattern and C-BIOS data there, constant through the game.
  Modelled in `internal/engine/highram.go`; BIOS-specific.
- **2026-10-12** (`$731A`): `LD A,I` (probably meant as `LD A,R`) reads
  the I register, which is 0 under C-BIOS (checked with a breakpoint) and
  on real machines after reset; the branch it decides is never taken.
