// Command sndpreview renders the port's audio to a WAV without a window,
// through the same session code the window uses (internal/session).
//
//	go run ./cmd/sndpreview -ticks 3000 -o attract.wav            # attract
//	go run ./cmd/sndpreview -keys play1.keys -ticks 6000 -o p.wav
//	go run ./cmd/sndpreview -sound 0x8B -frames 600 -o tune.wav     # one sound
//
// -keys takes the recorder's tick schedule (as refdump.tcl does), so the
// result can be compared with the original's audio recorded over the same
// schedule by scripts/refaudio.tcl (cmd/audiocmp).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go-msx-cryptexplorer/internal/session"
	"go-msx-cryptexplorer/internal/wav"
)

func main() {
	keysPath := flag.String("keys", "", "tick key schedule (tick:row:mask:down|up)")
	ticks := flag.Int("ticks", 3000, "game ticks to render")
	frames := flag.Int("frames", 600, "frames to render with -sound")
	sound := flag.String("sound", "", "render one sound id (hex or decimal) on an idle machine")
	out := flag.String("o", "out.wav", "output WAV")
	flag.Parse()

	type key struct {
		row  int
		mask byte
		down bool
	}
	sched := map[int][]key{}
	if *keysPath != "" {
		f, err := os.Open(*keysPath)
		if err != nil {
			fail(err)
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			l := strings.TrimSpace(sc.Text())
			if l == "" || l[0] == '#' {
				continue
			}
			p := strings.Split(l, ":")
			if len(p) != 4 {
				fail(fmt.Errorf("bad line %q", l))
			}
			t, _ := strconv.Atoi(p[0])
			r, _ := strconv.Atoi(p[1])
			m, _ := strconv.Atoi(p[2])
			sched[t] = append(sched[t], key{r, byte(m), p[3] == "down"})
		}
		f.Close()
	}
	s := session.New()
	var all []float32
	if *sound != "" {
		id, err := strconv.ParseUint(*sound, 0, 8)
		if err != nil {
			fail(err)
		}
		s.M.PlaySound(byte(id))
		for f := 0; f < *frames; f++ {
			all = append(all, s.SoundOnly()...)
		}
	} else {
		want := session.Released()
		applied := -1
		for s.Ticks < *ticks {
			if s.M.WillTick() && applied != s.Ticks {
				for _, k := range sched[s.Ticks] {
					if k.down {
						want[k.row] &^= k.mask
					} else {
						want[k.row] |= k.mask
					}
				}
				applied = s.Ticks
			}
			s.Step(want)
			all = append(all, s.Samples...)
		}
	}
	if err := wav.Write(*out, session.SampleRate, all); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s: %d ticks, %d frames\n", *out, s.Ticks, s.Frame)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "sndpreview:", err)
	os.Exit(1)
}
