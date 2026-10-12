// Package engine is the native Go port of the cartridge's game code. No
// Z80 code runs: each routine was read from the listing and rewritten
// here. Every ported function names the address range it came from.
//
// Confidence key in comments: High (traced instruction by instruction
// and/or confirmed by the lockstep comparator), Medium (control flow
// clear, meaning inferred), Low (plausible guess).
package engine

import (
	"fmt"

	"go-msx-cryptexplorer/assets"
)

// RAM covers the game's work area $E000-$EFFF. INIT clears only
// $E000-$E6FF and puts the stack at $E700; $E700-$EFFF (the stage map
// buffer and more) starts with whatever the machine left there.
const (
	RAMBase = 0xE000
	RAMSize = 0x1000
)

// Console is the hardware the game sees, plus the game RAM. The ported
// routines are methods on it.
type Console struct {
	RAM [RAMSize]byte
	VDP VDP
	PSG [16]byte
	// Keys is the keyboard matrix as SNSMAT returns it: a 0 bit is a
	// pressed key. Row 8 holds SPACE (bit 0) and the cursor keys.
	Keys [11]byte
	// JoyA is what PSG R14 reads with port A selected: bits 0-5 low when
	// pressed. $3F with nothing plugged in.
	JoyA byte

	// Unported is set by the first stub reached ("$XXXX"), so the
	// comparator can say where the port stops.
	Unported string

	// NestAt lists, for the tick in progress, where interrupts are taken
	// while the game is busy (see NestPoint). The comparator fills it
	// from the reference trace; the frontend from its pacing model.
	NestAt []NestPoint
	// VW counts VRAM data writes since the tick (or INIT) began, and
	// SetAddrs the VRAM address set-ups; together they predict how many
	// frames a tick takes (see Machine).
	VW       int
	SetAddrs int
	// SS counts sound events since the tick (or INIT) began.
	SS int
	// Cost counts slower per-byte work, for the pacing model:
	// [0] bit-mirrored bytes, [1] RAM bytes copied (LDIR/LDDR),
	// [2] VRAM fill bytes, [3] BIOS WRTVRM/RDVRM calls, [4] stream bytes.
	Cost [5]int
	// Interrupts counts interrupts taken (H.KEYI entries).
	Interrupts int

	// High is $F000-$F37F (see highram.go): $F000 is written by the map
	// fill at $6D9A, one byte beyond the map; the map probes can read
	// the rest.
	High [HighSize]byte
}

// New returns a console with the power-on state the reference machine
// (openMSX, C-BIOS_MSX1_JP) shows: keys released, no joystick. The game
// RAM above $E700 is seeded by the caller (NewMachine, or the comparator
// from tick 0).
func New() *Console {
	c := &Console{JoyA: 0x3F, High: powerOnHigh()}
	for i := range c.Keys {
		c.Keys[i] = 0xFF
	}
	return c
}

// unported records the first unported routine reached.
func (c *Console) unported(addr uint16) {
	if c.Unported == "" {
		c.Unported = fmt.Sprintf("$%04X", addr)
	}
}

// --- CPU address helpers, so pointer-walking routines translate line for
// line. ---

// rd reads a CPU address: game RAM, the cartridge (through the asset
// manifest, which panics on anything never extracted), or the two BIOS
// port bytes at $0006/$0007.
func (c *Console) rd(a uint16) byte {
	switch {
	case a >= RAMBase && a < RAMBase+RAMSize:
		return c.RAM[a-RAMBase]
	case a >= 0x4000 && a < 0x8000:
		return assets.ROMByte(int(a))
	case a == 0x0006 || a == 0x0007: // VDPDR, VDPDW: both $98 on C-BIOS
		return 0x98
	case a >= HighBase && a < HighBase+HighSize:
		return c.High[a-HighBase]
	}
	panic(fmt.Sprintf("engine: read of unmapped $%04X", a))
}

// wr writes a CPU address. Writes to the cartridge are ignored, as on
// hardware; anything outside the game RAM panics.
func (c *Console) wr(a uint16, v byte) {
	switch {
	case a >= RAMBase && a < RAMBase+RAMSize:
		c.RAM[a-RAMBase] = v
	case a >= 0x4000 && a < 0x8000:
	case a >= HighBase && a < HighBase+HighSize: // $F000: the map fill's last byte ($6D9A)
		c.High[a-HighBase] = v
	default:
		panic(fmt.Sprintf("engine: write of $%02X to unmapped $%04X", v, a))
	}
}

func (c *Console) rd16(a uint16) uint16 { return uint16(c.rd(a)) | uint16(c.rd(a+1))<<8 }

