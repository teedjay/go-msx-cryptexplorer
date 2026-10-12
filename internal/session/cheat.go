package session

import "fmt"

// The pyramid-select cheat: an extension over the port, not part of it.
// StartAt starts a game through the game's own start path (SPACE on the
// title), then sets the pyramid before the first intro builds it
// (engine.StartPyramid). Nothing happens unless StartAt is called.

// StartAt asks for a game starting at pyramid n (1-15). It reports
// false during a game.
func (s *Session) StartAt(n int) bool {
	if s.M.InGame() || n < 1 || n > 15 {
		return false
	}
	s.startPyramid = n
	return true
}

// pyramidCheat drives a pending StartAt: it taps SPACE (the logo and the
// demo go to the title, the title starts a game), then writes the
// pyramid at the first intro tick.
func (s *Session) pyramidCheat(want *[11]byte) {
	if s.startPyramid == 0 || !s.M.WillTick() {
		return
	}
	if !s.M.InGame() {
		if s.Ticks%8 < 4 {
			k := Named["SPACE"]
			want[k.row] &^= k.mask
		}
		return
	}
	if s.M.StartPyramid(byte(s.startPyramid)) {
		if s.Record != nil {
			// Recorded as pokes, so the session still replays into the
			// original (scripts/refdump.tcl applies them at the tick).
			fmt.Fprintf(s.Record, "%d:P:%d:%d\n%d:P:%d:%d\n", s.Ticks, 0xE055, s.M.RAMAt(0xE055), s.Ticks, 0xE056, s.M.RAMAt(0xE056))
		}
		s.startPyramid = 0
	}
}
