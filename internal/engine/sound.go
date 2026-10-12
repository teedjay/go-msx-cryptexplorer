package engine

// The sound driver: three 14-byte channel blocks at $E010, $E01E and
// $E02C, one per PSG tone channel (C = 1, 3, 5: the tone-period high
// register). Sound starts ($7AA9) load a block; the per-interrupt work
// ($7B36) advances each active block and writes the PSG through WRTPSG.
//
// Block layout (High for the fields the code below names; meaning
// Medium):
//
//	+0  frames left on the current note     +7  current volume
//	+1  note length (reload)                +8  decay counter
//	+2  sound id: bit 7 music, bit 6 no     +9  loop counter ($FE)
//	    tone/mixer reset, 0 = idle          +A  music length unit
//	+3  stream pointer (lo, hi)             +B  detune (+1 on the period)
//	+5  octave shift                        +C  decay start offset
//	+6  music volume                        +D  sustain level
//
// The game reads these blocks directly (sound events, see soundRead).

// soundMixer writes the mixer (PSG R7) and remembers it in $E03A.
// Ported from $7B2D-$7B35. High.
func (c *Console) soundMixer(a byte) {
	c.wr(0xE03A, a)
	c.wrtpsg(7, a)
}

// soundStart starts sound a (bits 0-5 the id, bit 6 and bit 7 flags kept
// in the block). The breakpoint the reference trace counts sound events
// with is on $7AA9, so an interrupt due before this start is taken
// first. Ported from $7AA9-$7AF5. High.
func (c *Console) soundStart(a byte) {
	c.pollNested()
	c.SS++
	cc := a
	id := a & 0x3F
	n := 2
	hl := uint16(0xE012)
	switch {
	case id < 0x0B:
		n = 1
		hl = 0xE02E
	case id < 0x11:
	default:
		n = 3
	}
	if id < c.rd(hl)&0x3F {
		return // a higher-priority sound is playing
	}
	de := 0x7CF3 + uint16(id)*2
	hl -= 2
	for ; n > 0; n-- {
		ix := hl
		c.wr(hl, 1)
		c.wr(hl+1, 1)
		c.wr(hl+2, cc)
		c.wr(hl+3, c.rd(de))
		c.wr(hl+4, c.rd(de+1))
		c.wr(ix+9, 0)
		hl += 4 + 0x0A
		de += 2
	}
}

// soundTick is the per-interrupt sound work: the mixer is rewritten from
// $E03A, then each active channel advances. Ported from $7B36-$7B5D.
// High.
func (c *Console) soundTick() {
	c.soundMixer(c.rd(0xE03A))
	ch := byte(1)
	for ix := uint16(0xE010); ix < 0xE010+3*0x0E; ix += 0x0E {
		a := c.rd(ix + 2)
		if a == 1 {
			c.soundSweep(ix, ch)
		}
		if a != 0 {
			c.soundChannel(ix, ch, a)
		}
		ch += 2
	}
}

// soundSweep keeps sound 1 (the falling tone) on channel 3 pointed at a
// stream the driver builds in RAM at $E03C: first a copy of the four
// bytes at $7B8D (count, length 1, volume $B, period), then on each later
// frame the period's low byte rises by 8 (with a carry into the high
// nibble), so the pitch falls. $E03B is the count; the game clears it to
// restart the sweep. Ported from $7B5E-$7B8C. High.
func (c *Console) soundSweep(ix uint16, ch byte) {
	if ch < 5 {
		return
	}
	if c.rd(0xE03B) >= 1 {
		v, f := add8(c.rd(0xE03E), 8, false)
		c.wr(0xE03E, v)
		if f.C {
			c.inc(0xE03D)
		}
	} else {
		c.lddr(0xE03E, 0x7B90, 4)
	}
	c.wr16(ix+3, 0xE03C)
}

// soundChanMixer selects channel 3's mixer setting: tone (d = 1) or noise
// (d = 0). Other channels leave the mixer alone. Ported from $7B20-$7B2C.
func (c *Console) soundChanMixer(ch, d byte) {
	if ch != 5 {
		return
	}
	if d == 1 {
		c.soundMixer(0xB8)
	} else {
		c.soundMixer(0x9C)
	}
}

// soundChannel advances one active channel; a is the A the code enters
// $7B91 with ((IX+2), or a stale byte after a loop command). Ported from
// $7B91-$7BA2, with the stream reader below. High.
func (c *Console) soundChannel(ix uint16, ch, a byte) {
	if a&0x40 == 0 {
		c.soundChanMixer(ch, 1)
	}
	if c.rd(ix+2)&0x80 != 0 {
		c.soundMusicFrame(ix, ch)
		return
	}
	if c.dec(ix) != 0 {
		return
	}
	c.soundNext(ix, ch)
}

