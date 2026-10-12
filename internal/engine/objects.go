package engine

// The pyramid's object lists, built by the stage builder ($6AA3).
// Each list has a count and an index byte; the accessors compute the
// entry address with the original's 8-bit multiplies.
//
//	daggers   count $E263, index $E262, 17 bytes from $E264 ($5B17)
//	jewels    count $E1F3, index $E1F4,  9 bytes from $E1F5 ($65B5)
//	doors     4,           index $E1F4,  7 bytes from $E1C5 ($675B)
//	picks     count $E2CB, index $E1F4,  5 bytes from $E2CC ($5C9E)
//	walls     count $E31D, index $E31C,  7 bytes from $E31E ($6A1C)
//	traps     count $E3FE, index $E3FD,  5 bytes from $E3FF ($68D4)
//	enemies   count $E164, index $E165, 22 bytes from $E16A ($73DF)

// obj is $5B17: dagger $E262's byte off.
func (c *Console) obj(off byte) uint16 {
	n := c.rd(0xE262)
	return 0xE264 + uint16(off) + uint16(n*17)
}

// jewel is $65B5: jewel $E1F4's byte off.
func (c *Console) jewel(off byte) uint16 {
	n := c.rd(0xE1F4)
	return 0xE1F5 + uint16(off) + uint16(n*9)
}

// door is $675B: door $E1F4's byte off, returned as its value.
func (c *Console) door(off byte) byte { return c.rd(c.doorAddr(off)) }

func (c *Console) doorAddr(off byte) uint16 {
	n := c.rd(0xE1F4)
	return 0xE1C5 + uint16(off) + uint16(n*7)
}

// pick is $5C9E: pick $E1F4's first byte.
func (c *Console) pick() uint16 {
	n := c.rd(0xE1F4)
	return 0xE2CC + uint16(n*5)
}

// wall is $6A1C: wall $E31C's byte off.
func (c *Console) wall(off byte) uint16 {
	n := c.rd(0xE31C)
	return 0xE31E + uint16(off) + uint16(n*7)
}

// trap is $68D4: trap $E3FD's byte off.
func (c *Console) trap(off byte) uint16 {
	n := c.rd(0xE3FD)
	return 0xE3FF + uint16(off) + uint16(n*5)
}

// spot reads the position of the object whose Y is at hl: D = Y, E = X
// (hl+2), and reports whether it is on the screen shown ($E13A = hl+3).
// It returns hl+3. Ported from $5C04-$5C10. High.
func (c *Console) spot(hl uint16) (d, e byte, on bool, end uint16) {
	d = c.rd(hl)
	e = c.rd(hl + 2)
	return d, e, c.rd(0xE13A) == c.rd(hl+3), hl + 3
}

// drawAt writes tile a at the name-table position of the object whose
// Y is at hl+2, if it is on the screen shown. It returns whether it did
// (Z) and the name-table address. Ported from $5C11-$5C1E. High.
func (c *Console) drawAt(hl uint16, a byte) (bool, uint16) {
	d, e, on, end := c.spot(hl + 2)
	if !on {
		return false, end
	}
	na := nameAddr(d, e)
	c.wrtvrm(na, a)
	return true, na
}

// boxHit is $5D0B: carry when (b, cc) lies within the box around
// (e, d) given by the four bytes at hl (x offset, x width, y offset,
// y width). Ported from $5D0B-$5D18. High.
func (c *Console) boxHit(hl uint16, b, cc, d, e byte) bool {
	a := b - e - c.rd(hl)
	if _, f := add8(a, c.rd(hl+1), false); !f.C {
		return false
	}
	a = cc - d - c.rd(hl+2)
	_, f := add8(a, c.rd(hl+3), false)
	return f.C
}

// playerBox is $5CFD: boxHit against the player ($E137, $E139).
func (c *Console) playerBox(hl uint16, d, e byte) bool {
	return c.boxHit(hl, c.rd(0xE139), c.rd(0xE137), d, e)
}

// --- daggers ---

