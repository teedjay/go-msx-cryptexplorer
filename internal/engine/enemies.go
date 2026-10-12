package engine

// The enemies (mummies): count $E164, 22-byte blocks from $E16A, the
// current one's index in $E165 and its address in $E162 (IX).
//
// Block (roles Medium; offsets High from the code):
//
//	+0 state (table at $6F80)        +$0A walk counter, +$0B frame
//	+1 input (4 left, 8 right,       +$0C jump pointer, +$0E falling
//	   1 up, 2 down, $10 jump)       +$0F stair parity
//	+2 facing (+1 >> 2, each frame)  +$10 player above (2) / below (1)
//	+3 Y, +4 fraction, +5 X, +6      +$11 timer
//	   screen; +7..+9 velocity       +$14 type, +$15 stuck counter

// iReg is the Z80's I register as the reference machine has it: 0 (the
// BIOS never sets it). $731A reads it with LD A,I, probably meant as
// LD A,R; with I = 0 the branch it decides is never taken.
const iReg = 0

// enemy is $73DF: enemy n's block, also left in $E162.
func (c *Console) enemy(n byte) uint16 {
	b := n << 1
	a := b
	b <<= 1
	a += b
	b <<= 2
	a += b
	hl := 0xE16A + uint16(a)
	c.wr16(0xE162, hl)
	return hl
}

// enemyAt is $7200: the current enemy's byte off (through $E162).
func (c *Console) enemyAt(off byte) uint16 { return c.rd16(0xE162) + uint16(off) }

// enemiesUpdate runs every enemy's state, then (unless the state
// discarded the continuation) its sprite. Ported from $6F64-$6F91 and
// $70B3-$70BD. High.
func (c *Console) enemiesUpdate() {
	c.wr(0xE165, 0)
	for {
		ix := c.enemy(c.rd(0xE165))
		c.Cost[7]++
		c.wr(ix+2, c.rd(ix+1)>>2)
		if c.enemyState(ix) {
			c.enemySprite(ix)
		}
		if c.inc(0xE165) >= c.rd(0xE164) {
			return
		}
	}
}

// enemyState runs the enemy's state handler. It reports false when the
// handler dropped the sprite continuation ($707D). Ported from the table
// at $6F80. High.
func (c *Console) enemyState(ix uint16) bool {
	switch c.rd(ix) {
	case 0:
		c.enemyWalk(ix)
	case 1:
		c.enemyJumping(ix)
	case 2:
		c.enemyFalling(ix)
	case 3:
		c.enemyOnStairs(ix)
	case 4:
		return c.enemyAppear(ix)
	case 5:
		c.enemyMaterialize(ix)
	case 6:
		return c.enemyDying(ix)
	case 7:
		c.enemyWait(ix)
	case 8:
		return c.enemyVanish(ix)
	default:
		panic("engine: enemy state out of range")
	}
	return true
}

// enemyWalk is state 0. Ported from $6F92-$7052. High.
func (c *Console) enemyWalk(ix uint16) {
	if c.rd(ix+0x11) == 0 {
		c.wr(ix, 7)
		c.wr(ix+0x11, c.rd(0x7078+uint16(c.rd(ix+0x14))))
		return
	}
	if c.rd(0xE003)&0x0F == 0 {
		c.dec(ix + 0x11)
	}
	c.enemyChase(ix)
	if c.rd(ix+1)&0x10 != 0 {
		c.enemyTryJump(ix)
		return
	}
	if c.fallCheck(ix + 3) {
		c.enemyFall(ix)
		return
	}
	c.wr(0xE14D, 1)
	if ok, v := c.stairsStartV(ix + 1); ok {
		c.enemyStairs(ix, v)
		return
	}
	blocked, _ := c.wallCheckInput(ix + 1)
	hl := c.enemyAt(0x15)
	a := c.rd(hl)
	if blocked {
		a = a&0xF0 + 0x1F
		c.wr(hl, a)
		if a == 0xAF {
			c.enemyStuck(ix)
			return
		}
		c.enemyBlocked(ix)
		return
	}
	if a != 0xF0 && c.rd(0xE003)&0x1F == 0 {
		c.dec(hl)
	}
	c.move(c.enemyAt(4), c.rd16(ix+7))
	c.enemyAnim(ix)
}

