package engine

// The player. RAM (roles Medium unless noted):
//
//	$E134 state: 0 walk/stand, 1 jump, 2 fall, 3 stairs, 4 throw,
//	      5 dig, 6 push (door)               (High: the jump table at $4C7F)
//	$E135 input this frame (copied from $E009 when walking or on stairs)
//	$E136 facing: bit 0 left (1), bit 1 right (2)
//	$E137 Y; $E139/$E13A X (16-bit: $E13A is the screen 0-2)
//	$E13B velocity for $7402 ($01E0 walking)
//	$E13E walk counter, $E13F animation frame
//	$E140 jump table pointer, $E142 falling flag (jump)
//	$E143 stair parity; $E144 item (high nibble: $10 dagger, $20 pick)
//	$E145 timer; $E146 push counter; $E148-$E14C the dig point
//	$E14D 1 while an enemy uses the shared stair test

// playerUpdate runs the player's state, then updates its two sprites
// ($4CFA is pushed as a continuation). Ported from $4C59-$4C8C. High.
func (c *Console) playerUpdate() {
	st := c.rd(0xE134)
	if st == 0 || st == 3 {
		a := c.rd(0xE009)
		c.wr(0xE135, a)
		if a != 0 {
			if d := a >> 2 & 3; d != 0 {
				c.wr(0xE136, d)
			}
		}
	}
	switch c.rd(0xE134) {
	case 0:
		c.playerWalk()
	case 1:
		c.playerJumping()
	case 2:
		c.playerFalling()
	case 3:
		c.playerStairs()
	case 4:
		c.playerThrow()
	case 5:
		c.playerDig()
	case 6:
		c.playerPush()
	default:
		panic("engine: player state out of range")
	}
	c.playerSprite()
}

// playerWalk is state 0. Ported from $4C8D-$4CF9. High.
func (c *Console) playerWalk() {
	if c.rd(0xE008)&0x10 != 0 {
		switch a := c.rd(0xE144) & 0xF0; {
		case a == 0:
			c.playerJump()
		case a != 0x10:
			c.digStart()
		default:
			c.throwStart()
		}
		return
	}
	if c.fallCheck(0xE137) {
		c.playerStartFall()
		return
	}
	c.wr(0xE14D, 0)
	if c.stairsStart(0xE135) {
		return
	}
	if c.rd(0xE135)&0x0C == 0 {
		c.wr(0xE145, 0)
		c.wr(0xE13F, 1)
		return
	}
	c.inc(0xE13E)
	blocked, hl := c.wallCheck(0xE136, c.rd(0xE136))
	if !blocked {
		c.wr(0xE146, 0)
		c.playerMove()
		return
	}
	if a := c.rd(hl); a&0xF0 == 0x50 && (a&0x0F)-1 >= 2 {
		if c.inc(0xE146) == 0x10 {
			c.wr(0xE134, 6)
			c.wr(0xE145, 0x20)
			c.soundStart(0x03)
			c.doorPush()
			return
		}
	}
	c.walkFrame()
}

// playerMove moves the player by its velocity and animates the walk.
// Ported from $4CF4-$4CF9.
func (c *Console) playerMove() {
	c.move(0xE138, c.rd16(0xE13B))
	c.walkFrame()
}

// walkFrame sets the animation frame from the walk counter. Ported from
// $4D2E-$4D38. High.
func (c *Console) walkFrame() { c.animFrame(0xE13E) }

// animFrame sets (hl+1) = ((hl) >> 2) & 7. Ported from $4D31-$4D38.
func (c *Console) animFrame(hl uint16) {
	c.wr(hl+1, c.rd(hl)>>2&7)
}

// playerSprite writes the player's two 16x16 sprites (frame $E13F from
// the table at $4D46; Y - 2, one lower on odd frames; patterns +$60
// when facing left, the mirrored set). Ported from $4CFA-$4D2D. High.
func (c *Console) playerSprite() {
	y := c.rd(0xE137)
	x := c.rd(0xE139)
	a := c.rd(0xE13F)
	y -= 2
	if a&1 != 0 {
		y++
	}
	d := c.rd(0x4D46 + uint16(a))
	hl := uint16(0xE0C0)
	for i := 0; i < 2; i++ {
		c.wr(hl, y)
		c.wr(hl+1, x)
		p := d
		if c.rd(0xE136)&1 != 0 {
			p += 0x60
		}
		c.wr(hl+2, p)
		d += 4
		hl += 4
	}
}

