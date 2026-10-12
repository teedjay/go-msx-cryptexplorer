package session

import (
	"fmt"
	"os"
	"testing"
)

// TestStartAt starts a game at every pyramid through the cheat and plays
// it for a while with scripted keys: the pyramid must be the one asked
// for, and nothing may panic (every pyramid's data goes through the
// builder and the play code).
func TestStartAt(t *testing.T) {
	if _, err := os.Stat("../../assets/raw/vdpregs.bin"); err != nil {
		t.Skip("assets not extracted; run go run ./cmd/extract")
	}
	for n := 1; n <= 15; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			s := New()
			held := Released()
			for s.Frame < 400 {
				s.Step(held)
			}
			if !s.StartAt(n) {
				t.Fatal("StartAt refused outside a game")
			}
			sc, _ := ParsePress("100:RIGHT,300:-RIGHT,300:LEFT,500:-LEFT,520:UP,600:-UP,600:SPACE,604:-SPACE")
			start := s.Frame
			for s.Frame < start+1500 {
				sc.Apply(s.Frame-start-400, &held)
				s.Step(held)
			}
			if got := s.M.RAMAt(0xE054); got != byte(n) {
				t.Errorf("pyramid %d, want %d", got, n)
			}
		})
	}
}
