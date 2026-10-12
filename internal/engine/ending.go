package engine

// Between pyramids (state 5 with $E051 even: the map of the pyramids)
// and the ending (state 10, after pyramid 15).

// mapScreenInit draws the pyramids map and places the markers for the
// pyramid left and the door. Ported from $7748-$77A2. High.
func (c *Console) mapScreenInit() {
	c.spritesOff(0xE0B0, 0x20)
	c.rle3(0x2600, 0x7841)
	c.rle3(0x0600, 0x78EA)
	c.rleAddr(0x7913)
	c.wr(0xE130, 0)
	c.wr(0xE140, 0)
	c.soundStart(0x91)
	c.pyramidMarker(c.rd(0xE054))
	c.doorMarker(c.rd(0xE057), 0x7A79)
}

// doorMarker places the second marker sprite ($E100) relative to the
// first by door a (/2, 4 -> 3), its pattern from the table at de.
// Ported from $777B-$77A2. High.
func (c *Console) doorMarker(a byte, de uint16) {
	a >>= 1
	if a == 4 {
		a--
	}
	hl := 0x7A71 + uint16(a*2)
	c.wr(0xE100, c.rd(0xE0FC)+c.rd(hl))
	c.wr(0xE101, c.rd(0xE0FD)+c.rd(hl+1))
	c.wr(0xE102, c.rd(de+uint16(a)))
}

// pyramidMarker puts the first marker sprite ($E0FC, pattern $E4) on
// pyramid a of the map. Ported from $782E-$7840. High.
func (c *Console) pyramidMarker(a byte) {
	hl := 0x7A53 + uint16((a-1)*2)
	c.ldir(0xE0FC, hl, 2)
	c.wr(0xE0FE, 0xE4)
}

// mapScreenTick runs the map screen: the markers flash; after $58 ticks
// they move to the next pyramid; after $E0 the next pyramid starts
// ($E00D). Going from pyramid 15 to 1 instead leads to the ending, once
// the tune has ended and $80 more ticks have passed. Ported from
// $77C3-$781C. High.
func (c *Console) mapScreenTick() bool {
	a := c.rd(0x7A4B + uint16(c.rd(0xE003)&7))
	c.wr(0xE0FF, a)
	c.wr(0xE103, a)
	c.drawSprites()
	if c.pause() {
		return true
	}
	d := c.rd(0xE054) - c.rd(0xE055)
	t := c.inc(0xE130)
	if d != 0x0E {
		switch t {
		case 0x58:
			c.pyramidMarker(c.rd(0xE055))
			c.doorMarker(c.rd(0xE056), 0x7A7D)
		case 0xE0:
			c.wr(0xE00D, t+1)
		}
		return false
	}
	if t == 0x58 {
		c.wr(0xE0FC, 0x97)
		c.wr(0xE0FD, 0x7B)
		c.doorMarker(1, 0x7A7D)
		c.wr(0xE0FC, 0xC3)
		return false
	}
	if c.soundRead(0xE010) != 0 {
		return false
	}
	v := c.rd(0xE140)
	c.inc(0xE140)
	if v != 0x80 {
		return false
	}
	c.inc(0xE058)
	c.wr(0xE059, 0)
	c.ldir(0xE05A, 0xE059, 0x0A)
	c.setState(0x0A)
	c.wr(0xE001, 0)
	return false
}

// stateEnding is state 10: the ending scene. Ported from $75BD-$7747.
// High.
func (c *Console) stateEnding(sub byte) {
	c.drawSprites()
	switch sub {
	case 1:
		c.ldir(0xE134, 0x773C, 0x0C)
		c.playerSprite()
		c.wr16(0xE080, 0x75E0)
		c.wr(0xE082, 0x88)
		c.incSub()
	case 2:
		c.edges(0xE009, c.demoInput())
		c.playerUpdate()
		if c.rd(0xE053) != 0 {
			return
		}
		a := c.playerJumpNow()
		c.wr(0xE136, a)
		c.wr(0xE135, a-1)
		c.wr(0xE004, 2)
		c.incSub()
	case 3:
		c.playerUpdate()
		if c.rd(0xE142) == 0 {
			return
		}
		c.incSub()
	case 4:
		c.extraLife()
		c.endingBonus()
		c.wr(0xE004, 0xD0)
		c.incSub()
	case 5:
		if c.dec(0xE004) != 0 {
			return
		}
		c.wr(0xE000, 8)
	default:
		c.endingInit()
	}
}

// endingInit wipes the screen, then draws the ending: the pyramid, its
// door and the stones. Ported from $762A-$76B2. High.
func (c *Console) endingInit() {
	c.spritesOff(0xE0B0, 0x20)
	if !c.wipeColumn() {
		return
	}
	c.setItem(0x20)
	c.soundStart(0x8B)
	c.mapFill()
	c.wr(0xE700, 0)
	c.ldir(0xE701, 0xE700, 0x720)
	c.rle3(0x2480, 0x76E2)
	c.rle3(0x0480, 0x7713)
	c.pyramidSide(0x391F, 0x90, 0)
	c.pyramidSide(0x3A03, 0x92, 0)
	c.pyramidSide(0x3A2B, 0x92, 0)
	c.pyramidSide(0x3A04, 0x94, 1)
	c.pyramidSide(0x3A2C, 0x94, 1)
	c.vblock(0x3A1E, 0x772F, 3, 2)
	c.vfill(0x3A60, 0x20, 0x96)
	c.drawScoreLine()
	for i := uint16(0); i < 6; i++ {
		c.wrtvrm(0x3900|uint16(c.rd(0x7735+i)), 0x97)
	}
	c.incSub()
}

// pyramidSide draws one side of a pyramid down to row $3A80: on each
// row, tile cc at the edge and cc+1 filling the rows so far inward
// (leftwards, or rightwards with dir 1). Ported from $76B3-$76E1. High.
func (c *Console) pyramidSide(hl uint16, cc, dir byte) {
	for b := 0; ; b++ {
		p := hl
		for k := 0; k < b; k++ {
			c.wrtvrm(p, cc+1)
			p--
			if dir != 0 {
				p += 2
			}
		}
		c.wrtvrm(p, cc)
		hl += 0x20
		if hl >= 0x3A80 {
			return
		}
	}
}

// endingBonus draws the ending text and adds 2 x 5000 points. Ported
// from $781D-$782D. High.
func (c *Console) endingBonus() {
	c.rleAddr(0x7A81)
	c.addScore(0x5000)
	c.addScore(0x5000)
}