// enemyStuck is $6FF5: state 8 (vanish and reappear), timer $22.
func (c *Console) enemyStuck(ix uint16) {
	c.wr(ix, 8)
	c.wr(ix+1, 4)
	c.wr(ix+0x11, 0x22)
}

// enemyBlocked: a type-0 enemy walled in on both sides waits (state 7);
// otherwise it jumps if it can, else turns round. Ported from
// $7022-$7052. High.
func (c *Console) enemyBlocked(ix uint16) {
	if c.rd(ix+0x14) == 0 {
		hl := c.enemyAt(3) - 1
		if blocked, _ := c.wallCheck(hl, c.rd(hl)^3); blocked {
			c.wr(ix+0x11, 0xFF)
			c.wr(ix, 7)
			return
		}
	}
	if c.rd(c.enemyAt(3)) >= 8 {
		if free, _ := c.headRoom(c.enemyAt(3)); free {
			c.jumpStart(ix)
			return
		}
	}
	c.wr(ix+1, c.rd(ix+1)^0x0C)
}

// enemyAnim advances the walk animation. Ported from $7053-$705B.
func (c *Console) enemyAnim(ix uint16) {
	hl := c.enemyAt(0x0A)
	c.inc(hl)
	c.animFrame(hl)
}

// enemyStairs sets the enemy's direction and stair parity on stepping
// onto a stair whose cell is v. Ported from $705C-$7077. High.
func (c *Console) enemyStairs(ix uint16, v byte) {
	b := byte(8)
	a := v
	if c.rd(ix+1)&1 != 0 {
		b = 4
		a ^= 1
	}
	a &= 1
	c.wr(ix+0x0F, a)
	if a != 0 {
		b ^= 0x0C
	}
	c.wr(ix+1, b)
}

// enemySprite writes the enemy's sprite ($E0C8 + 4n) when it is on the
// screen shown, else hides it. Ported from $707D-$70B2. High.
func (c *Console) enemySprite(ix uint16) {
	y := c.rd(ix + 3)
	x := c.rd(ix + 5)
	hl := c.enemySlot()
	if c.rd(0xE13A) != c.rd(ix+6) {
		c.wr(hl, 0xE0)
		return
	}
	y -= 2
	f := c.rd(ix + 0x0B)
	if f&1 == 0 {
		y++
	}
	d := c.rd(0x70BE + uint16(f))
	c.wr(hl, y)
	c.wr(hl+1, x)
	if c.rd(ix+2)&1 != 0 {
		d += 0x60
	}
	c.wr(hl+2, d)
}

// enemySlot is $71F3: the current enemy's sprite entry.
func (c *Console) enemySlot() uint16 { return 0xE0C8 + uint16(c.rd(0xE165)*4) }

// enemyTryJump jumps if Y >= 8 and there is head room. Ported from
// $70C9-$70D6. High.
func (c *Console) enemyTryJump(ix uint16) {
	if c.rd(c.enemyAt(3)) < 8 {
		return
	}
	if free, _ := c.headRoom(c.enemyAt(3)); !free {
		return
	}
	c.jumpStart(ix)
}

// enemyJumping is state 1. Ported from $70EE-$710A. High.
func (c *Console) enemyJumping(ix uint16) {
	hl := c.enemyAt(3) - 1
	c.jumpStep(hl)
	c.screenWrap(hl)
	if c.landed(ix, false) {
		c.enemyFalling(ix)
	}
}