// playerJump starts a jump if there is head room. Ported from
// $4D50-$4D6C. High.
func (c *Console) playerJump() {
	if c.rd(0xE137) == 0 {
		return
	}
	free, de := c.headRoom(0xE137)
	if !free {
		return
	}
	if c.rd(de)&0xF0 == 0x10 {
		return
	}
	c.playerJumpNow()
}

// playerJumpNow is $4D61: the jump direction is the input held now. It
// returns the A that $70D7 leaves behind ($3F), which the ending uses.
func (c *Console) playerJumpNow() byte {
	c.wr(0xE143, c.rd(0xE009))
	return c.jumpStart(0xE134)
}

// jumpStart puts the object whose state byte is at hl into a jump: state
// 1, input bit 4 cleared, frame 2, jump pointer $7557, rising. It returns
// the A it leaves (the low byte of hl+$0B, from ADD_HL_A). Ported from
// $70D7-$70ED. High.
func (c *Console) jumpStart(hl uint16) byte {
	c.wr(hl, 1)
	c.wr(hl+1, c.rd(hl+1)&^0x10)
	c.wr(hl+0x0B, 2)
	c.wr16(hl+0x0C, 0x7557)
	c.wr(hl+0x0E, 0)
	return byte(hl + 0x0B)
}

// playerJumping is state 1. Ported from $4D6D-$4D83. High.
func (c *Console) playerJumping() {
	c.jumpStep(0xE136)
	if !c.landed(0xE134, c.rd(0xE143) == 0x10) {
		return
	}
	c.wr(0xE134, 3)
	c.playerFalling()
}

// playerStartFall starts a fall: the falling tone restarts. Ported from
// $4D8E-$4D96. High.
func (c *Console) playerStartFall() {
	c.wr(0xE03B, 0)
	c.soundStart(0x01)
	c.playerFalling()
}

// playerFalling is state 2: on landing, sound 2 and state 0. Ported
// from $4D97-$4DA0 and $4D84-$4D8D. High.
func (c *Console) playerFalling() {
	if c.fallStep(0xE134) {
		return
	}
	c.soundStart(0x02)
	c.wr(0xE134, 0)
}

// playerStairs is state 3: moves on the stairs every other frame while
// left or right is held; off the stairs, state 0. Ported from
// $4DA1-$4DBE. High.
func (c *Console) playerStairs() {
	if c.rd(0xE135)&0x0C == 0 {
		return
	}
	c.wr(0xE05F, 0)
	if c.stairStep(0xE135, 1) {
		c.wr(0xE134, 0)
		return
	}
	c.walkFrame()
}

// throwStart (the trigger with item $10) starts a throw, unless one of
// the four doors is busy (class $30), when facing an empty or brick cell
// ahead. Ported from $4DBF-$4E1D. High.
func (c *Console) throwStart() {
	c.wr(0xE1F4, 0)
	for {
		if c.door(4)&0xF0 == 0x30 {
			return
		}
		if c.inc(0xE1F4) == 4 {
			break
		}
	}
	c.wr(0xE131, 0)
	b := byte(0xFF)
	if c.rd(0xE136)&1 == 0 {
		b = 0x12
	}
	cls, de, z := c.probe(0xE137, b, 0x00)
	if !z {
		cls, z = classSolid(c.rd(de))
		if !z {
			c.throwSet()
			return
		}
	}
	if cls != 0x10 {
		return
	}
	if c.rd(0xE139)&7 != 4 {
		return
	}
	if cls, _, _ := c.probe(0xE137, b, 0xFF); cls != 0 {
		return
	}
	c.inc(0xE131)
	c.throwSet()
}

