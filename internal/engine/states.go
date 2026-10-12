package engine

// The top-level states ($E000) and their sub-states ($E001), dispatched
// from $40A3 through the table at $40BA. Sub-states are switched with
// DJNZ chains on B = $E001: an inefficient switch on the current value.
//
//	0 $40D0 logo (scrolls up), pause, title letters
//	1 $410D title (waits $E004 frames, then the demo)
//	2 $4115 demo set-up
//	3 $429F game start ("PLAY START" flashing)
//	4 $4141 pyramid intro / stage set-up (demo and game)
//	5 $41D5 play
//	6 $41F5 life lost (waits for the tune)
//	7 $4243 game over
//	8 $4266 next life
//	9 $427B leaving a pyramid
//	10 $429C ending
//
// $E003 counts frames, $E004 is the state timer, $E002 bit 6 is "in a
// game" (clear in the demo and attract).

// stateLogo is state 0. Ported from $40D0-$410C. High.
func (c *Console) stateLogo(sub byte) {
	switch sub {
	case 1:
		if c.rd(0xE003)&1 == 0 {
			return
		}
		if !c.logoScroll() {
			return
		}
		c.rleAddr(0x47FE)
		c.nextSub(0)
	case 2:
		if c.dec(0xE004) != 0 {
			return
		}
		c.clearScreen()
		c.wrtvdp(7, 0xE0)
		c.wr(0xE00A, 0)
		c.incSub()
	case 3:
		if c.titleLetter() {
			return
		}
		c.nextState(0)
	default: // 0, and 4 and up
		c.clearScreen()
		c.loadGraphics()
		c.vdpRegs()
		c.incSub()
	}
}

// nextSub sets the state timer and moves to the next sub-state. Ported
// from $4139-$4140.
func (c *Console) nextSub(timer byte) {
	c.wr(0xE004, timer)
	c.incSub()
}

// incSub is $413C: INC ($E001).
func (c *Console) incSub() { c.inc(0xE001) }

// nextState sets the state timer, moves to the next state and resets the
// sub-state. Ported from $41AD-$41B8.
func (c *Console) nextState(timer byte) {
	c.wr(0xE004, timer)
	c.inc(0xE000)
	c.wr(0xE001, 0)
}

// setState sets the state and the timer $20, sub-state 0. Ported from
// $4238-$4242.
func (c *Console) setState(s byte) {
	c.wr(0xE000, s)
	c.wr(0xE004, 0x20)
	c.wr(0xE001, 0)
}

// stateTitle is state 1: after $E004 frames, the demo. Ported from
// $410D-$4114. High.
func (c *Console) stateTitle() {
	if c.dec(0xE004) != 0 {
		return
	}
	c.nextState(0x20)
}

// stateDemoStart is state 2: demo set-up, then state 4. Ported from
// $4115-$4138. High.
func (c *Console) stateDemoStart() {
	c.wr16(0xE000, 0x0004)
	c.wr16(0xE062, 0x0000)
	c.wr(0xE058, 0)
	c.newGameVars()
	c.wr16(0xE055, 0x0805)
	c.wr16(0xE080, 0x4ACE)
	c.wr(0xE082, 8)
}

// stateStart is state 3: "PLAY START" flashes, then a new game. Ported
// from $429F-$42C4. High.
func (c *Console) stateStart(sub byte) {
	if sub == 1 {
		v := c.dec(0xE004)
		if v == 0 {
			c.clearGameRAM()
			c.nextState(0x20)
			return
		}
		if v&0x04 != 0 {
			c.text(0x47E1, 0x00)
		} else {
			c.text(0x47E1, 0xFF)
		}
		return
	}
	c.soundStart(0x97)
	c.wr(0xE004, 0x50)
	c.incSub()
}

// clearGameRAM clears $E049-$E130 and sets the new-game variables.
// Ported from $430A-$4316, falling into $4317. High.
func (c *Console) clearGameRAM() {
	for a := uint16(0xE049); a <= 0xE049+0xE7; a++ {
		c.wr(a, 0)
	}
	c.newGameVars()
}

// newGameVars copies the 7 bytes at $4323 to $E050 (lives, ...).
// Ported from $4317-$4322. High.
func (c *Console) newGameVars() { c.ldir(0xE050, 0x4323, 7) }