// enemyFall: about to fall, some enemies jump instead. Ported from
// $711F-$713D. High.
func (c *Console) enemyFall(ix uint16) {
	if t := c.rd(ix + 0x14); t != 0 && t != 3 && c.rd(ix+0x10) != 1 {
		if c.rd(c.enemyAt(3)) >= 8 {
			if free, _ := c.headRoom(c.enemyAt(3)); free {
				c.jumpStart(ix)
				return
			}
		}
	}
	c.enemyFalling(ix)
}

// enemyFalling is state 2: on landing, state 0. Ported from
// $713E-$7147. High.
func (c *Console) enemyFalling(ix uint16) {
	if !c.fallStep(ix) {
		c.wr(ix, 0)
	}
}

// enemyOnStairs is state 3: moves along the stair (speed mask from
// $7179 by type) while it has a direction; off the stair, state 0.
// Ported from $7148-$7178. High.
func (c *Console) enemyOnStairs(ix uint16) {
	if c.rd(ix+1)&0x0C == 0 {
		return
	}
	b := c.rd(0x7179 + uint16(c.rd(ix+0x14)))
	c.wr(0xE05F, 1)
	if c.stairStep(ix+1, b) {
		c.wr(ix+0x11, 0)
		c.wr(ix, 0)
		return
	}
	c.enemyAnim(ix)
}

// enemyAppear is state 4: invisible for (timer) odd frames, then state 5
// with sound $87. No sprite update. Ported from $717E-$719B. High.
func (c *Console) enemyAppear(ix uint16) bool {
	if c.rd(0xE003)&1 != 0 {
		return false
	}
	if c.dec(ix+0x11) != 0 {
		return false
	}
	c.inc(ix)
	c.wr(ix+0x11, 0x82)
	c.soundStart(0x87)
	return false
}

// enemyMaterialize is state 5: flickers between frames 8 and 9 every 32
// frames, then walks. Ported from $719C-$71C3. High.
func (c *Console) enemyMaterialize(ix uint16) {
	c.wr(ix+1, 8)
	if c.dec(ix+0x11) == 0 {
		c.wr(ix, 0)
		c.wr(ix+0x0B, 2)
		return
	}
	a := c.rd(ix + 0x11)
	if a < 7 {
		c.wr(ix+0x0B, 2)
		return
	}
	if a&0x1F != 0 {
		return
	}
	f := byte(8)
	if a&0x20 != 0 {
		f++
	}
	c.wr(ix+0x0B, f)
}

// enemyDying is state 6: frame $0A, then it reappears (state 4). Ported
// from $71C4-$71E0. High.
func (c *Console) enemyDying(ix uint16) bool {
	c.wr(ix+1, 8)
	a := c.dec(ix + 0x11)
	if a == 0 {
		c.enemyReset(ix)
		return false
	}
	if a&0x1F == 0 {
		c.wr(ix+0x0B, 0x0A)
	}
	return true
}

// enemyReset hides the sprite and restarts the enemy as appearing
// (state 4, timer $80, frame 9). Ported from $71E1-$71F2. High.
func (c *Console) enemyReset(ix uint16) {
	c.wr(c.enemySlot(), 0xE0)
	c.wr(ix, 4)
	c.wr(ix+0x11, 0x80)
	c.wr(ix+0x0B, 9)
}

// enemyWait is state 7: stands (frame 2) for timer x 32 frames, then
// picks a direction; timer $E0 turns it round and it vanishes if walled
// in. Ported from $7208-$7260. High.
func (c *Console) enemyWait(ix uint16) {
	a := c.rd(ix + 0x11)
	switch a {
	case 0:
		c.enemyChoose(ix)
		c.wr(ix, 0)
		return
	case 0xE0:
		v := c.rd(ix+1) ^ 0x0C
		c.wr(ix+1, v)
		c.wr(ix+2, v>>2&3)
		if a, _ := c.mapCell(c.enemyAt(3), 0x00, 0xFC); a&0xF0 != 0x10 {
			c.enemyTryJump(ix)
			return
		}
		if a, _ := c.mapCell(c.enemyAt(3), 0x10, 0xFC); a&0xF0 != 0x10 {
			c.enemyTryJump(ix)
			return
		}
		c.enemyStuck(ix)
		return
	}
	c.wr(ix+0x0B, 2)
	c.wr(ix+2, a&3)
	if c.rd(0xE003)&0x1F != 0 {
		return
	}
	if c.dec(ix+0x11) != 0 {
		return
	}
	c.enemyChoose(ix)
	c.wr(ix, 0)
}