// daggersUpdate runs every dagger's state. Ported from $57FF-$581F and
// $5B00-$5B0A. High.
//
//	0 lying (placed into the map)   5 flying
//	1-3 idle (held: 2)              6 turning back off a wall
//	4 thrown (launch)               7 bouncing, 8 dropping
func (c *Console) daggersUpdate() {
	c.wr(0xE262, 0)
	for {
		switch c.rd(c.obj(0)) {
		case 0:
			c.daggerPlace()
		case 1, 2, 3:
		case 4:
			c.daggerLaunch()
		case 5:
			c.daggerFly()
		case 6:
			c.daggerTurn()
		case 7:
			c.daggerBounce()
		case 8:
			c.daggerDrop()
		default:
			panic("engine: dagger state out of range")
		}
		if c.inc(0xE262) == c.rd(0xE263) {
			return
		}
	}
}

// daggerPlace puts a lying dagger into the map: its cell is saved in
// +10 (unless already a $3x cell) and becomes $30 ($31/$32 on the stair
// cells $21/$22, kept on $31/$32); tile $4B is drawn. Ported from
// $5820-$5858. High.
func (c *Console) daggerPlace() {
	c.inc(c.obj(0))
	_, de := c.mapAt(c.obj(0) + 2)
	b := c.rd(de)
	if b&0xF0 != 0x30 {
		c.wr(c.obj(0x0A), b)
	}
	a := b
	if b-0x31 >= 2 {
		if b-0x21 < 2 {
			a = b + 0x10
		} else {
			a = 0x30
		}
	}
	c.wr(de, a)
	c.drawAt(c.obj(0), 0x4B)
}

// daggerLaunch starts a thrown dagger from the player: speed $0200,
// position and facing copied, X snapped to the cell ahead (+4), and the
// two map cells it covers saved in +10, +11. Ported from $585A-$589F.
// High.
func (c *Console) daggerLaunch() {
	c.soundStart(0x06)
	c.ldir(c.obj(6), 0x6588, 0x0B)
	c.inc(c.obj(0))
	hl := c.obj(1)
	f := c.rd(0xE136)
	c.ldir(hl, 0xE136, 5)
	b := byte(8)
	if f&1 != 0 {
		b = 0
	}
	c.wr(hl+3, (c.rd(0xE139)+b)&0xF8|0x04)
	_, cell := c.mapAt(hl + 1)
	d := c.obj(0x0A)
	c.ldir(d, cell, 2)
}

// daggerTile is the flying dagger's tile for its X: $58B1 by (X>>2)&3.
// Ported from $58A0-$58B0. High.
func (c *Console) daggerTile() byte {
	return c.rd(0x58B1 + uint16(c.rd(c.obj(4))>>2&3))
}

// daggerFly moves a flying dagger, draws it, restores the cells it
// leaves, and stops it at a wall, a stair or an enemy. Ported from
// $58B6-$59E7. High.
func (c *Console) daggerFly() {
	c.move(c.obj(3), c.rd16(c.obj(6)))
	x := c.rd(c.obj(4))
	if x&3 != 0 {
		return
	}
	if x&7 == 0 {
		c.daggerCross()
		return
	}
	if x < 8 || x >= 0xFC {
		return
	}
	t := c.daggerTile()
	if ok, na := c.drawAt(c.obj(0), t); ok {
		c.wrtvrm(na+1, t+1)
	}
	hl := c.obj(1)
	cell := c.rd(hl)
	_, p := c.mapAt(hl + 1)
	if cell&1 == 0 {
		p++
	}
	if c.rd(p)&0xF0 != 0x50 {
		return
	}
	c.inc(hl - 1)
	de := c.obj(0x0A)
	ok, na := c.drawAt(hl-1, c.mapTile(c.rd(de)))
	if !ok {
		return
	}
	c.wrtvrm(na+1, c.mapTile(c.rd(de+1)))
}