func (c *Console) wr16(a uint16, v uint16) {
	c.wr(a, byte(v))
	c.wr(a+1, byte(v>>8))
}

// inc and dec are INC (HL) / DEC (HL); they return the new value.
func (c *Console) inc(a uint16) byte { v := c.rd(a) + 1; c.wr(a, v); return v }
func (c *Console) dec(a uint16) byte { v := c.rd(a) - 1; c.wr(a, v); return v }

// ldir copies n bytes upwards, byte by byte (so overlapping fills work).
func (c *Console) ldir(dst, src, n uint16) {
	c.Cost[1] += int(n)
	for ; n > 0; n-- {
		c.wr(dst, c.rd(src))
		dst++
		src++
	}
}

// lddr copies n bytes downwards.
func (c *Console) lddr(dst, src, n uint16) {
	c.Cost[1] += int(n)
	for ; n > 0; n-- {
		c.wr(dst, c.rd(src))
		dst--
		src--
	}
}

// --- VDP (TMS9918A) ---

// VDP is the video chip as the game drives it: 16KB VRAM, R0-R7 and the
// address pointer, which auto-increments on every data access and is
// shared by reads and writes.
type VDP struct {
	VRAM [0x4000]byte
	Regs [8]byte
	Addr uint16
}

// setAddr sets the VRAM pointer (SETWRT/SETRD).
func (c *Console) setAddr(a uint16) {
	c.VDP.Addr = a & 0x3FFF
	c.SetAddrs++
}

// out98 writes a VRAM data byte at the pointer, counts it, and takes any
// nested interrupt due after this write.
func (c *Console) out98(v byte) {
	c.VDP.VRAM[c.VDP.Addr] = v
	c.VDP.Addr = (c.VDP.Addr + 1) & 0x3FFF
	c.VW++
	c.pollNested()
}

// in98 reads a VRAM data byte at the pointer.
func (c *Console) in98() byte {
	v := c.VDP.VRAM[c.VDP.Addr]
	c.VDP.Addr = (c.VDP.Addr + 1) & 0x3FFF
	return v
}

// NestPoint places an interrupt taken while busy: after VW VRAM writes
// and SS sound events (sound starts, and the game's direct reads of the
// sound blocks). Writes alone cannot place an interrupt between the last
// write and a later sound event.
//
// Exit marks the handler's exit path instead ($4034: CALL M,$7B36): a
// VBlank that came while interrupts were off at the very end of the tick
// is seen there, after the busy flag is cleared, and its sound work runs
// once inline.
type NestPoint struct {
	VW, SS int
	Exit   bool
}

// pollNested takes the interrupts due at the current point: called after
// each VRAM write and before each sound event.
func (c *Console) pollNested() {
	for len(c.NestAt) > 0 && !c.NestAt[0].Exit && c.NestAt[0].VW <= c.VW && c.NestAt[0].SS <= c.SS {
		c.NestAt = c.NestAt[1:]
		c.nestedInterrupt()
	}
}

// soundRead is the game reading its sound blocks directly ($41F5,
// $4B13, $4B17, $77FA). It is a sound event for placing interrupts.
func (c *Console) soundRead(a uint16) byte {
	c.pollNested()
	c.SS++
	return c.rd(a)
}

// --- BIOS (the C-BIOS routines the game calls) ---

// wrtvdp is WRTVDP ($0047): register C&7 = B.
func (c *Console) wrtvdp(reg, v byte) { c.VDP.Regs[reg&7] = v }

// wrtvrm is WRTVRM ($004D): VRAM[HL] = A.
func (c *Console) wrtvrm(hl uint16, a byte) {
	c.Cost[3]++
	c.setAddr(hl)
	c.out98(a)
}

// rdvrm is RDVRM ($004A): A = VRAM[HL].
func (c *Console) rdvrm(hl uint16) byte {
	c.Cost[3]++
	c.setAddr(hl)
	return c.in98()
}

// wrtpsg is WRTPSG ($0093): PSG register A = E.
func (c *Console) wrtpsg(a, e byte) { c.PSG[a&15] = e }

// rdpsg is RDPSG ($0096). R14 reads the joystick port selected by R15
// bit 6; only port A is modelled (the game selects it).
func (c *Console) rdpsg(a byte) byte {
	if a&15 == 14 {
		if c.PSG[15]&0x40 == 0 {
			return c.JoyA | 0xC0
		}
		return 0xFF
	}
	return c.PSG[a&15]
}

// snsmat is SNSMAT ($0141).
func (c *Console) snsmat(row byte) byte {
	if int(row) < len(c.Keys) {
		return c.Keys[row]
	}
	return 0xFF
}
