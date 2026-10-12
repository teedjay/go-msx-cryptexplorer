package engine

// Hooks for the frontend's cheats (internal/session/cheat.go). They sit
// over the port: nothing here runs unless a cheat calls it, so every
// trace still matches.

// Pyramids is the number of pyramids in a loop.
const Pyramids = 15

// EntryDoor returns the door value ($E056) the game uses when the player
// arrives at pyramid n (1-15) through a door of pyramid n-1, derived
// from the pyramid data the builder ($6AA3) reads: each door record's
// third byte holds the destination pyramid (high nibble) and door (low
// nibble). It falls back to any door leading to n, then to the new-game
// value 8.
func (c *Console) EntryDoor(n byte) byte {
	fallback := byte(8)
	if n == 1 {
		return fallback
	}
	for _, from := range []byte{n - 1, 0} {
		for p := byte(1); p <= Pyramids; p++ {
			if from != 0 && p != from {
				continue
			}
			for _, d := range c.pyramidDoors(p) {
				if d>>4 == n {
					return d & 0x0F
				}
			}
		}
	}
	return fallback
}

// pyramidDoors returns the destination bytes of pyramid p's doors, read
// the way $6AA3 walks the data: the section bytes (up to four, ending
// early after a $3x one), then four door records of three bytes, or a
// single $FF for none.
func (c *Console) pyramidDoors(p byte) []byte {
	de := c.rd16(0x6D70 + uint16((p-1)*2))
	for b := 4; b > 0; b-- {
		a := c.rd(de)
		de++
		if a&0xF0 == 0x30 {
			break
		}
	}
	var out []byte
	for n := 0; n < 4; n++ {
		if c.rd(de) == 0xFF {
			de++
			continue
		}
		out = append(out, c.rd(de+2))
		de += 3
	}
	return out
}

// StartPyramid sets the pyramid the intro is about to build (state 4,
// sub-state 0, before the wipe ends): $E055 and its entry door $E056.
// It reports false outside that window.
func (c *Console) StartPyramid(n byte) bool {
	if c.rd(0xE002)&0x40 == 0 || c.rd(0xE000) != 4 || c.rd(0xE001) != 0 || c.rd(0xE051)&1 == 0 {
		return false
	}
	c.wr(0xE055, n)
	c.wr(0xE056, c.EntryDoor(n))
	return true
}

// InGame reports whether a game is running ($E002 bit 6), and State the
// top-level state.
func (c *Console) InGame() bool { return c.rd(0xE002)&0x40 != 0 }

// State is $E000.
func (c *Console) State() byte { return c.rd(0xE000) }

// RAMAt reads a game RAM byte (for tests and tools).
func (c *Console) RAMAt(a uint16) byte { return c.rd(a) }

// MapScreen returns a copy of VRAM with the name table showing map
// screen s (0-2) as the game would draw it (rows 1-22), for tools that
// look at a whole pyramid.
func (c *Console) MapScreen(s int) [0x4000]byte {
	v := c.VDP.VRAM
	for r := 0; r < 0x16; r++ {
		for x := 0; x < 0x20; x++ {
			v[0x3820+r*0x20+x] = c.mapTile(c.rd(uint16(0xE760 + r*0x60 + s*0x20 + x)))
		}
	}
	return v
}

// MapCell is the map byte at column x, row y (for tools).
func (c *Console) MapCell(x, y int) byte { return c.rd(uint16(0xE700 + y*0x60 + x)) }

// Poke writes a game RAM byte: the pyramid-select cheat's writes, as a
// recorded schedule replays them into the original and the comparator.
func (c *Console) Poke(a uint16, v byte) { c.wr(a, v) }
