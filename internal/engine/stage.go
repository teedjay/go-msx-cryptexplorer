package engine

// Building a pyramid: graphics, map and object lists.
//
// RAM: $E054 the pyramid (1-15), $E055 the pyramid to enter next, $E056
// the door to enter by (x2), $E057 the door left by; $E058 the loop
// count; $E062/$E063 a bit per pyramid cleared.

// stageSetup clears the pyramid RAM, loads the pyramid graphics and the
// player's sprites, then builds the pyramid. Ported from $432A-$432F,
// $4FEE-$5055 (falling into $5056) and $6AA3. High.
func (c *Console) stageSetup() {
	for a := uint16(0xE130); a <= 0xE130+0x500; a++ {
		c.wr(a, 0)
	}
	c.rle3(0x2200, 0x5638)
	c.rle3(0x0228, 0x5779)
	c.mirrorTiles(0x2340, 0x23B8, 0x0F)
	c.rle3(0x03B8, 0x57DD)
	hl := uint16(0x2430)
	for b := 0; b < 6; b++ {
		c.rle3(hl, 0x576F)
		hl += 8
	}
	c.rle3(0x0430, 0x57F2)
	c.rleAddr(0x5596)
	c.rleAddr(0x5536)
	c.mirrorSprites(0x1940, 0x1C50, 3)
	c.playerPatterns()
	c.stageBuild()
}

// mapFill fills the map from row 1 with $14 (and one byte past it, at
// $F000). Ported from $6D9A-$6DA7. High.
func (c *Console) mapFill() {
	c.wr(0xE760, 0x14)
	c.ldir(0xE761, 0xE760, 0x8A0)
}

// loadWallTiles loads the brick colours for the pyramid's group of four
// (the same 8-byte stream into tiles $40-$44 of each third). Ported from
// $6DA8-$6DCB. High.
func (c *Console) loadWallTiles() {
	g := (c.rd(0xE054) - 1) >> 2 & 3
	de := 0x6DCC + uint16(g*9)
	hl := uint16(0x0200)
	for b := 0; b < 5; b++ {
		c.rle3(hl, de)
		hl += 8
	}
}

// stageBuild builds pyramid $E055 from its data ($6D70): up to four
// 16-column sections of bit-packed brick, then the doors, enemies,
// jewels, daggers, picks, sliding walls, traps and stairs. Ported from
// $6AA3-$6CCA. High.
func (c *Console) stageBuild() {
	c.wr(0xE05E, 0)
	c.wr(0xE05B, 0)
	c.mapFill()
	c.wr(0xE053, 1) // $6AAD: INC A, A still 0 from the XOR A
	c.spritesOff(0xE0B0, 0x20)
	c.wr(0xE054, c.rd(0xE055))
	c.loadWallTiles()
	de := c.rd16(0x6D70 + uint16((c.rd(0xE054)-1)*2))
	ix := uint16(0xE700)
	for b := 4; b > 0; b-- {
		a := c.rd(de)
		tmpl := c.rd16(0x6D8E + uint16(a>>3&0x1E))
		hl := tmpl + uint16(a&0x0F)*44
		dst := ix
		for row := 0; row < 0x16; row++ {
			for k := 0; k < 2; k++ {
				bits := c.rd(hl)
				for i := 0; i < 8; i++ {
					v := byte(0)
					if bits&0x80 != 0 {
						v = 0x12
					}
					bits <<= 1
					c.wr(dst, v)
					dst++
				}
				hl++
			}
			dst += 0x50
		}
		ix += 0x10
		a = c.rd(de)
		de++
		if a&0xF0 == 0x30 {
			break
		}
	}
	hl := de
	// Doors.
	c.wr(0xE1C4, 4)
	d := uint16(0xE1C5)
	for n := byte(0); n < 4; n++ {
		if c.rd(hl) == 0xFF {
			c.wr(d, 0xFF)
			hl++
			d += 7
			continue
		}
		d = c.copy4(d, hl)
		hl += 2
		e := c.rd(0xE056) >> 1
		if e == 4 {
			e--
		}
		a := byte(0x10)
		if e != n {
			a = 0
			if c.pyramidCleared() {
				a = 0x80
			}
		}
		c.wr(d, a)
		v := c.rd(hl)
		c.wr(d+1, v>>4)
		c.wr(d+2, v&0x0F)
		d += 3
		hl++
	}
	// Enemies: Y, X|screen, type.
	n := c.rd(hl)
	c.wr(0xE164, n)
	hl++
	c.ldir(0xE14E, hl, uint16(n*3))
	hl += uint16(n * 3)
	// Jewels.
	n = c.rd(hl)
	c.wr(0xE1F3, n)
	hl++
	d = 0xE1F5
	for b := n; ; {
		c.wr(d, c.rd(hl)&0xF0|1)
		hl++
		c.wr(d+1, 1)
		d = c.copy4(d+2, hl)
		hl += 2
		d += 3
		b--
		if b == 0 {
			break
		}
	}
	c.wr(0xE1F4, 0)
	for {
		j := c.jewel(0)
		if c.pyramidCleared() {
			c.wr(j, c.rd(j)&0xF0)
			c.wr(j+1, 3)
		} else {
			_, cell := c.mapAt(j + 2)
			c.mapStamp(cell, 0x6D96)
			c.wr(cell, c.rd(c.jewel(0))>>4+0x40)
		}
		if !c.nextJewel() {
			break
		}
	}
	// Daggers.
	n = c.rd(hl)
	c.wr(0xE263, n)
	hl++
	d = 0xE264
	for b := n; b > 0; b-- {
		d = c.copy4(d+2, hl)
		hl += 2
		d += 0x0B
	}
	// Picks.
	n = c.rd(hl)
	c.wr(0xE2CB, n)
	hl++
	if n != 0 {
		d = 0xE2CC
		for b := n; b > 0; b-- {
			c.wr(d, 1)
			d = c.copy4(d+1, hl)
			hl += 2
		}
		p := uint16(0xE2CD)
		for b := n; b > 0; b-- {
			_, cell := c.mapAt(p)
			c.wr(cell, 0x80)
			p += 5
		}
	}
	// Sliding walls.
	n = c.rd(hl)
	hl++
	c.wr(0xE31D, n)
	if n != 0 {
		d = 0xE31E
		for b := n; b > 0; b-- {
			c.ldir(d, hl, 2)
			hl += 2
			v := c.rd(hl)
			c.wr(d+3, v&0xF8)
			c.wr(d+4, v>>2&1)
			c.wr(d+5, v<<2&0x0C)
			hl++
			d += 7
		}
		ix := uint16(0xE31E)
		for {
			c.wallMap(ix)
			ix += 7
			if c.inc(0xE31C) == c.rd(0xE31D) {
				break
			}
		}
	}
	// Traps.
	n = c.rd(hl)
	c.wr(0xE3FE, n)
	hl++
	d = 0xE3FF
	for b := n; b > 0; b-- {
		d = c.copy4(d+1, hl)
		hl += 2
	}
	// Stairs: from each start, up diagonally through empty cells.
	n = c.rd(hl)
	hl++
	for ; n > 0; n-- {
		c.wr(0xE083, c.rd(hl))
		c.wr(0xE084, 0)
		v := c.rd(hl + 1)
		c.wr(0xE085, v&0xF8)
		dir := v & 1
		c.wr(0xE086, v>>1&1)
		_, cell := c.mapAt(0xE083)
		for c.rd(cell) == 0 {
			cell = c.stairPair(cell, 0x20, dir)
			if dir == 0 {
				cell -= 0x62
			} else {
				cell -= 0x60
			}
		}
		c.stairPair(cell, 0x15, dir)
		hl += 2
	}
}