// stateIntro is state 4: the pyramid set-up. With $E051 bit 0 clear
// (between pyramids) it is stateIntroB. Ported from $4141-$41B8. High.
func (c *Console) stateIntro(sub byte) {
	if c.rd(0xE051)&1 == 0 {
		c.stateIntroB(sub)
		return
	}
	a := sub // $4148: LD A,B
	if sub != 0 {
		a = c.flashExits()
	}
	switch sub {
	case 1:
		if c.dec(0xE004) != 0 {
			return
		}
		c.nextSub(a) // A is stale: the last fill byte of $4BE4
	case 2:
		c.drawSprites()
		c.doorSequence()
	case 3:
		if c.dec(0xE004) != 0 {
			return
		}
		c.wr(0xE133, 0)
		c.doorsUpdate()
		if c.rd(0xE002)&0x40 != 0 {
			c.soundStart(0x8B)
		}
		c.wr(0xE053, 1)
		c.nextState(0x20)
	default: // 0, and 4 and up
		if !c.wipeColumn() {
			return
		}
		c.spritesOff(0xE0B0, 0x20)
		c.dec(0xE050)
		// $416D: LD A,($E05E) / OR A / JR NZ,$4173: both ways continue.
		c.stageSetup()
		c.doorsUpdate()
		c.playerEnter()
		c.playerSprite()
		c.drawSprites()
		c.playerAppear()
		c.drawHUD()
		c.nextSub(0x10)
	}
}

// stateIntroB is state 4 with $E051 bit 0 clear. Ported from
// $41B9-$41D4. Medium.
func (c *Console) stateIntroB(sub byte) {
	if sub == 1 {
		if c.dec(0xE004) != 0 {
			return
		}
		c.drawScoreLine()
		c.mapScreenInit()
		c.wr(0xE053, 1)
		c.nextState(0x20)
		return
	}
	if !c.wipeColumn() {
		return
	}
	c.dec(0xE050)
	c.nextSub(1)
}

// wipeColumn erases one name-table column per tick (from the right,
// column $1F - $E004) and hides the sprites. The frame counter does not
// advance meanwhile. It reports true once $E004 has gone negative.
// Ported from $4330-$4348 (falling into $4349). High.
func (c *Console) wipeColumn() bool {
	c.dec(0xE003)
	v := c.dec(0xE004)
	if v&0x80 != 0 {
		return true
	}
	hl := uint16(0x3800) | uint16(v^0x1F)
	for b := 0; b < 0x18; b++ {
		c.wrtvrm(hl, 0)
		hl += 0x20
	}
	c.hideSprites()
	return false
}

// statePlay is state 5. Ported from $41D5-$41F4. Medium.
func (c *Console) statePlay() {
	if c.rd(0xE051)&1 != 0 {
		c.playTick()
	} else {
		c.mapScreenTick()
	}
	if c.rd(0xE00D) != 0 {
		c.wr(0xE000, 7)
		c.inc(0xE000)
		c.wr(0xE001, 0)
		return
	}
	if c.rd(0xE053) != 0 {
		return
	}
	c.nextState(0x20)
}

// stateLifeLost is state 6: once channel 1 is silent, either "GAME
// OVER" (no lives left) or the next life. Ported from $41F5-$4242. High.
func (c *Console) stateLifeLost() {
	if c.soundRead(0xE010) != 0 {
		return
	}
	if c.rd(0xE002)&0x40 == 0 {
		c.setState(0) // the demo ended: back to the logo
		return
	}
	if c.rd(0xE050) != 0 {
		c.setState(4)
		return
	}
	hl := uint16(0x3929)
	for b := 0; b < 5; b++ {
		c.vfill(hl, 0x0C, 0)
		hl += 0x20
	}
	c.soundStart(0x9A)
	c.text(0x47F2, 0xFF)
	c.wr(0xE000, 6)
	c.nextState(0xB8)
}

// stateGameOver is state 7: on odd frames, $E004 counts down; then the
// attract continuation runs once more, and the game is over. Ported
// from $4243-$4265. High.
func (c *Console) stateGameOver() {
	if c.rd(0xE003)&1 == 0 {
		return
	}
	if c.dec(0xE004) != 0 {
		return
	}
	c.attractKeys()
	if c.rd(0xE000) == 7 {
		c.wr(0xE002, c.rd(0xE002)&0xBF)
		c.setState(0)
		return
	}
	c.wr(0xE002, c.rd(0xE002)&0xBF)
}

// stateNextLife is state 8: a life is used, the life counter $E051 (BCD)
// advances. Ported from $4266-$427A. Medium.
func (c *Console) stateNextLife() {
	c.soundStart(0x20)
	c.inc(0xE050)
	v, _ := bcdAdd(c.rd(0xE051), 1, false)
	c.wr(0xE051, v)
	c.wr(0xE00D, 0)
	c.setState(4)
}
