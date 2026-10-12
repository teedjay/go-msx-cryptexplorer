package engine

// The play state's tick, the screen flip, pause, and leaving or
// entering through a door.

// playTick is state 5 inside a pyramid ($E051 odd). Ported from
// $4B0C-$4B74. High.
func (c *Console) playTick() {
	if c.rd(0xE133) != 0 {
		b := c.soundRead(0xE012)
		a := c.soundRead(0xE020)
		if a|b == 0 {
			c.wr(0xE133, 0)
			c.soundStart(0x8B)
		}
	}
	c.drawSprites()
	if c.screenEdge() {
		return
	}
	c.flashExits()
	if c.rd(0xE132) != 0 {
		c.doorSequence()
		return
	}
	if c.pause() {
		return
	}
	c.enemiesUpdate()
	c.jewelsUpdate()
	c.playerUpdate()
	c.daggersUpdate()
	c.daggerPickup()
	c.jewelsTouch()
	if c.doorsUpdate() {
		return
	}
	c.trapsUpdate()
	c.picksTouch()
	c.enemiesTouch()
	c.wallsUpdate()
	if c.rd(0xE002)&0x40 == 0 {
		return
	}
	if ^c.snsmat(6)&0x40 != 0 { // F2: give up this life
		c.wr(0xE053, 0)
		c.wr(0xE05E, 1)
		c.soundStart(0x1D)
	}
}

// enemiesTouch: an enemy touching the player (both on stairs or both
// off them, same screen, within the box at $5CF6) kills the player.
// Ported from $5CA8-$5CF5. High.
func (c *Console) enemiesTouch() {
	for n := byte(0); ; {
		ix := c.enemy(n)
		hl := c.rd16(0xE162)
		if s := c.rd(hl); s < 4 || s == 7 {
			ps := c.rd(0xE134)
			ok := true
			if ps == 3 || s == 3 {
				ok = ps == c.rd(hl)
			}
			if ok && c.rd(0xE13A) == c.rd(ix+6) {
				if c.playerBox(0x5CF6, c.rd(ix+3), c.rd(ix+5)) {
					c.soundStart(0x1D)
					c.wr(0xE053, 0)
					return
				}
			}
		}
		n++
		if n == c.rd(0xE164) {
			return
		}
	}
}

// screenEdge: the player at the screen edge in the facing direction
// starts the screen flip (state 9) and the rest of the tick is skipped
// (the original pops its caller's return address). Ported from
// $4BBC-$4BE3. High.
func (c *Console) screenEdge() bool {
	x := c.rd(0xE139)
	if c.rd(0xE136)&1 != 0 {
		if x >= 2 {
			return false
		}
	} else if x < 0xF4 {
		return false
	}
	c.wr(0xE004, 0x20)
	c.wr(0xE147, 1)
	c.hideSprites()
	c.spritesOff(0xE0B0, 0x20)
	c.wr(0xE000, 9)
	return true
}

// stateExit is state 9: the screen scrolls by 4 cells a tick; then the
// player is on the next screen. Ported from $427B-$429B. High.
func (c *Console) stateExit() {
	if c.screenScroll() {
		return
	}
	c.wr(0xE000, 5)
	b := c.rd(0xE13A)
	cc := byte(0xF0)
	if c.rd(0xE136)&1 != 0 {
		b--
	} else {
		b++
		cc = 4
	}
	c.wr(0xE139, cc)
	c.wr(0xE13A, b)
	c.playerSprite()
}

// screenScroll draws the next scroll step; true (carry) while it goes
// on. Ported from $5D19-$5D37. High.
func (c *Console) screenScroll() bool {
	a := c.rd(0xE004) - 4
	c.wr(0xE004, a)
	if a == 0xFC {
		return false
	}
	if c.rd(0xE136)&1 == 0 {
		a = -(a - 0x20)
	}
	c.scrollDraw(a)
	return c.rd(0xE147) != 0
}