// throwSet is $4DF6: state 4 with timer $15, frame 8.
func (c *Console) throwSet() {
	c.wr(0xE145, 0x15)
	c.wr(0xE134, 4)
	c.wr(0xE13F, 8)
}

// classSolid is $5AC8: Z for the classes $10, $30, $80 and $40; it
// returns the class.
func classSolid(a byte) (byte, bool) {
	cls := a & 0xF0
	return cls, cls == 0x10 || cls == 0x30 || cls == 0x80 || cls == 0x40
}

// playerThrow is state 4: after the wind-up (timer bit 4 set) the dagger
// is launched as object $E261 in the $E264 list; then, when the timer
// runs out, back to walking with no item. Ported from $4E1E-$4E77 and
// $4EBA-$4EC0. High.
func (c *Console) playerThrow() {
	if c.rd(0xE145)&0x10 == 0 {
		if c.dec(0xE145) != 0 {
			return
		}
		c.wr(0xE134, 0)
		c.setItem(0)
		return
	}
	if c.dec(0xE145)&0x0F != 0 {
		return
	}
	c.wr(0xE262, c.rd(0xE261))
	hl := c.obj(0)
	c.wr(hl, 4)
	hl++
	c.wr(hl, c.rd(0xE136)&3)
	if c.rd(0xE131) != 0 {
		c.wr(hl-1, 7)
		hl++
		c.wr(hl, c.rd(0xE137)-8)
		hl += 2
		c.wr(hl, c.rd(0xE139)+4)
		hl++
		c.wr(hl, c.rd(0xE13A))
		c.wr(hl+4, 0)
	}
	c.wr(0xE13F, 9)
}

// setItem sets the item $E144 and reloads the player's sprite patterns
// for it. Ported from $4EBB-$4EC0 (into $5056).
func (c *Console) setItem(a byte) {
	c.wr(0xE144, a)
	c.playerPatterns()
}

// playerDig is state 5: the pick swings (frames 8/9 by timer bit 4);
// each swing digs ($68F7) until the dig counter $E148 runs out. Ported
// from $4E78-$4EB9. High.
func (c *Console) playerDig() {
	hl := uint16(0xE145)
	if c.dec(hl)&0x0F != 0 {
		a := byte(9)
		if c.rd(hl)&0x10 != 0 {
			a = 8
		}
		c.wr(0xE13F, a)
		return
	}
	b := byte(4)
	if c.rd(hl)&0x10 == 0 {
		b = 8
		c.wr(0xE148, c.rd(0xE148)-3)
		c.digStep()
	}
	c.wr(hl, (c.rd(hl)&0xF0|b)^0x10)
	if c.rd(0xE148) != 0 {
		return
	}
	c.digDone()
	c.wr(0xE13F, 1)
	c.wr(0xE134, 0)
	c.setItem(0)
}

// digStart (the trigger with the pick) starts digging at the cell below
// and ahead, if it is diggable brick. Ported from $4EC1-$4F6B. High.
func (c *Console) digStart() {
	if !c.standing(0xE137) {
		return
	}
	if blocked, _ := c.wallCheck(0xE136, c.rd(0xE136)); blocked {
		if blocked, _ := c.wallCheck(0xE136, c.rd(0xE136)^3); blocked {
			c.digSide()
			return
		}
	}
	e := c.rd(0xE136)
	d := c.rd(0xE137) + 0x10
	b, cc := byte(0), byte(0x10)
	if c.rd(0xE139)&7 >= 5 {
		b = 8
	}
	a := cc
	if e&1 != 0 {
		a = b
	}
	ex := a + c.rd(0xE139)
	if ex&0xF8 == 0 || ex&0xF8 == 0xF8 {
		return
	}
	c.wr(0xE149, d)
	c.wr(0xE14B, ex)
	c.wr(0xE14C, c.rd(0xE13A))
	v, hl := c.mapAt(0xE149)
	if v&0xF0 != 0x10 {
		return
	}
	if lo := c.rd(hl) & 0x0F; lo >= 4 && lo != 9 {
		return
	}
	if c.rd(hl+0x60)&0xF0 == 0x50 {
		return
	}
	if k := c.rd(hl-0xC0+0x60) & 0xF0; k != 0 && k != 0x20 {
		return
	}
	c.wr(0xE134, 5)
	x := c.rd(0xE139)
	if c.rd(0xE136)&1 != 0 {
		if x&7 >= 5 {
			x = (x+4)&0xF8 + 2
		}
	} else if (x&7)-1 >= 3 {
		x &= 0xFC
	}
	c.wr(0xE139, x)
	c.digArm()
}

