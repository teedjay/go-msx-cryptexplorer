package engine

// The pyramid map and the probes into it.
//
// The map is 96 columns (three screens) by 24 rows of one byte per 8x8
// cell, at $E700-$EFFF (row r at $E700 + $60*r). The high nibble is the
// cell's class (High from the probes below; meanings Medium):
//
//	$00 empty            $40 jewel / pick-up?   $80 hidden door cell
//	$10 solid brick      $50 stair (up-right)   $12 brick placed by the map builder
//	$20 stair top?       $14 the map's fill byte
//	$30 door?
//
// Objects keep a position as Y (one byte) at p and X (16-bit, pixels
// across the three screens) at p+2, p+3.

// mapCell returns the map cell under the object at p, offset by c pixels
// down and by the signed b pixels across, and its address. Ported from
// $50E1-$511A. High.
func (c *Console) mapCell(p uint16, b, cc byte) (byte, uint16) {
	c.Cost[6]++
	row := uint16((c.rd(p)+cc)>>3) & 0x1F
	x := c.rd16(p + 2)
	x += uint16(int16(int8(b)))
	col := x >> 3
	hl := col + row*0x60 + 0xE700
	return c.rd(hl), hl
}

// mapAt is the cell under the object at p. Ported from $50D6-$50E0.
func (c *Console) mapAt(p uint16) (byte, uint16) { return c.mapCell(p, 0, 0) }

// probe is $50B7: the cell class at (b across, c down) from the object
// at p, with the object's state byte (p-3) deciding the test. It returns
// the class (A), the cell address (DE) and Z: set for brick ($10), and,
// unless the state byte is 2, for a stair ($50). Ported from
// $50B7-$50D5. High.
func (c *Console) probe(p uint16, b, cc byte) (cls byte, de uint16, z bool) {
	a, hl := c.mapCell(p, b, cc)
	cls = a & 0xF0
	if cls == 0x10 {
		return cls, hl, true
	}
	if c.rd(p-3) == 2 {
		return cls, hl, false // DEC B from 2: NZ
	}
	return cls, hl, cls == 0x50
}

// standing is $50AD: Z when the object at p has brick (or, unless
// falling, a stair) under one of its feet (6 or 10 pixels across, 17
// down). Ported from $50AD-$50B6. High.
func (c *Console) standing(p uint16) bool {
	if _, _, z := c.probe(p, 0x06, 0x11); z {
		return true
	}
	_, _, z := c.probe(p, 0x0A, 0x11)
	return z
}

// fallCheck is $5098: when the object at p stands on nothing it snaps
// its X (p+2) to the nearest 4-pixel step and reports true (carry).
// Ported from $5098-$50AC. High.
func (c *Console) fallCheck(p uint16) bool {
	if c.standing(p) {
		return false
	}
	p += 2
	a := c.rd(p)
	if a&7 < 4 {
		a += 4
	}
	c.wr(p, a&0xFC)
	return true
}

// wallAhead is $507E: Z when there is brick 15 pixels down at 5 pixels
// across (facing right, bit 0 of p-1 set) or 11 (facing left); otherwise
// carry. Returns z. Ported from $507E-$5094. High.
func (c *Console) wallAhead(p uint16) bool {
	b := byte(0x05)
	if c.rd(p-1)&1 == 0 {
		b = 0x0B
	}
	a, _ := c.mapCell(p, b, 0x0F)
	return a&0xF0 == 0x10
}

// onGround is $511D: for an object at p with Y on an 8-pixel boundary,
// Z when the cell 16 down and 8 across is brick or empty; NZ when Y is
// not aligned. Ported from $511D-$512F. High.
func (c *Console) onGround(p uint16) bool {
	if c.rd(p)&7 != 0 {
		return false
	}
	a, _ := c.mapCell(p, 0x08, 0x10)
	a &= 0xF0
	return a == 0x10 || a == 0
}

// nameAddr is the name-table address of pixel (d = Y, e = X). Ported
// from $5B28-$5B3E. High.
func nameAddr(d, e byte) uint16 {
	return 0x3800 + uint16(d>>3)*0x20 + uint16(e>>3)
}

// mapTile is the name-table tile for map cell a: a 16-entry row per
// class from the pointer table at $5D8B. Ported from $5D75-$5D8A. High.
func (c *Console) mapTile(a byte) byte {
	c.Cost[5]++
	p := c.rd16(0x5D8B + uint16(a>>4)*2)
	return c.rd(p + uint16(a&0x0F))
}

// drawMap draws 22 rows of 32 cells from the map at de (the left edge of
// the visible screen, row 2) to the name table from $3820. Ported from
// $5D54-$5D74. High.
func (c *Console) drawMap(de uint16) {
	c.setwrt(0x3820)
	for b := 0; b < 0x16; b++ {
		p := de
		for x := 0; x < 0x20; x++ {
			c.out98(c.mapTile(c.rd(p)))
			p++
		}
		de += 0x60
	}
}

// screenMap is the map address of the visible screen's row 2 for
// screen $E13A: $E760 + $20*screen. Ported from $6CD0-$6CDB.
func (c *Console) screenMap() uint16 { return 0xE760 + uint16(c.rd(0xE13A)*0x20) }