// enemyVanish is state 8: frame $0A, then it starts again from its
// place in the pyramid data. Ported from $7261-$7283. High.
func (c *Console) enemyVanish(ix uint16) bool {
	c.wr(ix+1, 8)
	a := c.dec(ix + 0x11)
	if a == 0 {
		c.enemyReset(ix)
		c.wr(ix+0x11, 0xFF)
		c.enemyInit()
		return false
	}
	if a&0x1F == 0 {
		c.wr(ix+0x0B, 0x0A)
	}
	return true
}

// enemyChase steers an enemy near a screen edge when the player is just
// across it on the next screen (the enemy turns away). Ported from
// $7284-$72C7. High.
func (c *Console) enemyChase(ix uint16) {
	b := c.rd(ix + 6)
	if c.rd(0xE13A) == b {
		return
	}
	x := c.rd(ix + 5)
	cc := byte(0x50)
	if x >= 0x50 {
		cc = 0xB0
		if x < 0xB0 {
			return
		}
		b += 2
	}
	b--
	if c.rd(0xE13A) != b {
		return
	}
	cc = -cc
	px := c.rd(0xE139)
	if cc == 0x50 {
		if px >= cc {
			return
		}
	} else if px < cc {
		return
	}
	a := byte(4)
	if c.rd(0xE13A) < c.rd(ix+6) {
		a = 8
	}
	c.wr(ix+1, a)
}

// enemyChoose picks the enemy's next direction: towards the player on
// the same level if nothing solid is between them, else towards a stair
// up or down ($7356's scan) by where the player is. Ported from
// $72C8-$7332. High.
func (c *Console) enemyChoose(ix uint16) {
	c.wr(ix+0x11, 5)
	c.enemyScan(ix)
	switch c.enemyLevel(ix) {
	case 1:
		c.enemyUseStair(0xE168)
		return
	case 2:
		c.enemyUseStair(0xE166)
		return
	}
	var cy bool
	if c.rd(0xE13A) != c.rd(ix+6) {
		cy = c.rd(0xE13A) < c.rd(ix+6)
	} else {
		cy = c.rd(0xE139) < c.rd(ix+5)
	}
	a := byte(8)
	if cy {
		a = 4
	}
	c.wr(ix+1, a)
	_, de := c.mapAt(0xE137)
	_, hl := c.mapAt(c.enemyAt(3))
	for b := 0; b < 0x20; b++ {
		if hl == de {
			return
		}
		if c.rd(hl)&0xF0 == 0x10 {
			// $731A: LD A,I / RRA / OR C / RRA / CALL C,$732C.
			if (iReg>>1|a)&1 != 0 {
				c.enemyUseStair(0xE166)
			}
			c.enemyUseStair(0xE168)
			return
		}
		hl--
		if a&0x04 == 0 {
			hl += 2
		}
	}
}

// enemyUseStair sets the direction recorded at hl by the scan, if any.
// Ported from $7322-$7332. High.
func (c *Console) enemyUseStair(hl uint16) {
	if a := c.rd(hl); a != 0 {
		c.wr(c.rd16(0xE162)+1, a)
	}
}

// enemyLevel is $7333: 0 when the player is on the enemy's level (or
// both on stairs ...), 1 when below, 2 when above; kept in +$10. Ported
// from $7333-$7355. High.
func (c *Console) enemyLevel(ix uint16) byte {
	b := byte(0)
	py := c.rd(0xE137)
	ey := c.rd(c.enemyAt(3))
	if c.rd(0xE134) != 3 {
		a := py - ey - 0x0A
		if _, f := add8(a, 0x12, false); f.C {
			c.wr(ix+0x10, 0)
			return 0
		}
	}
	b = 1
	if py < ey {
		b = 2
	}
	c.wr(ix+0x10, b)
	return b
}

