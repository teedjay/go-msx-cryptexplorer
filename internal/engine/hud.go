package engine

// Score, hi-score and the numbers on screen.
//
// RAM (High): score $E049-$E04B and hi-score $E043-$E045, packed BCD,
// low byte first. $E052 is the next extra-life threshold (BCD, in units
// of the score's high byte).

// addScore adds the BCD value de to the score (only in a game: $E002 bit
// 6), awards an extra life at each threshold, updates the hi-score and
// redraws. Ported from $4412-$4479. High.
func (c *Console) addScore(de uint16) {
	if c.rd(0xE002)&0x40 == 0 {
		return
	}
	e, d := byte(de), byte(de>>8)
	var cy bool
	e, cy = bcdAdd(c.rd(0xE049), e, false)
	c.wr(0xE049, e)
	d, cy = bcdAdd(c.rd(0xE04A), d, cy)
	c.wr(0xE04A, d)
	de = uint16(d)<<8 | uint16(e)
	if cy {
		v, cy2 := bcdAdd(c.rd(0xE04B), 1, false)
		c.wr(0xE04B, v)
		if cy2 {
			// The score overflowed: the hi-score becomes 999999.
			c.wr16(0xE043, 0x9999)
			c.wr16(0xE044, 0x9999)
			c.drawHiScore()
			c.drawScore()
			return
		}
		if t := c.rd(0xE052); t < v {
			t, cy3 := bcdAdd(t, 2, false)
			if cy3 {
				t = 0xFF
			}
			c.wr(0xE052, t)
			c.extraLife()
		}
	}
	// $4453: compare with the hi-score.
	hi := c.rd(0xE045)
	b := c.rd(0xE04B)
	switch {
	case hi < b:
	case hi > b:
		c.drawScore()
		return
	default:
		if c.rd16(0xE043) >= de {
			c.drawScore()
			return
		}
	}
	c.wr16(0xE043, de)
	c.wr(0xE045, b)
	c.drawHiScore()
	c.drawScore()
}

// drawHiScore prints the hi-score at $3811. Ported from $446B-$4473.
func (c *Console) drawHiScore() { c.printBCD(0x3811, 0xE045, 3) }

// drawScore prints the score at $3807. Ported from $4474-$4479.
func (c *Console) drawScore() { c.printBCD(0x3807, 0xE04B, 3) }

// printBCD prints b BCD bytes from de downwards as digits (tile = digit
// + $10) at VRAM hl. Ported from $447A-$4493. High.
func (c *Console) printBCD(hl, de uint16, b int) {
	for ; b > 0; b-- {
		hl = c.printBCDByte(hl, c.rd(de))
		de--
	}
}

// printBCDByte prints the two digits of a at hl and returns hl + 2.
// Ported from $447D-$448F (one pass). High.
func (c *Console) printBCDByte(hl uint16, a byte) uint16 {
	c.wrtvrm(hl, a>>4+0x10)
	hl++
	c.wrtvrm(hl, a&0x0F+0x10)
	return hl + 1
}

// toBCD converts a (mod 100) to packed BCD. Ported from $4393-$43AA. High.
func toBCD(a byte) byte {
	b := a
	for {
		v, f := sub8(a, 0x64, false)
		if f.C {
			break
		}
		a = v
		b = a
	}
	cc := byte(0)
	for {
		v, f := sub8(b, 0x0A, false)
		if f.C {
			break
		}
		cc += 0x10
		b = v
		if f.Z {
			break
		}
	}
	return cc | b
}

// drawRest prints the lives ($E050) at $381D. Ported from $4385-$4392.
// High.
func (c *Console) drawRest() {
	c.printBCDByte(0x381D, toBCD(c.rd(0xE050)))
}

// drawPyramid prints the "PYRAMID-" text and the pyramid number,
// 15 x $E058 + $E054, at $3AF3. Ported from $436B-$4384. High.
func (c *Console) drawPyramid() {
	c.text(0x480E, 0xFF)
	a := c.rd(0xE058)
	a = a*16 - a + c.rd(0xE054)
	c.printBCDByte(0x3AF3, toBCD(a))
}

// drawHUD draws the pyramid number, then the score line. Ported from
// $4404-$4406, falling into $4407.
func (c *Console) drawHUD() {
	c.drawPyramid()
	c.drawScoreLine()
}

// drawScoreLine prints "SCORE- HI- REST-" with the lives, hi-score and
// score. Ported from $4407-$4411. High.
func (c *Console) drawScoreLine() {
	c.text(0x47AA, 0xFF)
	c.drawRest()
	c.drawHiScore()
	c.drawScore()
}
