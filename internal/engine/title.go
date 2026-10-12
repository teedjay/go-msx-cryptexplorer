package engine

// The logo and the title screen.

// loadGraphics loads the logo, then the font and the common tiles, into
// all three pattern/colour thirds. Ported from $42CC-$4309. High.
func (c *Console) loadGraphics() {
	c.loadLogo()
	c.loadFont()
	c.rle3(0x0008, 0x47A7)
	c.loadTiles()
}

// loadTiles loads the shared tile patterns and colours. Ported from
// $42DB-$4309. High.
func (c *Console) loadTiles() {
	c.rle3(0x2480, 0x490B)
	c.rle3(0x0480, 0x4A97)
	hl := uint16(0x44D8) // a VRAM address: $04D8 once masked
	for b := 0; b < 0x16; b++ {
		c.rle3(hl, 0x4AAB)
		hl += 0x10
	}
	c.vfill3(hl, 0x10, 0x40)
}

// loadFont loads the font patterns and gives them colour $F0. Ported
// from $44CF-$44DF. High.
func (c *Console) loadFont() {
	c.rle3(0x2080, 0x467D)
	c.vfill3(0x0080, 0x180, 0xF0)
}

// loadLogo loads the logo's patterns and colours and starts its scroll
// at name-table $3AAA, 14 rows. Ported from $4823-$4841. High.
func (c *Console) loadLogo() {
	c.wr(0xE00A, 0x0E)
	c.wr16(0xE00E, 0x3AAA)
	c.rle3(0x6300, 0x4874)
	c.vfill3(0x0300, 0xD8, 0xF0)
}

// logoScroll draws the logo one row higher (3 + 11 + 12 tiles from $60,
// then 12 blanks below) and reports true when the scroll is done
// ($E00A reaches 0). Ported from $4842-$4873. High.
func (c *Console) logoScroll() bool {
	hl := c.rd16(0xE00E) - 0x20
	c.wr16(0xE00E, hl)
	a := byte(0x60)
	hl, a = c.logoRow(hl, a, 3)
	hl, a = c.logoRow(hl, a, 0x0B)
	hl, _ = c.logoRow(hl, a, 0x0C)
	c.vfill(hl, 0x0C, 0)
	return c.dec(0xE00A) == 0
}

// logoRow writes b consecutive tiles from a at hl, and returns the
// next row's start and the next tile. Ported from $4866-$4873. High.
func (c *Console) logoRow(hl uint16, a byte, b int) (uint16, byte) {
	de := hl
	for ; b > 0; b-- {
		c.wrtvrm(hl, a)
		hl++
		a++
	}
	return de + 0x20, a
}

// drawTitle reloads the tiles and draws the title letters at once.
// Ported from $43AB-$43B7. High.
func (c *Console) drawTitle() {
	c.loadTiles()
	c.wr(0xE00A, 0)
	for c.titleLetter() {
	}
}

// titleLetter draws the next title letter (two tiles high, $E00A
// counts them: 9 on the first line at $38A7, the rest at $3904), and
// after the last one the texts. It reports whether there are more.
// Ported from $43B8-$4403. High.
func (c *Console) titleLetter() bool {
	a := c.rd(0xE00A)
	c.inc(0xE00A)
	if a >= 0x16 {
		c.text(0x47C1, 0xFF)
		c.vblock(0x3892, 0x4ABC, 3, 6)
		return false
	}
	hl := uint16(0x38A7)
	if a >= 9 {
		hl = 0x3904
	}
	cc := a
	hl = hl&0xFF00 | uint16(byte(hl)+a)
	cc = cc*2 + 0x9B
	for b := 0; b < 2; b++ {
		c.wrtvrm(hl, cc)
		hl += 0x20
		cc++
	}
	if v := byte(hl) - 0xEC; v < 2 {
		c.wrtvrm(hl, v+0xC7)
	}
	return true
}

// vblock copies b rows of n bytes from de to VRAM hl, one name-table
// row ($20) apart. Ported from $65C8-$65D6. High.
func (c *Console) vblock(hl, de uint16, b int, n byte) uint16 {
	for ; b > 0; b-- {
		de = c.vcopy(hl, de, uint16(n))
		hl += 0x20
	}
	return de
}

// flashExits recolours tiles from the tables at $4C09 and $4C0D by the
// frame counter (an animation; role Low) and returns the last fill
// byte, which a caller uses as a stale A. Ported from $4BE4-$4C08. High.
func (c *Console) flashExits() byte {
	i := uint16(c.rd(0xE003)>>1) & 3
	c.vfill3(0x0288, 0x18, c.rd(0x4C09+i))
	a := c.rd(0x4C0D + i)
	c.vfill3(0x02E5, 3, a)
	return a
}