// daggerCross handles a flying dagger crossing into a new cell (X on an
// 8-pixel boundary). Ported from $591B-$59E7. High.
func (c *Console) daggerCross() {
	ix := c.obj(1)
	t := c.daggerTile()
	c.drawAt(c.obj(0), t)
	_, cell := c.mapAt(c.obj(2))
	f := c.rd(ix)
	cell++
	if f&1 != 0 {
		cell -= 2
	}
	a := c.rd(cell)
	var d byte
	if f&1 != 0 {
		d = c.rd(ix + 0x0A)
		c.wr(ix+0x0A, c.rd(ix+9))
		c.wr(ix+9, a)
	} else {
		d = c.rd(ix + 9)
		c.wr(ix+9, c.rd(ix+0x0A))
		c.wr(ix+0x0A, a)
	}
	var e byte
	if c.rd(0xE13A) == c.rd(ix+4) {
		tile := c.mapTile(d)
		p := c.obj(2)
		d = c.rd(p)
		e = c.rd(p + 2)
		na := nameAddr(d, e) - 1
		if c.rd(ix)&1 != 0 {
			na += 2
		}
		c.wrtvrm(na, tile)
	}
	var hl uint16
	if c.daggerHit(ix) {
		hl = c.obj(0)
	} else {
		p := c.obj(2)
		_, q := c.mapAt(p)
		q--
		if c.rd(ix)&1 == 0 {
			q += 2
		}
		b := c.rd(q)
		if _, z := classSolid(b); !z || b == 0x40 {
			// $59D9: at the screen edge the dagger's cells are
			// redrawn as it leaves.
			b := c.rd(p - 1)
			hl = p + 2
			if x := c.rd(hl); x != 0 && x != 0xF8 {
				return
			}
			c.daggerRedraw(hl, b, d, e)
			return
		}
		hl = p - 2
	}
	c.inc(hl)
	b := c.rd(hl + 1)
	c.daggerRedraw(hl+4, b, d, e)
}

// daggerRedraw is $59BF: on the screen shown, redraw the map cell the
// dagger covered (+10, or +11 facing left) at (d, e). hl is the dagger's
// +4.
func (c *Console) daggerRedraw(hl uint16, b, d, e byte) {
	hl++
	if c.rd(0xE13A) != c.rd(hl) {
		return
	}
	hl += 5
	if b&1 != 0 {
		hl++
	}
	c.wrtvrm(nameAddr(d, e), c.mapTile(c.rd(hl)))
}

// daggerHit tests the dagger against every enemy that can be hit
// (states below 4, or 7): a hit scores 100, plays sound 8 and starts
// the enemy's death (state 6). Ported from $5B48-$5BA8. High.
func (c *Console) daggerHit(ix uint16) bool {
	for n := byte(0); ; {
		e := c.enemy(n)
		if s := c.rd(e); s < 4 || s == 7 {
			p := c.obj(2)
			d, ex := c.rd(p), c.rd(p+2)
			if c.rd(p+3) == c.rd(e+6) {
				if c.boxHit(0x5BA9, c.rd(e+5), c.rd(e+3), d, ex) {
					c.addScore(0x0100)
					c.soundStart(0x08)
					c.wr(e, 6)
					c.wr(e+1, 4)
					c.wr(e+3, c.rd(e+3)&0xF8)
					c.wr(e+0x11, 0x22)
					return true
				}
			}
		}
		n++
		if n == c.rd(0xE164) {
			return false
		}
	}
}

// daggerTurn turns a dagger that hit a wall: it bounces back, X to the
// cell, arc step 0. Ported from $59E8-$5A03. High.
func (c *Console) daggerTurn() {
	c.inc(c.obj(0))
	hl := c.obj(1)
	a := c.rd(hl) ^ 3
	c.wr(hl, a)
	x := c.rd(hl + 3)
	if a&1 == 0 {
		x += 4
	}
	c.wr(hl+3, x&0xF8)
	c.wr(hl+8, 0)
}

// daggerBounce moves a bouncing dagger along its arc (every fourth
// frame) until it lands; on brick it lies down again. Ported from
// $5A04-$5A62 and $5AA3-$5AB9. High.
func (c *Console) daggerBounce() {
	if c.rd(0xE003)&3 != 0 {
		c.daggerSprite()
		return
	}
	hl := c.obj(1)
	x := hl + 3
	c.inc(x)
	if c.rd(hl)&1 != 0 {
		c.dec(x)
		c.dec(x)
	}
	c.screenWrap(hl)
	b := c.rd(0x5A63 + uint16(c.rd(c.obj(9))))
	p := c.obj(2)
	c.wr(p, c.rd(p)+b)
	a, _ := c.mapCell(p, 0x04, 0x08)
	cls, z := classSolid(a)
	if !z || a == 0x41 || a == 0x42 {
		if c.inc(p+7) == 8 {
			c.inc(c.obj(0))
			return
		}
		c.daggerSprite()
		return
	}
	if cls != 0x10 {
		c.daggerRearc()
		return
	}
	c.daggerLand()
}

// daggerLand hides the dagger's sprite and lays it down (state 0).
// Ported from $5AA3-$5AAB, $5AB7.
func (c *Console) daggerLand() {
	c.spriteHide()
	c.wr(c.obj(0), 0)
}