// enemyScan looks up to five cells along the floor each way from two
// cells ahead: $E166/$E167 record a stair up (class $20, or $3x but not
// $30) and $E168/$E169 a way down (an empty cell below, or a cell below
// of low nibble 5 or more), each as direction|1 or |2 and the furthest
// distance. Ported from $7356-$73DA. High.
func (c *Console) enemyScan(ix uint16) {
	for a := uint16(0xE166); a < 0xE16A; a++ {
		c.wr(a, 0)
	}
	dir := c.rd(ix + 1)
	c.enemyScanDir(dir)
	c.enemyScanDir(dir ^ 0x0C)
}

func (c *Console) enemyScanDir(dir byte) {
	step := func(hl uint16) uint16 {
		hl--
		if dir&0x04 == 0 {
			hl += 2
		}
		return hl
	}
	_, hl := c.mapCell(c.enemyAt(3), 0x08, 0x08)
	hl = step(hl)
	record := func(de uint16, b, cc byte) {
		if c.rd(de) >= b {
			return
		}
		c.wr(de, b)
		c.wr(de-1, dir&0xFC|cc)
	}
	for b := byte(0); b != 5; b++ {
		hl = step(hl)
		a := c.rd(hl)
		cls := a & 0xF0
		if cls == 0x10 {
			return
		}
		if cls == 0x20 || cls == 0x30 && a != 0x30 {
			record(0xE167, b, 1)
		}
		below := c.rd(hl + 0x60)
		if below&0xF0 == 0 {
			record(0xE169, b, 2)
			return
		}
		if below&0x0F >= 5 {
			record(0xE169, b, 2)
		}
	}
}

// stairsStartV is stairsStart also returning the stair cell (C), which
// the enemy code uses.
func (c *Console) stairsStartV(hl uint16) (bool, byte) {
	a := c.rd(hl) & 3
	if a == 0 {
		return false, 0
	}
	if a&1 == 0 {
		if c.rd(hl+4)&7 != 4 {
			return false, 0
		}
		_, cell := c.mapAt(hl + 2)
		return c.stairsDown(hl), c.rd(cell + 0xC1)
	}
	x := c.rd(hl+4) & 7
	if x == 0 {
		return false, 0
	}
	_, cell := c.mapAt(hl + 2)
	v := c.rd(cell + 0x61)
	return c.stairsStart(hl), v
}

// enemyInit (re)starts enemy $E165 from the pyramid data at $E14E (3
// bytes each: Y, X|screen, type): appearing (state 4), speed and colour
// from $6D44 by type + $E058 (at most 4), timer $10. Ported from
// $6CF8-$6D43. High.
func (c *Console) enemyInit() {
	n := c.rd(0xE165)
	ix := c.enemy(n)
	c.wr(ix, 4)
	src := 0xE14E + uint16(n*3)
	de := c.copy4(ix+3, src)
	src += 2
	a := c.rd(0xE058) + c.rd(src)
	if a >= 5 {
		a = 4
	}
	b := c.rd(0x6D44 + uint16(a))
	c.wr(de, b&0xF0)
	c.wr(de+0x0A, 0x10)
	c.wr(de+0x0D, a)
	c.wr(c.enemySlot()+3, b&0x0F)
}

// copy4 is $6D61: (de) = (hl), (de+2) = (hl+1) & $F8, (de+3) = (hl+1) &
// 1 (Y; X and screen from one byte). It returns de + 4. Ported from
// $6D61-$6D6F. High.
func (c *Console) copy4(de, hl uint16) uint16 {
	c.wr(de, c.rd(hl))
	c.wr(de+2, c.rd(hl+1)&0xF8)
	c.wr(de+3, c.rd(hl+1)&1)
	return de + 4
}
