// Command keysearch searches, on the port, for an input schedule that
// gets through pyramids without losing a life, and writes it in the
// reference recorder's format. The port is only the search engine:
// record the schedule on the original (scripts/refdump.sh) and let
// cmd/refcompare judge. The game reads no entropy (no LD A,R), so a
// schedule found here replays exactly.
//
//	go run ./cmd/keysearch -phases 4 -o /tmp/run.keys
//	go run ./cmd/keysearch -from /tmp/run.keys -phases 8 -o more.keys
//
// It is an archive search (in the manner of Go-Explore): the game state
// is reduced to a cell (progress, screen, a coarse player position,
// item held), and every cell reached keeps the shortest way found to
// it. Each round picks a cell from the highest progress level reached,
// favouring cells chosen least often, restores it and plays a few random
// chunks of input from there, adding any new cell. Progress is phases
// ($E051 counts pyramids and map screens), then jewels collected, then
// the door sequence. A lost life ends a branch.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"go-msx-cryptexplorer/internal/engine"
)

const (
	row   = 8
	space = 0x01
	left  = 0x10
	up    = 0x20
	down  = 0x40
	right = 0x80
)

var masks = []byte{0, left, right, left, right, left | up, right | up, left | down, right | down,
	up, down, space, left | space, right | space}

// node is one archived cell: the machine there, and how it was reached
// (the parent and the input held for the ticks since).
type node struct {
	m      *engine.Machine
	parent *node
	mask   byte
	ticks  int
	tick   int // tick reached
	chosen int
	level  int
}

func main() {
	from := flag.String("from", "", "replay this schedule first")
	phases := flag.Int("phases", 2, "stop once $E051 has advanced this many times")
	out := flag.String("o", "", "output schedule")
	seed := flag.Uint64("seed", 1, "random seed")
	rounds := flag.Int("rounds", 2000000, "give up after this many rounds")
	flag.Parse()
	rng := rand.New(rand.NewPCG(*seed, 7))

	m := engine.NewMachine()
	var prefix []string
	tick := 0
	held := byte(0)
	if *from != "" {
		ev := readSchedule(*from)
		last := 0
		for t := range ev {
			last = max(last, t)
		}
		for tick <= last {
			if m.WillTick() {
				for _, e := range ev[tick] {
					if e.down {
						m.Keys[e.row] &^= e.mask
					} else {
						m.Keys[e.row] |= e.mask
					}
					prefix = append(prefix, fmt.Sprintf("%d:%d:%d:%s", tick, e.row, e.mask, map[bool]string{true: "down", false: "up"}[e.down]))
				}
			}
			if m.Frame() {
				tick++
			}
		}
		held = ^m.Keys[row]
	} else {
		for _, st := range []struct {
			mask byte
			n    int
		}{{0, 60}, {space, 4}, {0, 56}, {space, 4}, {0, 60}} {
			prefix = append(prefix, edges(tick, held, st.mask)...)
			held = st.mask
			tick = play(m, st.mask, st.n, tick)
		}
	}
	startPhase := int(m.RAMAt(0xE051))
	root := &node{m: m, mask: held, tick: tick, level: level(m)}
	archive := map[string]*node{cell(m): root}
	top := root.level
	goal := func(n *node) bool { return int(n.m.RAMAt(0xE051))-startPhase >= *phases }
	var done *node
	for r := 0; r < *rounds && done == nil; r++ {
		// Candidates: the cells at the top level (or one below, a tenth
		// of the time).
		lv := top
		if rng.IntN(10) == 0 {
			lv--
		}
		var cands []*node
		var w []float64
		total := 0.0
		for _, n := range archive {
			if n.level == lv || n.level == top {
				x := 1 / math.Sqrt(float64(n.chosen)+1)
				cands = append(cands, n)
				w = append(w, x)
				total += x
			}
		}
		pick := rng.Float64() * total
		var cur *node
		for i, x := range w {
			pick -= x
			if pick <= 0 {
				cur = cands[i]
				break
			}
		}
		if cur == nil {
			cur = cands[len(cands)-1]
		}
		cur.chosen++
		if r%2000 == 0 {
			fmt.Fprintf(os.Stderr, "round %d: %d cells, %d at the top level %d\n", r, len(archive), len(cands), top)
		}
		mm := cur.m.Clone()
		parent := cur
		t := cur.tick
		for k := 0; k < 6; k++ {
			mask := masks[rng.IntN(len(masks))]
			n := 4 + rng.IntN(24)
			lost := false
			for i := 0; i < n; i++ {
				t = play(mm, mask, 1, t)
				if died(mm) {
					lost = true
					break
				}
			}
			if lost {
				break
			}
			key := cell(mm)
			nd := &node{parent: parent, mask: mask, ticks: n, tick: t, level: level(mm)}
			if old, ok := archive[key]; !ok || nd.tick < old.tick {
				nd.m = mm.Clone()
				archive[key] = nd
				if nd.level > top {
					top = nd.level
					fmt.Fprintf(os.Stderr, "round %d tick %d level %d: phase %d pyramid %d jewels %d/%d (%d cells)\n",
						r, t, top, mm.RAMAt(0xE051), mm.RAMAt(0xE054), mm.RAMAt(0xE1F2), mm.RAMAt(0xE1F3), len(archive))
				}
				if goal(nd) {
					done = nd
					break
				}
			}
			parent = nd
		}
	}
	if done == nil {
		fmt.Fprintln(os.Stderr, "no schedule reached the goal")
		os.Exit(1)
	}
	// Rebuild the schedule from the chain of segments.
	var chain []*node
	for n := done; n != root; n = n.parent {
		chain = append(chain, n)
	}
	sched := prefix
	t := root.tick
	h := root.mask
	for i := len(chain) - 1; i >= 0; i-- {
		n := chain[i]
		sched = append(sched, edges(t, h, n.mask)...)
		h = n.mask
		t += n.ticks
	}
	sched = append(sched, edges(t, h, 0)...)
	fmt.Fprintf(os.Stderr, "done: tick %d phase %d pyramid %d lives %d\n", done.tick, done.m.RAMAt(0xE051), done.m.RAMAt(0xE054), done.m.RAMAt(0xE050))
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		w := bufio.NewWriter(f)
		fmt.Fprintf(w, "# keysearch -phases %d -seed %d\n", *phases, *seed)
		for _, l := range sched {
			fmt.Fprintln(w, l)
		}
		w.Flush()
		f.Close()
	}
}

