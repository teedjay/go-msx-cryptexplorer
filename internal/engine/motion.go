package engine

// Motion shared by the player and the enemies. An object block is laid
// out like the player's from $E134: state, input, facing, Y, (spare),
// X lo, X hi (screen), then per-object fields.

// move adds the 24-bit velocity (de, with its high byte at hl+5) to the
// 24-bit position at hl (fraction, X lo, X hi = screen), negated when
// facing left (bit 0 of the facing byte at hl-2: the high byte is only
// complemented). Ported from $7402-$7433. High.
func (c *Console) move(hl, de uint16) {
	pos := c.rd16(hl)
	b := c.rd(hl + 2)
	cc := c.rd(hl + 5)
	if c.rd(hl-2)&1 != 0 {
		cc = ^cc
		de = ^de + 1
	}
	sum := uint32(pos) + uint32(de)
	carry := byte(0)
	if sum > 0xFFFF {
		carry = 1
	}
	c.wr16(hl, uint16(sum))
	c.wr(hl+2, b+cc+carry)
}

// stairsStart is $5133: with up (bit 0) or down (bit 1) in the input at
// hl, step onto a stair if one starts here, setting the state (hl-1) to 3
// and the stair parity $E143 (unless $E14D: an enemy). Z (true) when it
// did. Ported from $5133-$51D1. High.
func (c *Console) stairsStart(hl uint16) bool {
	a := c.rd(hl) & 3
	if a == 0 {
		return false
	}
	if a&1 == 0 {
		return c.stairsDown(hl)
	}
	x := c.rd(hl+4) & 7
	if x == 0 {
		return false
	}
	b := byte(0x22)
	if x >= 5 {
		b = 0x21
	}
	y := hl + 2
	_, cell := c.mapAt(y)
	v := c.rd(cell + 0x61)
	if v != b {
		b += 0x10
		if v != b {
			return false
		}
	}
	if c.rd(0xE14D) == 0 {
		c.wr(0xE143, v&1^1)
	}
	c.wr(y, c.rd(0x518D+uint16(c.rd(y+2)&7))+c.rd(y))
	c.wr(y-3, 3)
	return true
}

// stairsDown is $5198: down onto a stair ($16/$17 two rows down, one
// across) when X is at the 4-pixel point. Ported from $5198-$51D1. High.
func (c *Console) stairsDown(hl uint16) bool {
	if c.rd(hl+4)&7 != 4 {
		return false
	}
	y := hl + 2
	_, cell := c.mapAt(y)
	v := c.rd(cell + 0xC1)
	if v != 0x16 && v != 0x17 {
		return false
	}
	if c.rd(0xE14D) == 0 {
		c.wr(0xE143, v&1)
	}
	c.wr(y, c.rd(y)+4)
	c.wr(y-3, 3)
	return true
}

// wallCheck is $51DB: is the way blocked for the object whose facing
// byte is at hl, facing b? It looks at the cell ahead at head height
// and one row down: brick ($10) when X is at the 4-pixel point, a stair
// ($50) when X is on the 8-pixel boundary; other X: not blocked. It
// returns carry (blocked) and the cell address it stopped at. Ported
// from $51DB-$520D. High.
func (c *Console) wallCheck(hl uint16, b byte) (bool, uint16) {
	hl++
	de := hl
	_, cell := c.mapAt(hl)
	x := c.rd(de+2) & 7
	cls := byte(0x50)
	if x != 0 {
		if x != 4 {
			return false, cell
		}
		cls = 0x10
	}
	if b&1 == 0 {
		cell++
		if x != 0 {
			cell++
		}
	}
	if c.rd(cell)&0xF0 == cls {
		return true, cell
	}
	cell += 0x60
	return c.rd(cell)&0xF0 == cls, cell
}

// wallCheckInput is $51D5: no move (NC) without left or right in the
// input at hl, else wallCheck with the facing at hl+1.
func (c *Console) wallCheckInput(hl uint16) (bool, uint16) {
	if c.rd(hl)&0x0C == 0 {
		return false, hl
	}
	return c.wallCheck(hl+1, c.rd(hl+1))
}

// headRoom is $7434: can the object at p (Y) jump? Carry (true) when the
// cells above are free; de is the last cell probed. With bit 4 (the
// trigger) as the whole input at p-2 only the straight-up test runs.
// Ported from $7434-$7449. High.
func (c *Console) headRoom(p uint16) (bool, uint16) {
	if c.rd(p-2) == 0x10 {
		return c.headUp(p)
	}
	if free, de := c.headUp(p); !free {
		return false, de
	}
	b := byte(0x03)
	if c.rd(p-1)&1 == 0 {
		b = 0x0C
	}
	return c.probeAheadDE(p, b, 0x04)
}

// headUp is $746A: carry (true) when neither cell 2 pixels up (at 4 and
// 10 across) is solid. Ported from $746A-$7479. High.
func (c *Console) headUp(p uint16) (bool, uint16) {
	if _, de, z := c.probe(p, 0x04, 0xFE); z {
		return false, de
	}
	_, de, z := c.probe(p, 0x0A, 0xFE)
	return !z, de
}