// digArm is $4F61: dig counter $15, state 5.
func (c *Console) digArm() {
	c.wr(0xE148, 0x15)
	c.wr(0xE134, 5)
}

// digSide digs sideways when walled in on both sides. Ported from
// $4F6C-$4FB7. High.
func (c *Console) digSide() {
	c.wr(0xE149, c.rd(0xE137))
	a := c.rd(0xE139)
	if c.rd(0xE136)&1 == 0 {
		a += 0x10
	}
	c.wr(0xE14B, a)
	if a&0xF8 == 0 || a&0xF8 == 0xF8 {
		return
	}
	c.wr(0xE14C, c.rd(0xE13A))
	c.digArm()
	c.wr(0xE145, 2)
	_, hl := c.mapAt(0xE137)
	if c.rd(0xE136)&1 == 0 {
		hl += 2
	}
	if c.rd(hl)&0xF0 != 0x10 {
		c.wr(0xE149, c.rd(0xE149)+8)
		c.wr(0xE148, 9)
	}
	c.soundStart(0x45)
}

// playerPush is state 6: walks on for $E145 frames (every other frame).
// Ported from $4FB8-$4FCE. High.
func (c *Console) playerPush() {
	if c.rd(0xE003)&1 != 0 {
		return
	}
	if c.dec(0xE145) == 0 {
		c.wr(0xE134, 0)
		return
	}
	c.inc(0xE13E)
	c.playerMove()
}

// digDone re-aligns the player after digging: to the 4-pixel step on
// whichever side is open. Ported from $4FCF-$4FED. High.
func (c *Console) digDone() {
	b := byte(4)
	if c.probeAhead(0xE137, 0x04, 0x0C) {
		if c.probeAhead(0xE137, 0x0B, 0x0C) {
			return
		}
		b = 0
	}
	c.wr(0xE139, (c.rd(0xE139)+b)&0xFC)
}

// playerPatterns loads the player's sprite patterns for the item held
// (one of the RLE streams from $5075 by $E144's high nibble) to $1800,
// then mirrors them into $1B00 for facing the other way. Ported from
// $5056-$5074. High.
func (c *Console) playerPatterns() {
	i := uint16(c.rd(0xE144) >> 3 & 0x1E)
	c.rleAddr(c.rd16(0x5075 + i))
	c.mirrorSprites(0x1800, 0x1B10, 0x0A)
}

// mirrorSprites mirrors cnt 16x16 sprite patterns from VRAM hl to de:
// each source's left half (16 bytes) goes, bit-reversed, to the
// destination's right half and the right half to the left. E advances
// as a byte only. Ported from $454C-$4567. High.
func (c *Console) mirrorSprites(hl, de uint16, cnt byte) {
	for ; cnt > 0; cnt-- {
		d := de
		for {
			for b := 0; b < 0x10; b++ {
				c.vmirror(hl, d)
				hl++
				d = d&0xFF00 | uint16(byte(d)+1)
			}
			d = d&0xFF00 | uint16(byte(d)-0x20)
			if d&0x10 != 0 {
				break
			}
		}
		de += 0x20
	}
}

// mirrorTiles mirrors cnt 8x8 patterns from VRAM hl to de, into each of
// the three thirds (the source stays in the first). Ported from
// $4568-$4583. High.
func (c *Console) mirrorTiles(hl, de uint16, cnt byte) {
	for t := 0; t < 3; t++ {
		s, d := hl, de
		for n := cnt; n > 0; n-- {
			for b := 0; b < 8; b++ {
				c.vmirror(s, d)
				s++
				d++
			}
		}
		de += 0x800
	}
}