// play runs n ticks with mask held and returns the tick count.
func play(m *engine.Machine, mask byte, n int, t int) int {
	for i := 0; i < n; {
		if m.WillTick() {
			m.Keys[row] = ^mask
		}
		if m.Frame() {
			t++
			i++
		}
	}
	return t
}

// edges is the schedule lines that change the held keys from h to mask at
// tick t.
func edges(t int, h, mask byte) []string {
	var out []string
	if r := h &^ mask; r != 0 {
		out = append(out, fmt.Sprintf("%d:%d:%d:up", t, row, r))
	}
	if p := mask &^ h; p != 0 {
		out = append(out, fmt.Sprintf("%d:%d:%d:down", t, row, p))
	}
	return out
}

// died reports a lost life or the game over.
func died(m *engine.Machine) bool {
	s := m.RAMAt(0xE000)
	return s == 6 || s == 7 || s == 5 && m.RAMAt(0xE053) == 0 && m.RAMAt(0xE051)&1 == 1 && m.RAMAt(0xE132) == 0
}

// level is the progress measure.
func level(m *engine.Machine) int {
	l := int(m.RAMAt(0xE058))*1000 + int(m.RAMAt(0xE051))*20
	if m.RAMAt(0xE051)&1 == 1 {
		l += int(m.RAMAt(0xE1F2))
		if m.RAMAt(0xE132) != 0 {
			l += 15
		}
	}
	return l
}

// cell reduces the state to the archive key.
func cell(m *engine.Machine) string {
	t := byte(0)
	if m.RAMAt(0xE000) != 5 || m.RAMAt(0xE132) != 0 || m.RAMAt(0xE051)&1 == 0 {
		t = m.RAMAt(0xE004)>>4 ^ m.RAMAt(0xE130)>>3 // timers outside play
	}
	return fmt.Sprintf("%d/%d/%d/%d/%d/%d/%d/%d/%d/%d", level(m), m.RAMAt(0xE000), m.RAMAt(0xE001), t, m.RAMAt(0xE13A), m.RAMAt(0xE139)>>4,
		m.RAMAt(0xE137)>>3, m.RAMAt(0xE144)>>4, m.RAMAt(0xE134), m.RAMAt(0xE130))
}

type keyEvent struct {
	row  int
	mask byte
	down bool
}

func readSchedule(path string) map[int][]keyEvent {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	ev := map[int][]keyEvent{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || l[0] == '#' {
			continue
		}
		p := strings.Split(l, ":")
		t, _ := strconv.Atoi(p[0])
		r, _ := strconv.Atoi(p[1])
		mk, _ := strconv.Atoi(p[2])
		ev[t] = append(ev[t], keyEvent{r, byte(mk), p[3] == "down"})
	}
	return ev
}
