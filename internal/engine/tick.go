package engine

// Boot, the interrupt handler and the tick.
//
// The original runs entirely inside the VBlank interrupt: INIT parks in
// JR $ at $409A, and the H.KEYI hook at $401A runs the per-interrupt
// sound work, then (unless the busy flag $E005 bit 0 is set) the tick.
// The port splits that into Boot, Interrupt and Tick so the comparator
// can check the state where the reference trace records it ($402A).

// Boot runs INIT up to its EI at $4099. Interrupts the original took
// during INIT (BIOS calls re-enable them) come from NestAt, as VRAM write
// counts since INIT's entry. Ported from $406C-$4099. High.
func (c *Console) Boot() {
	c.VW = 0
	c.SS = 0
	// $406C-$407A: DI, IM 1, H.KEYI patched with JP $401A, SP=$E700: no
	// equivalent in a native port.
	for i := 0; i < 0x700; i++ { // $407D: LDIR clear of $E000-$E6FF
		c.RAM[i] = 0
	}
	c.wr(0xE005, 1)
	c.vdpInit()
	c.wr(0xE005, 0)
	// $4096: RDVDP, $4099: EI.
}

// vdpInit sets the PSG mixer, starts sound $20, clears all 16KB of VRAM
// and writes R0-R7. Ported from $459B-$45AE (and $409C-$40A2, a
// development leftover: NOP x4 / JP $44AB). High.
func (c *Console) vdpInit() {
	c.soundMixer(0xB8)
	// The BIOS WRTPSG re-enables interrupts: the reference machine takes
	// its first INIT-time interrupt here, before any VRAM write.
	c.pollNested()
	c.soundStart(0x20)
	// The fill's SETWRT uses HL as the sound start left it ($E03A): the
	// 16KB fill wraps, so only the final pointer depends on it.
	c.vfill(0xE03A, 0x4000, 0)
	c.vdpRegs()
}

// vdpRegs writes R0-R7 from the table at $45C0. Ported from $45AF-$45BF.
// High.
func (c *Console) vdpRegs() {
	for r := uint16(0); r < 8; r++ {
		c.wrtvdp(byte(r), c.rd(0x45C0+r))
	}
}

// nestedInterrupt is an interrupt taken while the game is busy (during
// INIT or a tick that overran its frame): the handler runs the sound
// work, sees the busy flag and returns. Ported from $401A-$4026, $4034.
func (c *Console) nestedInterrupt() {
	c.Interrupts++
	c.soundTick()
}

// Interrupt is the frame interrupt taken while the game is idle (at the
// JR $ in INIT, or in the previous tick's exit path after the busy flag
// is cleared): the handler runs the sound work and claims the busy flag.
// The tick body follows (Tick). Ported from $401A-$4029. High.
func (c *Console) Interrupt() {
	c.Interrupts++
	c.soundTick()
	c.inc(0xE005)
	c.VW = 0
	c.SS = 0
	c.SetAddrs = 0
	c.Cost = [5]int{}
}

// Tick runs one tick body and releases the busy flag. Interrupts due
// before the first VRAM write (NestAt entries of 0) are taken at its
// start, and any left over after the last write at its end; an exit-path
// entry runs after the busy flag is cleared. Ported from $402A-$403D.
// High.
func (c *Console) Tick() {
	c.pollNested()
	c.readInput()
	c.tickDispatch()
	for len(c.NestAt) > 0 && !c.NestAt[0].Exit {
		c.NestAt = c.NestAt[1:]
		c.nestedInterrupt()
	}
	c.wr(0xE005, 0)
	// The exit path reads a pending VBlank itself: the sound work runs,
	// but no interrupt is taken.
	for len(c.NestAt) > 0 {
		c.NestAt = c.NestAt[1:]
		c.soundTick()
	}
}

// readInput reads the controls into $E009 (held) and $E008 (newly
// pressed): during the demo ($E000 = 5 with $E002 bit 6 clear) from the
// recorded demo input, else from the joystick and keyboard. Ported from
// $45C8-$45DD. High.
func (c *Console) readInput() {
	var a byte
	if c.rd(0xE002)&0x40 == 0 && c.rd(0xE000) == 5 {
		a = c.demoInput()
	} else {
		a = c.input()
	}
	c.edges(0xE009, a)
}