// scrollDraw draws the map from column b of the screen pair the flip is
// between (screens 0-1, or 1-2). Ported from $5D38-$5D53. High.
func (c *Console) scrollDraw(b byte) {
	f := c.rd(0xE136)
	if f == 3 {
		f = 1
	}
	base := byte(0)
	if f+c.rd(0xE13A) == 3 {
		base = 0x20
	}
	c.drawMap(0xE760 + uint16(base+b))
}

// pause toggles the pause with F1 (in a game). While paused the tick
// stops here and "PAUSING" flashes. It reports true when the rest of the
// tick is skipped (the original pops its caller's return address).
// Ported from $4C11-$4C58. High.
func (c *Console) pause() bool {
	if c.rd(0xE002)&0x40 == 0 {
		return false
	}
	a := c.edges(0xE065, ^c.snsmat(6))
	v := c.rd(0xE067)
	if a&0x20 != 0 {
		v ^= 1
		c.wr(0xE067, v)
		if v == 0 {
			c.vcopy(0x3AF6, 0x658A, 7)
		}
		return false
	}
	if v == 0 {
		return false
	}
	if b := c.rd(0xE003); b&7 == 0 {
		de := uint16(0x4C52)
		if b&0x10 != 0 {
			de = 0x658A
		}
		c.vcopy(0x3AF6, de, 7)
	}
	return true
}

// doorSequence runs a door transition ($E130): the player walks through
// it, the doors close, the clear bonus, and the wait. Ported from
// $6DF0-$6EB7. High.
func (c *Console) doorSequence() {
	switch c.rd(0xE130) {
	case 0:
		c.doorWalk()
	case 1:
		if c.doorsUpdate() {
			return
		}
		if c.rd(0xE1C2) == 0 {
			return
		}
		if c.rd(0xE000) == 4 {
			c.wr(0xE004, 0x28)
			c.incSub()
			return
		}
		c.soundStart(0x20)
		c.inc(0xE130)
		c.wr(0xE1F4, 0)
	case 2:
		if c.dec(0xE004)&0x80 == 0 {
			return
		}
		if !c.pyramidCleared() {
			c.addScore(0x2000)
			c.soundStart(0x8F)
		}
		c.inc(0xE130)
		c.wr(0xE004, 0x70)
		m := c.pyramidBit()
		c.wr(0xE062, c.rd(0xE062)|byte(m))
		c.wr(0xE063, c.rd(0xE063)|byte(m>>8))
	case 3:
		if c.dec(0xE004) != 0 {
			return
		}
		c.wr(0xE132, 0)
		c.wr(0xE000, 8)
	default:
		panic("engine: door sequence out of range")
	}
}

// doorWalk moves the player through the door every fourth frame: down
// into the pyramid in the intro, up and out when leaving. Ported from
// $6DFE-$6E5D. High.
func (c *Console) doorWalk() {
	if c.rd(0xE003)&3 != 0 {
		return
	}
	c.wr16(0xE13B, 0x01E0)
	c.move(0xE138, c.rd16(0xE13B))
	var a byte
	if c.rd(0xE000) == 4 {
		a = c.inc(0xE137) & 7
		if a == 0 {
			c.wr(0xE13F, 1)
			c.playerSprite()
			c.doorWalkDone()
			return
		}
	} else {
		c.dec(0xE137)
		a = c.rd(0xE139) + 8
		if a >= c.rd(0xE0B1) {
			c.spritesOff(0xE0B0, 0x20)
			c.doorWalkDone()
			return
		}
	}
	// $6E3E: INC ($E13F), then overwritten with the stale A & 7.
	c.wr(0xE13F, a&7)
	c.playerSprite()
}

// doorWalkDone is $6E48: walk speed $00A8, the door sprites hidden, the
// next step after 32 frames.
func (c *Console) doorWalkDone() {
	c.wr16(0xE13B, 0x00A8)
	c.spritesOff(0xE0B0, 4)
	c.inc(0xE130)
	c.wr(0xE003, 0x20)
}

// extraLife awards a life: sound $8A and the count redrawn. Ported from
// $6EAC-$6EB7. High.
func (c *Console) extraLife() {
	c.inc(0xE050)
	c.soundStart(0x8A)
	c.drawRest()
}
