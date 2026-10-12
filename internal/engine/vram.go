package engine

// VRAM helpers. The game writes VRAM through OUT (C),A with C from the
// BIOS byte at $0006 (VDPDR, $98 on C-BIOS), after a SETWRT; and through
// the BIOS WRTVRM/RDVRM.

// setwrt is $4536: SETWRT(HL) and C' = the VDP data port. High.
func (c *Console) setwrt(hl uint16) { c.setAddr(hl) }

// vfill writes bc bytes of a from VRAM hl. Like the original's DEC BC /
// test loop, bc = 0 writes 65,536 bytes. Ported from $44AB-$44BB. High.
func (c *Console) vfill(hl, bc uint16, a byte) {
	c.setwrt(hl)
	c.fillRun(bc, a)
}

// fillRun is the OUT loop of $44AE-$44BB.
func (c *Console) fillRun(bc uint16, a byte) {
	for {
		c.Cost[2]++
		c.out98(a)
		bc--
		if bc == 0 {
			return
		}
	}
}

// vcopy copies bc bytes from CPU address de to VRAM hl and returns the
// advanced de. Ported from $44C0-$44CE. High.
func (c *Console) vcopy(hl, de, bc uint16) uint16 {
	c.setwrt(hl)
	return c.copyRun(de, bc)
}

// copyRun is the OUT loop of $44C3-$44CE.
func (c *Console) copyRun(de, bc uint16) uint16 {
	for {
		c.out98(c.rd(de))
		de++
		bc--
		if bc == 0 {
			return de
		}
	}
}

// vfill3 fills bc bytes of a at hl in each of the three Screen 2 thirds
// ($800 apart) and returns hl + $1800. Ported from $44E0-$44F0. High.
func (c *Console) vfill3(hl, bc uint16, a byte) uint16 {
	for d := 0; d < 3; d++ {
		c.vfill(hl, bc, a)
		hl += 0x800
	}
	return hl
}

// rle3 decodes the RLE stream at de to VRAM hl in each of the three
// thirds (DE is restored, so the same stream is decoded three times) and
// returns hl + $1800. Ported from $44F1-$4500. High.
func (c *Console) rle3(hl, de uint16) uint16 {
	for b := 0; b < 3; b++ {
		c.rle(hl, de)
		hl += 0x800
	}
	return hl
}

// rleAddr decodes the RLE stream at de that starts with its VRAM
// address. Ported from $4514-$4519. High.
func (c *Console) rleAddr(de uint16) uint16 {
	hl := c.rd16(de)
	return c.rle(hl, de+2)
}

// rle decodes the RLE stream at de to VRAM hl and returns the stream
// pointer after its end byte. Format (see internal/streams): 0 end, $80
// new VRAM address word, $01-$7F repeat the next byte n times, $81-$FF
// n&$7F literal bytes. Ported from $451A-$4535. High.
func (c *Console) rle(hl, de uint16) uint16 {
	c.setwrt(hl)
	for {
		ctrl := c.rd(de)
		de++
		n := ctrl & 0x7F
		if n == 0 {
			if ctrl == 0 {
				return de
			}
			hl = c.rd16(de)
			de += 2
			c.setwrt(hl)
			continue
		}
		c.Cost[4] += int(n)
		if ctrl&0x80 == 0 {
			a := c.rd(de)
			de++
			c.fillRun(uint16(n), a)
		} else {
			de = c.copyRun(de, uint16(n))
		}
	}
}

// text writes the text stream at de (VRAM address word, then bytes,
// $FE new address, $FF end) through WRTVRM, each byte ANDed with mask:
// $FF writes it, $00 erases. Ported from $4051-$406B. High.
func (c *Console) text(de uint16, mask byte) {
	for {
		hl := c.rd16(de)
		de += 2
		for {
			a := c.rd(de)
			de++
			if a == 0xFF {
				return
			}
			if a == 0xFE {
				break
			}
			c.wrtvrm(hl, a&mask)
			hl++
		}
	}
}

// clearScreen hides all sprites and clears the name table. Ported from
// $44A1-$44AA (the address $7800 is masked to $3800). High.
func (c *Console) clearScreen() {
	c.hideSprites()
	c.vfill(0x7800, 0x300, 0)
}

// hideSprites writes Y = $C3 to the whole sprite attribute table.
// Ported from $4349-$4355. High.
func (c *Console) hideSprites() { c.vfill(0x3B00, 0x80, 0xC3) }

// spritesOff sets Y = $E1 in b 4-byte sprite entries in RAM from hl.
// Ported from $4356-$4363 ($4356: all 32 from $E0B0; $4364: 10 from
// $E0C8). High.
func (c *Console) spritesOff(hl uint16, b int) {
	for ; b > 0; b-- {
		c.wr(hl, 0xE1)
		hl += 4
	}
}

// mirrorByte bit-reverses a. Ported from $4584-$458E. High.
func mirrorByte(a byte) byte {
	var r byte
	for i := 0; i < 8; i++ {
		r = r<<1 | a&1
		a >>= 1
	}
	return r
}

// vmirror copies VRAM hl to de bit-reversed, one byte. Ported from
// $458F-$459A. High.
func (c *Console) vmirror(hl, de uint16) {
	c.Cost[0]++
	c.wrtvrm(de, mirrorByte(c.rdvrm(hl)))
}
