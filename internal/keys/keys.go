// Package keys reads and writes the key schedules shared by the
// reference recorder (scripts/refdump.tcl) and the port's tools: one
// line per change, "tick:row:mask:down|up" for the key matrix, or
// "tick:P:addr:value" (decimal) for a RAM poke (the pyramid-select
// cheat's writes), applied at the start of that tick.
package keys

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go-msx-cryptexplorer/internal/engine"
)

// Event is one scheduled change.
type Event struct {
	Row  int
	Mask byte
	Down bool
	Poke bool // a RAM write instead of a key
	Addr uint16
	Val  byte
}

// Schedule maps a tick to the changes applied at its start.
type Schedule map[int][]Event

// Read parses a schedule file.
func Read(path string) (Schedule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := Schedule{}
	sc := bufio.NewScanner(f)
	for ln := 1; sc.Scan(); ln++ {
		l := strings.TrimSpace(sc.Text())
		if l == "" || l[0] == '#' {
			continue
		}
		p := strings.Split(l, ":")
		if len(p) != 4 {
			return nil, fmt.Errorf("%s:%d: bad line %q", path, ln, l)
		}
		t, err1 := strconv.Atoi(p[0])
		a, err2 := strconv.Atoi(p[2])
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("%s:%d: bad line %q", path, ln, l)
		}
		if p[1] == "P" {
			v, err := strconv.Atoi(p[3])
			if err != nil {
				return nil, fmt.Errorf("%s:%d: bad poke %q", path, ln, l)
			}
			s[t] = append(s[t], Event{Poke: true, Addr: uint16(a), Val: byte(v)})
			continue
		}
		r, err := strconv.Atoi(p[1])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: bad row %q", path, ln, l)
		}
		s[t] = append(s[t], Event{Row: r, Mask: byte(a), Down: p[3] == "down"})
	}
	return s, sc.Err()
}

// Last is the last tick with a change.
func (s Schedule) Last() int {
	last := 0
	for t := range s {
		last = max(last, t)
	}
	return last
}

// Apply applies tick t's changes to the console: keys to its matrix,
// pokes to RAM.
func (s Schedule) Apply(c *engine.Console, t int) {
	for _, e := range s[t] {
		switch {
		case e.Poke:
			c.Poke(e.Addr, e.Val)
		case e.Down:
			c.Keys[e.Row] &^= e.Mask
		default:
			c.Keys[e.Row] |= e.Mask
		}
	}
}

// Line formats an event at tick t.
func (e Event) Line(t int) string {
	if e.Poke {
		return fmt.Sprintf("%d:P:%d:%d", t, e.Addr, e.Val)
	}
	d := "up"
	if e.Down {
		d = "down"
	}
	return fmt.Sprintf("%d:%d:%d:%s", t, e.Row, e.Mask, d)
}