// stairPair writes a stair's two cells (cc, cc+1; +2 going the other
// way) and returns the second cell. Ported from $6CC0-$6CCA. High.
func (c *Console) stairPair(cell uint16, cc, dir byte) uint16 {
	if dir != 0 {
		cc += 2
	}
	c.wr(cell, cc)
	c.wr(cell+1, cc+1)
	return cell + 1
}

// playerAppear shows the pyramid: the map drawn for the player's screen,
// the sprites, the daggers placed and every enemy started. Ported from
// $6CCB-$6CF7. High.
func (c *Console) playerAppear() {
	c.wr(0xE147, 1)
	c.drawMap(c.screenMap())
	c.drawSprites()
	c.daggersUpdate()
	c.wr(0xE165, 0)
	for {
		c.enemyInit()
		if c.inc(0xE165) == c.rd(0xE164) {
			return
		}
	}
}

// playerEnter puts the player in the entry door ($E056 / 2, 4 -> 3):
// door sprites, position, walk speed, and that door opening (state 5).
// Ported from $6ECB-$6F4E. High.
func (c *Console) playerEnter() {
	c.ldir(0xE0B0, 0x6F54, 0x10)
	c.wr(0xE0C3, 0x0E)
	c.wr(0xE0C7, 0x06)
	e := c.rd(0xE056) >> 1
	if e == 4 {
		e--
	}
	hl := 0xE1C5 + uint16(e*7)
	c.wr(0xE137, c.rd(hl)-8)
	c.wr(0xE138, c.rd(hl+1))
	c.wr(0xE139, c.rd(hl+2)+8)
	c.wr(0xE13A, c.rd(hl+3))
	c.ldir(0xE13B, 0x6F4F, 5)
	c.playerSprite()
	c.wr(0xE1F4, e)
	c.wr(c.doorAddr(4), 0x50)
	c.doorSprites(c.rd(hl), c.rd(hl+2))
	c.wr(0xE130, 0)
	c.wr(0xE136, 1)
}

// drawSprites copies the sprite table from RAM to VRAM $3B00: the first
// 6 entries as they are, the next 16 rotated by the frame ($E061) so
// that more than four sprites on a line flicker instead of vanishing,
// then the rest. Ported from $4B75-$4BBB. High.
func (c *Console) drawSprites() {
	c.vcopy(0x3B00, 0xE0B0, 0x18)
	r := c.inc(0xE061) & 3
	de := 0xE0C8 + uint16(r*4)
	hl := uint16(0x3B18)
	for b := 0; b < 4; b++ {
		c.vcopy(hl, de, 4)
		de += 4
		hl += 4
		r++
		if r == 4 {
			de = 0xE0C8
			r = 0
		}
	}
	c.vcopy(0x3B28, 0xE0D8, 0x58)
}