// probeAhead is $744D: carry (true) when the column at b across is
// clear from c down: if the cell at c is solid, the three cells 8, 16
// and (at 8 across) 16 pixels higher must be free; otherwise only the
// one 8 higher. Ported from $744D-$7483. High.
func (c *Console) probeAhead(p uint16, b, cc byte) bool {
	ok, _ := c.probeAheadDE(p, b, cc)
	return ok
}

func (c *Console) probeAheadDE(p uint16, b, cc byte) (bool, uint16) {
	_, de, z := c.probe(p, b, cc)
	if !z {
		cc -= 8
		_, de, z = c.probe(p, b, cc)
		return !z, de
	}
	cc -= 8
	if _, de, z = c.probe(p, b, cc); z {
		return false, de
	}
	cc -= 8
	if _, de, z = c.probe(p, b, cc); z {
		return false, de
	}
	_, de, z = c.probe(p, 0x08, cc)
	return !z, de
}

// landed is $7484/$748A for the object whose state byte is at ix (the
// player, or an enemy): during a jump, has it come down on something?
// z is the flag the caller enters $748A with ($E143 = $10 for the
// player: a straight-up jump skips the side tests). On landing, Y is
// snapped to 8 pixels and it returns true (Z). Ported from $7484-$74F3.
// High.
func (c *Console) landed(ix uint16, z bool) bool {
	hl := ix + 2
	d := c.rd(hl)
	hl++ // Y
	side := func(cc byte) bool {
		b := byte(0x03)
		if d&1 == 0 {
			b = 0x0C
		}
		_, _, z := c.probe(hl, b, cc)
		return z
	}
	snap := func() bool {
		c.wr(hl, c.rd(hl)&0xF8)
		return true
	}
	if !z && c.rd(ix+5)&7 == 4 {
		if side(0x00) {
			if side(0x14) {
				return snap()
			}
		}
		if side(0x08) || side(0x0E) {
			return snap()
		}
	}
	if c.rd(ix+0x0E) == 0 {
		return false
	}
	if c.wallAhead(hl) {
		return snap()
	}
	if c.standing(hl) {
		return snap()
	}
	return false
}

// jumpStep moves a jumping object (facing byte at hl): X by one pixel
// in the jump's direction (from the input at hl-1), Y by the next entry
// of the jump table (up while rising, then back down the table, then a
// steady 4 pixels a frame after its $FF start). In the ending
// ($E000 = 10) with $E004 = 2 the steps are four times larger; anywhere
// else $E004 is cleared. Ported from $74F4-$7555. High.
func (c *Console) jumpStep(hl uint16) {
	if c.rd(0xE000) != 0x0A {
		c.wr(0xE004, 0)
	}
	ix := hl
	de := c.rd16(hl + 0x0A)
	b := c.rd(hl + 0x0C)
	in := c.rd(hl - 1)
	xp := hl + 3
	if in&0x0C != 0 {
		c.dec(xp)
		if in&0x04 == 0 {
			c.inc(xp)
			c.inc(xp)
		}
	}
	yp := xp - 2
	a := c.rd(de)
	if a == 0xFF {
		c.wr(yp, c.rd(yp)+4)
		return
	}
	if c.rd(0xE004) == 2 {
		a *= 4
	}
	if b == 0 {
		a = -a
	}
	c.wr(yp, c.rd(yp)+a)
	de++
	if b != 0 {
		de -= 2
	}
	for {
		c.wr16(ix+0x0A, de)
		if c.rd(de) != 0xFE {
			return
		}
		c.wr(ix+0x0C, 1)
		de--
	}
}

// fallStep is $7564: the object whose state byte is at hl falls 4
// pixels a frame (state 2, Y kept on a 4-pixel step) while there is
// nothing under it. It reports true (carry) while still falling. Ported
// from $7564-$757C. High.
func (c *Console) fallStep(hl uint16) bool {
	c.wr(hl, 2)
	y := hl + 3
	c.wr(y, c.rd(y)&0xFC)
	if !c.fallCheck(y) {
		return false
	}
	c.wr(y, (c.rd(y)+4)&0xFC)
	return true
}

// stairStep is $757D: one step along a stair for the object whose input
// byte is at hl, on frames where $E003 & b is 0: X by one pixel in the
// facing direction, Y by one up or down by the stair parity ($E143 for
// the player, the object's own field for an enemy, at hl+$0E). It
// reports true (Z) when the object has stepped off the stair. Ported
// from $757D-$75BC. High.
func (c *Console) stairStep(hl uint16, b byte) bool {
	if c.rd(0xE003)&b != 0 {
		return false
	}
	hl++
	f := c.rd(hl)
	hl += 3
	a := c.rd(hl)
	hl -= 2 // Y
	if a&3 == 0 {
		if c.onGround(hl) {
			return true
		}
	}
	hl += 2 // X
	c.inc(hl)
	if f&1 != 0 {
		c.dec(hl)
		c.dec(hl)
	}
	par := c.rd(hl + 0x0A)
	c.inc(hl + 5)
	a = par + f
	d := byte(1)
	if a&1 != 0 {
		d = 0xFF
	}
	hl -= 2
	c.wr(hl, c.rd(hl)+d)
	return false
}
