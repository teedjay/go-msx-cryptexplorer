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
	near   float64
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
	dump := flag.Int("dump", 0, "after this many rounds, save the schedule to a top-level cell and stop (debugging)")
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
	schedule := func(end *node) []string {
		var chain []*node
		for n := end; n != root; n = n.parent {
			chain = append(chain, n)
		}
		sched := append([]string(nil), prefix...)
		t := root.tick
		h := root.mask
		for i := len(chain) - 1; i >= 0; i-- {
			n := chain[i]
			sched = append(sched, edges(t, h, n.mask)...)
			h = n.mask
			t += n.ticks
		}
		return sched
	}
	save := func(sched []string, final bool) {
		if *out == "" {
			return
		}
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		w := bufio.NewWriter(f)
		fmt.Fprintf(w, "# keysearch -phases %d -seed %d (final %v)\n", *phases, *seed, final)
		for _, l := range sched {
			fmt.Fprintln(w, l)
		}
		w.Flush()
		f.Close()
	}
	archive := map[string]*node{cell(m): root}
	top := root.level
	goal := func(n *node) bool { return int(n.m.RAMAt(0xE051))-startPhase >= *phases }
	var done *node
	stalled := 0
	for r := 0; r < *rounds && done == nil; r++ {
		stalled++
		// Candidates: the cells at the top level (or one below, a tenth
		// of the time).
		// Candidates: cells within a window below the top level, which
		// widens while no new top is found.
		window := 50 * (stalled / 300000)
		var cands []*node
		var w []float64
		total := 0.0
		for _, n := range archive {
			if n.level >= top-window {
				x := math.Exp(-n.near/96) / math.Sqrt(float64(n.chosen)+1)
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
		if *dump > 0 && r == *dump {
			save(schedule(cur), false)
			fmt.Fprintf(os.Stderr, "dumped a level-%d cell at tick %d\n", cur.level, cur.tick)
			return
		}
		if r%2000 == 0 {
			fmt.Fprintf(os.Stderr, "round %d: %d cells, %d candidates, top level %d\n", r, len(archive), len(cands), top)
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
				nd.near = near(mm)
				archive[key] = nd
				if nd.level > top {
					top = nd.level
					stalled = 0
					fmt.Fprintf(os.Stderr, "round %d tick %d level %d: phase %d pyramid %d jewels %d/%d (%d cells)\n",
						r, t, top, mm.RAMAt(0xE051), mm.RAMAt(0xE054), mm.RAMAt(0xE1F2), mm.RAMAt(0xE1F3), len(archive))
				}
				if goal(nd) {
					done = nd
					break
				}
				if int(mm.RAMAt(0xE051)) > int(root.m.RAMAt(0xE051)) && mm.RAMAt(0xE000) == 4 {
					// A new phase: checkpoint and start a fresh archive
					// here, so memory stays bounded.
					prefix = schedule(nd)
					save(prefix, false)
					root = &node{m: nd.m, mask: nd.mask, tick: nd.tick, level: nd.level}
					archive = map[string]*node{key: root}
					top = root.level
					stalled = 0
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
	sched := schedule(done)
	sched = append(sched, edges(done.tick, done.mask, 0)...)
	fmt.Fprintf(os.Stderr, "done: tick %d phase %d pyramid %d lives %d\n", done.tick, done.m.RAMAt(0xE051), done.m.RAMAt(0xE054), done.m.RAMAt(0xE050))
	save(sched, true)
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

// level is the progress measure: loops, phases, then jewels, then the
// door sequence.
func level(m *engine.Machine) int {
	l := int(m.RAMAt(0xE058))*100000 + int(m.RAMAt(0xE051))*1000
	if m.RAMAt(0xE051)&1 == 1 && m.RAMAt(0xE000) == 5 {
		l += int(m.RAMAt(0xE1F2)) * 50
		if m.RAMAt(0xE132) != 0 {
			l += 900
		}
	}
	return l
}

// near is how close the player is to the nearest jewel left (or, with
// all taken, to an open door), in pixels across the screens; 0 when
// there is nothing to go for.
func near(m *engine.Machine) float64 {
	if m.RAMAt(0xE051)&1 == 0 || m.RAMAt(0xE000) != 5 {
		return 0
	}
	py := float64(m.RAMAt(0xE137))
	px := float64(int(m.RAMAt(0xE13A))*256 + int(m.RAMAt(0xE139)))
	best := math.Inf(1)
	at := func(y, x float64) {
		best = min(best, math.Abs(x-px)+2*math.Abs(y-py))
	}
	if m.RAMAt(0xE1F2) != m.RAMAt(0xE1F3) {
		for n := 0; n < int(m.RAMAt(0xE1F3)); n++ {
			j := uint16(0xE1F5 + 9*n)
			if m.RAMAt(j)&0x0F != 0 {
				at(float64(m.RAMAt(j+2)), float64(int(m.RAMAt(j+5))*256+int(m.RAMAt(j+4))))
			}
		}
	} else {
		for n := 0; n < 4; n++ {
			d := uint16(0xE1C5 + 7*n)
			if m.RAMAt(d) != 0xFF && m.RAMAt(d+4)>>4 >= 2 && m.RAMAt(d+4)>>4 <= 4 {
				at(float64(m.RAMAt(d))-8, float64(int(m.RAMAt(d+3))*256+int(m.RAMAt(d+2))))
			}
		}
	}
	if math.IsInf(best, 1) {
		return 0
	}
	return best
}

// dug counts the map cells dug out (value 1).
func dug(m *engine.Machine) int {
	n := 0
	for a := uint16(0xE760); a < 0xF000; a++ {
		if m.RAMAt(a) == 1 {
			n++
		}
	}
	return n
}

// cell reduces the state to the archive key.
func cell(m *engine.Machine) string {
	t := byte(0)
	if m.RAMAt(0xE000) != 5 || m.RAMAt(0xE132) != 0 || m.RAMAt(0xE051)&1 == 0 {
		t = m.RAMAt(0xE004)>>4 ^ m.RAMAt(0xE130)>>3 // timers outside play
	}
	// What the player has changed: which jewels are taken, the sliding
	// walls' directions, and the cells dug out (value 1).
	jw := 0
	for n := 0; n < int(m.RAMAt(0xE1F3)) && n < 16; n++ {
		if m.RAMAt(uint16(0xE1F5+9*n))&0x0F == 0 {
			jw |= 1 << n
		}
	}
	walls := 0
	for n := 0; n < int(m.RAMAt(0xE31D)) && n < 8; n++ {
		walls = walls*16 + int(m.RAMAt(uint16(0xE31E+7*n))&1) + int(m.RAMAt(uint16(0xE31E+7*n+5)))
	}
	return fmt.Sprintf("%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%x/%x/%d", level(m), m.RAMAt(0xE000), m.RAMAt(0xE001), t, m.RAMAt(0xE13A), m.RAMAt(0xE139)>>4,
		m.RAMAt(0xE137)>>3, m.RAMAt(0xE144)>>4, m.RAMAt(0xE134), m.RAMAt(0xE130), jw, walls, dug(m))
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