// daggerRearc restarts the arc: state 7, step 0. Ported from
// $5AAC-$5AB9.
func (c *Console) daggerRearc() {
	hl := c.obj(0)
	c.wr(hl, 7)
	c.wr(hl+9, 0)
}

// daggerDrop drops a dagger straight down 4 pixels every fourth frame.
// Ported from $5A6B-$5AC7. High.
func (c *Console) daggerDrop() {
	if c.rd(0xE003)&3 != 0 {
		c.daggerSprite()
		return
	}
	p := c.obj(2)
	c.wr(p, c.rd(p)&0xFC)
	_, cell := c.mapAt(p)
	a := c.rd(cell + 0x60)
	cls, z := classSolid(a)
	if !z || a == 0x41 || a == 0x42 {
		c.wr(p, (c.rd(p)+4)&0xFC)
		c.daggerSprite()
		return
	}
	if cls != 0x10 {
		c.daggerRearc()
		return
	}
	c.daggerLand()
}

// daggerSprite shows the moving dagger as sprite $E262 at $E0EC (pattern
// $F0-$FC by the frame counter, colour 15) when on the screen shown.
// Ported from $5AD7-$5AFF. High.
func (c *Console) daggerSprite() {
	if c.rd(0xE13A) != c.rd(c.obj(5)) {
		c.spriteHide()
		return
	}
	p := c.obj(2)
	s := c.daggerSlot()
	c.wr(s, c.rd(p))
	c.wr(s+1, c.rd(p+2))
	c.wr(s+2, c.rd(0xE003)&0x0C+0xF0)
	c.wr(s+3, 0x0F)
}

// daggerSlot is $5B3F: the dagger's sprite entry $E0EC + 4 x $E262.
func (c *Console) daggerSlot() uint16 { return 0xE0EC + uint16(c.rd(0xE262)*4) }

// spriteHide is $5B0B: Y = $E0 for the dagger's sprite.
func (c *Console) spriteHide() { c.wr(c.daggerSlot(), 0xE0) }

// screenWrap is $710B: when X (hl+3) wraps past the screen edge in the
// facing direction (hl), the screen (hl+4) changes. Ported from
// $710B-$711E. High.
func (c *Console) screenWrap(hl uint16) {
	b := byte(0)
	if c.rd(hl)&1 != 0 {
		b = 0xFF
	}
	if c.rd(hl+3) != b {
		return
	}
	s := hl + 4
	c.inc(s)
	if b != 0 {
		c.dec(s)
		c.dec(s)
	}
}

// daggerPickup lets the player, empty-handed, pick up a lying dagger it
// touches: the dagger is held (state 2), the map cell is restored and
// redrawn. Ported from $5BAD-$5C01. High.
func (c *Console) daggerPickup() {
	if c.rd(0xE144) != 0 {
		return
	}
	c.wr(0xE262, 0)
	if c.rd(0xE263) == 0 {
		return
	}
	for {
		hl := c.obj(0)
		a := c.rd(hl)
		d, e, on, _ := c.spot(hl + 2)
		if on && a == 1 && c.playerBox(0x5C4D, d, e) {
			c.wr(0xE261, c.rd(0xE262))
			c.soundStart(0x04)
			c.setItem(0x10)
			hl = c.obj(0)
			c.inc(hl)
			b := c.rd(hl + 0x0A)
			_, cell := c.mapAt(hl + 2)
			c.wr(cell, b)
			c.drawAt(hl, c.mapTile(b))
			return
		}
		if c.inc(0xE262) == c.rd(0xE263) {
			return
		}
	}
}

// --- jewels ---

// jewelsTouch collects every jewel the player touches: 500 points,
// sound 9, state 2. Ported from $5C1F-$5C4C. High.
func (c *Console) jewelsTouch() {
	c.wr(0xE1F4, 0)
	for {
		hl := c.jewel(0)
		a := c.rd(hl)
		d, e, on, _ := c.spot(hl + 2)
		if on && a&0x0F != 0 && c.playerBox(0x5C4D, d, e) {
			c.addScore(0x0500)
			c.soundStart(0x09)
			c.wr(c.jewel(1), 2)
		}
		if !c.nextJewel() {
			return
		}
	}
}

// nextJewel is $65AD: advance $E1F4; false (Z) at the jewel count.
func (c *Console) nextJewel() bool {
	return c.inc(0xE1F4) != c.rd(0xE1F3)
}