// edges stores a in (hl) and the newly set bits in (hl-1), returning
// them. Ported from $45E1-$45E7. High.
func (c *Console) edges(hl uint16, a byte) byte {
	old := c.rd(hl)
	c.wr(hl, a)
	a = (a ^ old) & a
	c.wr(hl-1, a)
	return a
}

func rrca(a byte) byte { return a>>1 | a<<7 }

// input reads joystick port A and the keyboard into one byte: bit 0 up,
// 1 down, 2 left, 3 right, 4 trigger A / SPACE, 5 trigger B / SELECT.
// Ported from $45E8-$4620. High.
func (c *Console) input() byte {
	c.wrtpsg(15, 0x8F)
	joy := ^c.rdpsg(14) & 0x3F
	e := rrca(^c.snsmat(7)) & 0x20 // SELECT (row 7 bit 6)
	b := rrca(rrca(^c.snsmat(8)))
	cc := b&0x04 | e // LEFT
	b = rrca(rrca(b))
	cc |= b & 0x18       // RIGHT, SPACE
	cc |= rrca(b) & 0x03 // UP, DOWN
	return cc | joy
}

// demoInput returns the demo's recorded input: (input, frames) pairs
// from the pointer at $E080, the current one's frames left in $E082. A
// length of $FF ends the demo ($E053 = 0). Ported from $4621-$4643. High.
func (c *Console) demoInput() byte {
	b := c.dec(0xE082)
	hl := c.rd16(0xE080)
	a := c.rd(hl)
	if b == 0 {
		hl++
		n := c.rd(hl)
		if n == 0xFF {
			c.wr(0xE053, 0)
		} else {
			c.wr(0xE082, n)
			hl++
			c.wr16(0xE080, hl)
		}
	}
	return a
}

// tickDispatch counts the frame and runs the handler for state $E000
// with sub-state $E001. Outside a game ($E002 bit 6 clear) every handler
// returns to $4644 (a continuation pushed before the dispatch), which
// watches for a key press. Ported from $40A3-$40CF. High.
func (c *Console) tickDispatch() {
	c.inc(0xE003)
	state := c.rd(0xE000)
	sub := c.rd(0xE001)
	attract := c.rd(0xE002)&0x40 == 0
	c.dispatchState(state, sub)
	if attract {
		c.attractKeys()
	}
}

// dispatchState runs state handler s through JUMP_A ($404B) and the
// table at $40BA.
func (c *Console) dispatchState(s, sub byte) {
	switch s {
	case 0:
		c.stateLogo(sub)
	case 1:
		c.stateTitle()
	case 2:
		c.stateDemoStart()
	case 3:
		c.stateStart(sub)
	case 4:
		c.stateIntro(sub)
	case 5:
		c.statePlay()
	case 6:
		c.stateLifeLost()
	case 7:
		c.stateGameOver()
	case 8:
		c.stateNextLife()
	case 9:
		c.stateExit()
	case 10:
		c.stateEnding(sub)
	default:
		panic("engine: state out of range")
	}
}

// attractKeys is the continuation after every attract state: a new key
// press goes to the title screen, or, on the title, a trigger starts a
// game. Ported from $4644-$467C. High.
func (c *Console) attractKeys() {
	a := c.edges(0xE041, c.input()) // $403E: a leftover that jumps to $45E8
	if a == 0 {
		return
	}
	c.soundStart(0x20)
	b := a
	c.wr(0xE004, 0)
	if c.rd(0xE000) != 1 {
		c.wr(0xE000, 1)
		c.clearScreen()
		c.wrtvdp(7, 0xE0)
		c.drawTitle()
		return
	}
	if b&0x30 == 0 {
		return
	}
	c.wr(0xE002, c.rd(0xE002)|0x40)
	c.wr16(0xE000, 0x0003)
}
