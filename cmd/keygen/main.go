// Command keygen writes a scripted-input schedule for scripts/refdump.tcl
// (and the port): lines "tick:row:mask:down|up" on the key matrix.
//
//	go run ./cmd/keygen -start 60 -ticks 6000 -seed 1 > play.keys
//
// It taps SPACE at -start (the logo goes to the title) and again 60
// ticks later (the title starts a game), then from 120 ticks after
// -start holds a random combination of cursor keys and SPACE for each
// chunk of 4 to -chunk ticks. Row 8: bit 0 SPACE, 4 LEFT, 5 UP, 6 DOWN,
// 7 RIGHT.
package main

import (
	"flag"
	"fmt"
	"math/rand"
)

const (
	row   = 8
	space = 0x01
	left  = 0x10
	up    = 0x20
	down  = 0x40
	right = 0x80
)

func main() {
	start := flag.Int("start", 60, "tick to tap SPACE on the logo")
	ticks := flag.Int("ticks", 6000, "last tick of random play")
	chunk := flag.Int("chunk", 40, "longest random input chunk")
	seed := flag.Int64("seed", 1, "random seed")
	jump := flag.Float64("jump", 0.15, "probability of SPACE in a chunk")
	vert := flag.Float64("vert", 0.3, "probability of UP or DOWN in a chunk")
	flag.Parse()
	rng := rand.New(rand.NewSource(*seed))

	fmt.Printf("# keygen -start %d -ticks %d -chunk %d -seed %d -jump %g -vert %g\n", *start, *ticks, *chunk, *seed, *jump, *vert)
	tap := func(t int) { fmt.Printf("%d:%d:%d:down\n%d:%d:%d:up\n", t, row, space, t+4, row, space) }
	tap(*start)
	tap(*start + 60)
	held := byte(0)
	set := func(t int, want byte) {
		if r := held &^ want; r != 0 {
			fmt.Printf("%d:%d:%d:up\n", t, row, r)
		}
		if p := want &^ held; p != 0 {
			fmt.Printf("%d:%d:%d:down\n", t, row, p)
		}
		held = want
	}
	for t := *start + 120; t < *ticks; {
		var m byte
		switch rng.Intn(5) {
		case 0, 1:
			m |= left
		case 2, 3:
			m |= right
		}
		if rng.Float64() < *vert {
			if rng.Intn(2) == 0 {
				m |= up
			} else {
				m |= down
			}
		}
		if rng.Float64() < *jump {
			m |= space
		}
		set(t, m)
		t += 4 + rng.Intn(*chunk)
	}
	set(*ticks, 0)
}