// jewelsUpdate erases each jewel collected this frame (state 2 -> 3):
// tiles and map cells cleared, the count $E1F2 advanced, and lying
// daggers re-placed. Ported from $6532-$65AC. High.
func (c *Console) jewelsUpdate() {
	c.wr(0xE1F4, 0)
	for {
		if c.rd(c.jewel(1)) == 2 {
			hl := c.jewel(0)
			c.wr(hl, c.rd(hl)&0xF0)
			c.inc(hl + 1)
			d := c.rd(hl+2) - 8
			e := c.rd(hl + 4)
			na := nameAddr(d, e)
			c.wrtvrm(na, 0)
			c.vblock(na+0x1F, 0x658A, 1, 3)
			_, cell := c.mapAt(c.jewel(2))
			c.mapStamp(cell, 0x658A)
			c.inc(0xE1F2)
			c.daggersRelay()
		}
		if !c.nextJewel() {
			return
		}
	}
}

// mapStamp writes the four bytes at de to the map around cell: one
// above, then left, centre and right. Ported from $65D7-$65E9. High.
func (c *Console) mapStamp(cell, de uint16) {
	c.ldir(cell-0x60, de, 1)
	c.ldir(cell-1, de+1, 3)
}

// daggersRelay puts every idle dagger (state 1) back to state 0, so it
// is placed into the map again. Ported from $6653-$6669. High.
func (c *Console) daggersRelay() {
	c.wr(0xE262, 0)
	b := c.rd(0xE263)
	for {
		hl := c.obj(0)
		if c.rd(hl) == 1 {
			c.wr(hl, 0)
		}
		c.inc(0xE262)
		b--
		if b == 0 {
			return
		}
	}
}

// --- picks ---

// picksTouch lets the player, empty-handed, pick up a pick it touches.
// Ported from $5C51-$5C9D. High.
func (c *Console) picksTouch() {
	if c.rd(0xE144) != 0 {
		return
	}
	if c.rd(0xE2CB) == 0 {
		return
	}
	c.wr(0xE1F4, 0)
	for {
		hl := c.pick()
		a := c.rd(hl)
		d, e, on, _ := c.spot(hl + 1)
		if on && a != 0 && c.playerBox(0x5C4D, d, e) {
			c.soundStart(0x04)
			hl = c.pick()
			c.wr(hl, 0)
			_, cell := c.mapAt(hl + 1)
			c.wr(cell, 0)
			c.drawAt(hl-1, 0)
			c.wr(0xE2CA, c.rd(0xE1F4))
			c.setItem(0x20)
			return
		}
		if c.inc(0xE1F4) == c.rd(0xE2CB) {
			return
		}
	}
}

// --- doors ---

// doorsUpdate runs the four doors (state in +4's high nibble). It
// reports true when the player went through an exit door: the original
// then pops its caller's return address too. Ported from $65EA-$6609
// and $6767-$6771. High.
func (c *Console) doorsUpdate() bool {
	c.wr(0xE1F4, 0)
	c.wr(0xE1C2, 0)
	for {
		if c.door(0) != 0xFF {
			switch c.door(4) >> 4 {
			case 0:
				c.doorWait()
			case 1:
				c.doorOpen()
			case 2:
				c.doorEnter()
			case 3, 5:
				c.doorAnimate(false)
			case 4:
				if c.doorExit() {
					return true
				}
			case 6:
			case 7:
				c.doorCleared()
			case 8:
				c.doorDraw0(false)
			default:
				panic("engine: door state out of range")
			}
		}
		if c.inc(0xE1F4) == 4 {
			return false
		}
	}
}

// doorWait waits for every jewel to be collected; then the door starts
// opening (state 1), and the walls slide away. Ported from $661B-$6636.
// High.
func (c *Console) doorWait() {
	if c.rd(0xE1F2) != c.rd(0xE1F3) {
		return
	}
	c.wr(0xE133, 1)
	c.soundStart(0x94)
	c.wallsOpen()
	c.wr(c.doorAddr(4), 0x10)
}

// doorOpen draws the open door (state 2), with the inside graphic in
// the intro (state 4). Ported from $6637-$6652. High.
func (c *Console) doorOpen() {
	c.doorDraw0(c.rd(0xE000) == 4)
}

