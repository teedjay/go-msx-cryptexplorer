// Command tracediff compares the state recorded at two ticks, in the same
// or different traces: every differing RAM byte, and VRAM differences
// grouped into ranges. Used to check that a cheat start reproduces what a
// real arrival leaves behind.
//
//	go run ./cmd/tracediff -a real.trace:4374 -b cheat.trace:449
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go-msx-cryptexplorer/internal/reftrace"
)

func main() {
	a := flag.String("a", "", "TRACE:TICK")
	b := flag.String("b", "", "TRACE:TICK")
	flag.Parse()
	ra, rb := load(*a), load(*b)
	n := 0
	for i := range ra.RAM {
		if ra.RAM[i] != rb.RAM[i] {
			fmt.Printf("RAM $%04X: %02X %02X\n", reftrace.RAMBase+i, ra.RAM[i], rb.RAM[i])
			n++
		}
	}
	fmt.Printf("%d RAM bytes differ\n", n)
	lo, cnt := -1, 0
	flush := func(i int) {
		if lo >= 0 {
			fmt.Printf("VRAM $%04X-$%04X: %d bytes differ\n", lo, i-1, cnt)
			lo, cnt = -1, 0
		}
	}
	gap := 0
	for i := range ra.VRAM {
		if ra.VRAM[i] != rb.VRAM[i] {
			if lo < 0 {
				lo = i
			}
			cnt++
			gap = 0
		} else if lo >= 0 {
			gap++
			if gap > 16 {
				flush(i - gap + 1)
			}
		}
	}
	flush(len(ra.VRAM))
}

func load(spec string) *reftrace.Record {
	p, t, ok := strings.Cut(spec, ":")
	n, err := strconv.Atoi(t)
	if !ok || err != nil {
		fmt.Fprintln(os.Stderr, "want TRACE:TICK")
		os.Exit(2)
	}
	tr, err := reftrace.Open(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	r, err := tr.Read(n)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return r
}