// soundNext reads the channel's next stream command. Ported from
// $7BA3-$7C2B (effects) and $7AF6-$7B1F (the $FE loop command). High.
//
// Effects stream:
//
//	$FE n lo hi   loop: jump to hi:lo n-1 more times (n=0: forever)
//	$FF           end: silence the channel and free it
//	$2x           set the note length to x, then a note follows
//	$1x vv        noise period x (bit 4 of vv clear: x-$10), then the
//	              note's volume byte is vv with bit 4 cleared
//	vp pp         tone: volume v, period $pp + $100*p
//
// (bit 7 of the id: music, see soundMusic.)
func (c *Console) soundNext(ix uint16, ch byte) {
	hl := c.rd16(ix + 3)
	a := c.rd(hl)
	if a == 0xFE {
		c.soundLoop(ix, ch, hl)
		return
	}
	if a == 0xFF {
		c.wr(ix+9, 0)
		c.wr(ix+0x0B, 0)
		c.soundChanMixer(ch, 1)
		c.wr(ix+2, 0)
		c.soundVolume(ch, 0)
		return
	}
	if c.rd(ix+2)&0x80 != 0 {
		c.soundMusic(ix, ch, hl)
		return
	}
	if a&0xF0 == 0x20 {
		c.wr(ix+1, a&0x0F)
		hl++
		a = c.rd(hl)
	}
	b := a
	a &= 0xF0
	if a == 0x10 {
		e := c.rd(hl) & 0x1F
		hl++
		b = c.rd(hl)
		if b&0x10 == 0 {
			e -= 0x10
		}
		b &^= 0x10
		hl--
		c.wrtpsg(6, e)
		c.soundChanMixer(ch, 0)
		// A after $7B20: C when the channel is not 3, else 7 (WRTPSG
		// keeps A); either way its high nibble is 0 at $7BF6.
		a = 0
		hl++
	}
	if c.rd(ix+2)&0x40 != 0 {
		c.wr16(ix+3, hl+1)
		c.soundNote(ix, ch, b)
		return
	}
	a &= 0xF0
	b = a
	d := a ^ c.rd(hl)
	hl++
	e := c.rd(hl)
	c.wr16(ix+3, hl+1)
	c.soundPeriod(ch, uint16(d)<<8|uint16(e))
	c.soundNote(ix, ch, b>>4|b<<4)
}

// soundLoop is the $FE loop command at hl. Ported from $7AF6-$7B1F. High.
func (c *Console) soundLoop(ix uint16, ch byte, hl uint16) {
	hl++
	a := c.rd(ix+9) + 1
	var stale byte
	if a == c.rd(hl) {
		hl += 2
		c.wr(ix+9, 0)
		c.wr16(ix+3, hl+1)
		stale = 0
	} else {
		if _, f := sub8(a, c.rd(hl), false); !f.S {
			a--
		}
		c.wr(ix+9, a)
		hl++
		c.wr(ix+3, c.rd(hl))
		hl++
		c.wr(ix+4, c.rd(hl))
		stale = c.rd(hl)
	}
	c.inc(ix)
	c.soundChannel(ix, ch, stale)
}

// soundNote sets the channel volume h, reloads the note length and the
// decay counter. Ported from $7C09-$7C17 and $7C52-$7C59. High.
func (c *Console) soundNote(ix uint16, ch, h byte) {
	e := c.rd(ix + 1)
	c.wr(ix, e)
	c.wr(ix+8, c.rd(ix+0x0C)+e)
	c.soundVolume(ch, h)
}

// soundVolume writes the channel's volume register (R8-R10). Ported
// from $7C52-$7C59.
func (c *Console) soundVolume(ch, h byte) {
	c.wrtpsg(rrca(ch)+0x88, h)
}

// soundPeriod writes the channel's tone period. Ported from $7CD6-$7CE0.
func (c *Console) soundPeriod(ch byte, hl uint16) {
	c.wrtpsg(ch, byte(hl>>8))
	c.wrtpsg(ch-1, byte(hl))
}

// soundMusicFrame is a music channel between notes: the note counts
// down, and the volume decays one step per frame from the decay start
// until it reaches the sustain level. Ported from $7C2C-$7C51. High.
func (c *Console) soundMusicFrame(ix uint16, ch byte) {
	if c.dec(ix) == 0 {
		c.soundNext(ix, ch)
		return
	}
	a := c.dec(ix + 8)
	if a != c.rd(ix) {
		c.dec(ix + 8)
	} else if c.rd(ix+0x0D) < a {
		return
	}
	v := c.rd(ix+7) - 1
	if v&0x80 != 0 {
		return
	}
	c.wr(ix+7, v)
	c.soundVolume(ch, v)
}

// soundMusic reads one music note at hl. Ported from $7C5A-$7CD5. High.
//
// Music stream (prefixes optional, in this order):
//
//	$Dx           length unit x
//	$Fv cc dd     volume v, decay start cc, sustain level dd
//	$E8-$EF       detune flag (low 3 bits), then read again from $Dx
//	$E0-$E7       octave shift
//	nl            note n (0-9 from the table at $7CE9; 12 is a rest),
//	              length (l+1) units
func (c *Console) soundMusic(ix uint16, ch byte, hl uint16) {
	var a byte
	for {
		a = c.rd(hl)
		if a&0xF0 == 0xD0 {
			c.wr(ix+0x0A, a&0x0F)
			hl++
			a = c.rd(hl)
		}
		if a >= 0xF0 {
			c.wr(ix+6, a&0x0F)
			hl++
			c.wr(ix+0x0C, c.rd(hl))
			hl++
			c.wr(ix+0x0D, c.rd(hl))
			hl++
			a = c.rd(hl)
		}
		if a >= 0xE0 {
			a &= 0x0F
			if a&0x08 != 0 {
				c.wr(ix+0x0B, a)
				hl++
				continue
			}
			c.wr(ix+5, a)
			hl++
			a = c.rd(hl)
		}
		break
	}
	b := a & 0x0F
	a = c.rd(ix + 0x0A)
	if b != 0 {
		for ; b > 0; b-- {
			a += c.rd(ix + 0x0A)
		}
	}
	c.wr(ix+1, a)
	a = c.rd(hl)
	c.wr16(ix+3, hl+1)
	note := a >> 4
	vol := byte(0)
	if note != 0x0C {
		vol = c.rd(ix + 6)
	}
	c.wr(ix+7, vol)
	c.soundNote(ix, ch, vol)
	p := uint16(c.rd(0x7CE9 + uint16(note)))
	for s := c.rd(ix + 5); s > 0; s-- {
		p += p
	}
	if c.rd(ix+0x0B) != 0 {
		p++
	}
	c.soundPeriod(ch, p)
}