// doorDraw0 is $6644 with graphic 2 (intro) or 0: state 2, the door
// drawn into the map and screen, and outside the intro the idle daggers
// re-placed. State 8 enters here with graphic 0. Ported from
// $6642-$6652. High.
func (c *Console) doorDraw0(intro bool) {
	g := byte(0)
	if intro {
		g = 2
	}
	de := c.doorGfx(g)
	c.wr(c.doorAddr(4), 0x20)
	c.doorDraw(de)
	if intro {
		return
	}
	c.daggersRelay()
}

// doorEnter: while the player jumps into an open door, it starts
// closing behind them (state 3). Ported from $666A-$6693. High.
func (c *Console) doorEnter() {
	if c.rd(0xE134) != 1 {
		return
	}
	d, e, on, _ := c.spot(c.doorAddr(0))
	if !on {
		return
	}
	d -= 8
	e -= 0x10
	if !c.playerBox(0x67D0, d, e) {
		return
	}
	c.wr(0xE133, 1)
	c.wr(0xE003, 0x14)
	c.wr(c.doorAddr(4), 0x30)
	c.doorStep()
}

// doorAnimate is states 3 and 5: every 32 frames the door's frame
// advances. Ported from $6694-$6699. High.
func (c *Console) doorAnimate(bool) {
	if c.rd(0xE003)&0x1F != 0 {
		return
	}
	c.doorStep()
}

// doorStep advances the door animation: after 4 frames state 7 (intro)
// or the next state, with $E1C2 counting doors done; otherwise the frame
// is drawn (state 5 runs backwards), with sound $8D on frame 1. Ported
// from $669A-$66DF. High.
func (c *Console) doorStep() {
	c.daggersRelay()
	hl := c.doorAddr(4)
	c.inc(hl)
	a := c.rd(hl)
	if a&0x0F == 4 {
		if c.rd(0xE000) == 4 {
			c.wr(hl, 0x70)
		} else {
			c.wr(hl, a+0x10)
		}
		c.inc(0xE1C2)
		return
	}
	if a&0xF0 == 0x50 {
		a = -(a&0x0F - 4)
	}
	a = a&0x0F - 1
	if a == 1 {
		c.soundStart(0x8D)
	}
	c.doorDraw(c.doorGfx(a))
}

// doorExit: the player reaching an open exit door leaves the pyramid
// (the exit sequence, $E132 = 1) and the entry door is remembered.
// Ported from $66E0-$6744. High.
func (c *Console) doorExit() bool {
	d, e, on, _ := c.spot(c.doorAddr(0))
	if !on {
		return false
	}
	if !c.playerBox(0x67D4, d, e) {
		return false
	}
	c.spritesOff(0xE0C8, 0x0A)
	hl := c.doorAddr(4)
	c.wr(hl, 0x50)
	c.ldir(0xE055, hl+1, 2)
	a := c.rd(0xE1F4) + 1
	if a != 1 {
		a = (a - 1) * 2
		if a == 6 {
			a = 8
		}
	}
	c.wr(0xE057, a)
	c.doorSprites(d, e)
	c.soundStart(0x20)
	c.wr(0xE130, 0)
	c.wr(0xE132, 1)
	c.wr(0xE136, 2)
	return true
}

// doorSprites places the four door sprites ($E0B0) around (d, e).
// Ported from $6713-$6730 (and $6F28-$6F45). High.
func (c *Console) doorSprites(d, e byte) {
	hl := uint16(0xE0B0)
	e += 0x10
	d -= 0x11
	for b := 0; b < 2; b++ {
		for k := 0; k < 2; k++ {
			c.wr(hl, d)
			c.wr(hl+1, e)
			hl += 4
		}
		d += 0x10
	}
}

// doorCleared is state 7: the door shows closed (state 0) unless this
// pyramid was cleared before (state 2). Ported from $6745-$675A. High.
func (c *Console) doorCleared() {
	cleared := c.pyramidCleared()
	hl := c.doorAddr(4)
	c.wr(hl, 0)
	if cleared {
		c.wr(hl, 0x20)
		return
	}
	c.doorDraw(0x658A)
}

// doorGfx is $67C7: the door graphic a from the table at $67D8.
func (c *Console) doorGfx(a byte) uint16 { return c.rd16(0x67D8 + uint16(a*2)) }

// doorDraw writes the 5x3 door graphic at de into the map (from 8 up
// and 16 left of the door) and, on the screen shown, the name table.
// Ported from $6772-$67BE. High.
func (c *Console) doorDraw(de uint16) {
	_, cell := c.mapCell(c.doorAddr(0), 0xF0, 0xF8)
	g := de
	for b := 0; b < 3; b++ {
		c.ldir(cell, g, 5)
		cell += 5 + 0x5B
		g += 5
	}
	d, e, on, _ := c.spot(c.doorAddr(0))
	if !on {
		return
	}
	na := nameAddr(d, e) - 0x22
	g = de
	for b := 0; b < 3; b++ {
		for k := 0; k < 5; k++ {
			c.wrtvrm(na, c.mapTile(c.rd(g)))
			na++
			g++
		}
		na += 0x1B
	}
}

// --- traps (falling blocks) ---

// trapsUpdate runs the falling-block traps: a trap waits until the
// player stands exactly at its X and Y, then drops a block from the
// first solid cell above, 4 pixels every 32 frames, until it lands; a
// player standing under it is crushed. Ported from $680B-$68F6. High.
func (c *Console) trapsUpdate() {
	c.wr(0xE3FD, 0)
	if c.rd(0xE3FE) == 0 {
		return
	}
	for {
		c.trapStep()
		if c.inc(0xE3FD) == c.rd(0xE3FE) {
			return
		}
	}
}

func (c *Console) trapStep() {
	hl := c.trap(0)
	switch c.rd(hl) {
	case 0:
		c.trapArm(hl)
		return
	case 1:
	default:
		return
	}
	if c.rd(0xE003)&0x1F != 0 {
		return
	}
	hl++
	y := c.rd(hl) + 4
	c.wr(hl, y)
	cc := byte(0x19)
	if y&7 == 0 {
		cc++
	}
	_, de := c.mapAt(hl)
	if c.rd(hl)&7 == 0 && c.rd(de) != 0 {
		a := c.rd(de)
		if a != 1 && a&0xF0 != 0x10 {
			c.wr(hl, c.rd(hl)-4)
			return
		}
		c.inc(hl - 1)
		cc = 0x13
		if a == 0x14 {
			cc++
		}
		c.trapCell(hl, de, cc)
		return
	}
	c.trapCell(hl, de, cc)
	if c.rd(0xE134) != 0 {
		return
	}
	if c.trapUnder(0x05) || c.trapUnder(0x0B) {
		c.soundStart(0x1D)
		c.wr(0xE053, 0)
	}
}

// trapCell writes cell value cc at de and draws it. Ported from
// $6882-$688E. High.
func (c *Console) trapCell(hl, de uint16, cc byte) {
	c.wr(de, cc)
	c.drawAt(hl-2, c.mapTile(cc))
}

// trapUnder is $68EC: carry when the cell 0 down, b across from the
// player is a falling block ($19/$1A).
func (c *Console) trapUnder(b byte) bool {
	_, de, _ := c.probe(0xE137, b, 0)
	return c.rd(de)-0x19 < 2
}

// trapArm starts a trap when the player is at its position. Ported
// from $688F-$68D3. High.
func (c *Console) trapArm(hl uint16) {
	hl++
	de := hl
	if c.rd16(0xE139) != c.rd16(hl+2) {
		return
	}
	if c.rd(de) != c.rd(0xE137) {
		return
	}
	_, cell := c.mapAt(de)
	b := byte(0)
	for {
		if cell < 0xE760 {
			c.wr(cell, 0x12)
			break
		}
		if c.rd(cell) != 0 {
			break
		}
		b++
		cell -= 0x60
	}
	c.wr(de, c.rd(de)-b*8)
	c.inc(de - 1)
}

// --- digging ---

// digStep digs into the cell at the dig point ($E149): on counts 9, $0C
// and 3 the brick becomes empty (1), and the dig tile for the count is
// drawn. A cell that cannot be dug ends the dig. Ported from
// $68F7-$694A. High.
func (c *Console) digStep() {
	hl := uint16(0xE148)
	a := c.rd(hl)
	if a == 0 {
		return
	}
	hl++
	if a != 0x12 {
		c.wr(hl, c.rd(hl)+3)
	}
	b := a
	_, de := c.mapAt(hl)
	if b == 9 {
		v := c.rd(de)
		if v>>4 != 1 {
			c.wr(0xE148, 0)
			return
		}
		if lo := v & 0x0F; lo >= 4 && lo != 9 {
			c.wr(0xE148, 0)
			return
		}
	}
	if v := c.rd(hl - 1); v == 0x0C || v == 0x03 {
		c.wr(de, 1)
	}
	t := c.rd(0x6593 + uint16(c.rd(0xE148)))
	c.drawAt(hl-2, t)
	c.soundStart(0x45)
}

// --- sliding walls ---

// wallsUpdate animates the first active sliding wall, every 8 frames:
// 5 frames of tiles from $6A7B, then it is deactivated, its direction
// flips and it is written into the map. Ported from $694B-$69AE. High.
func (c *Console) wallsUpdate() {
	hl := uint16(0xE31E)
	b := c.rd(0xE31D)
	if b == 0 {
		return
	}
	for c.rd(hl)&1 == 0 {
		hl += 7
		b--
		if b == 0 {
			return
		}
	}
	if c.rd(0xE003)&7 != 0 {
		return
	}
	ix := hl
	a := c.rd(ix + 6)
	c.inc(ix + 6)
	if a == 5 {
		c.wr(ix, c.rd(ix)&^1)
		c.wr(ix+5, c.rd(ix+5)^0x0C)
		c.wallMap(ix)
		return
	}
	bb := a * 8
	if c.rd(ix+5) != 8 {
		a = 0x20 - bb
	} else {
		a = bb
	}
	de := 0x6A7B + uint16(a)
	rows := c.rd(ix)>>1 + 2
	if c.rd(0xE13A) != c.rd(ix+4) {
		return
	}
	c.vblock(nameAddr(c.rd(ix+1), c.rd(ix+3)), de, int(rows), 2)
}

// wallMap writes the wall's cells into the map: (rows) x 2 cells of
// $50/$51 (or $52/$53 with direction bit 2). Ported from $69AF-$69DB.
// High.
func (c *Console) wallMap(ix uint16) {
	_, cell := c.mapAt(ix + 1)
	rows := c.rd(ix)>>1&3 + 2
	cc := byte(0x50)
	if c.rd(ix+5)&0x04 != 0 {
		cc += 2
	}
	c.wallCells(cell, rows, cc)
}

// wallCells is $69CF: rows of (cc, cc+1) down from cell.
func (c *Console) wallCells(cell uint16, rows, cc byte) {
	for ; rows > 0; rows-- {
		c.wr(cell, cc)
		c.wr(cell+1, cc+1)
		cell += 0x60
	}
}

// doorPush starts the sliding wall the player pushes against: the one
// whose position is just ahead. Ported from $69DC-$6A1B. High.
func (c *Console) doorPush() {
	c.wr(0xE31C, 0)
	for {
		hl := c.wall(0)
		a := c.rd(hl)
		d := a*4&0x18 + c.rd(hl+1)
		o := byte(8)
		if c.rd(0xE136)&1 == 0 {
			o = 0xF8
		}
		e := o + c.rd(hl+3)
		if uint16(c.rd(0xE137))<<8|uint16(c.rd(0xE139)) == uint16(d)<<8|uint16(e) {
			c.wr(hl, c.rd(hl)|1)
			c.wr(hl+6, 0)
			return
		}
		if c.inc(0xE31C) == c.rd(0xE31D) {
			return
		}
	}
}

// wallsOpen clears every sliding wall from the map (cells 0/1) and the
// screen. Ported from $6A31-$6A70. High.
func (c *Console) wallsOpen() {
	if c.rd(0xE31D) == 0 {
		return
	}
	c.wr(0xE31C, 0)
	hl := uint16(0xE31E)
	for {
		rows := c.rd(hl)>>1&3 + 2
		_, cell := c.mapAt(hl + 1)
		c.wallCells(cell, rows, 0)
		d, e, on, _ := c.spot(hl + 1)
		if on {
			c.vblock(nameAddr(d, e), 0x658A, int(rows), 2)
		}
		hl += 7
		if c.inc(0xE31C) == c.rd(0xE31D) {
			return
		}
	}
}

// pyramidCleared is $6D51: NZ when this pyramid's bit ($E054 - 1) is
// set in $E062/$E063.
func (c *Console) pyramidCleared() bool {
	m := c.pyramidBit()
	v := c.rd16(0xE062)
	return byte(v>>8)&byte(m>>8)+byte(v)&byte(m) != 0
}

// pyramidBit is $6523: 1 << ($E054 - 1) (with $E054 = 0: 255 shifts).
func (c *Console) pyramidBit() uint16 {
	de := uint16(1)
	for b := c.rd(0xE054) - 1; b != 0; b-- {
		de <<= 1
	}
	return de
}
