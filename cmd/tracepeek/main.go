// Command tracepeek prints game RAM bytes from a reference trace, one
// line per tick, for chasing a divergence.
//
//	go run ./cmd/tracepeek -trace T -ticks 820-835 -addr E134-E13F,E009
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
	tracePath := flag.String("trace", "", "reference trace")
	ticks := flag.String("ticks", "0", "A-B")
	addrs := flag.String("addr", "E000-E00F", "comma-separated addresses or ranges (hex)")
	flag.Parse()
	t, err := reftrace.Open(*tracePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	lo, hi := span(*ticks)
	var list []int
	for _, f := range strings.Split(*addrs, ",") {
		a, b := span(f)
		if b == a && !strings.Contains(f, "-") {
			b = a
		}
		for x := a; x <= b; x++ {
			list = append(list, x)
		}
	}
	for i := lo; i <= hi && i < t.Len; i++ {
		r, err := t.Read(i)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%6d i%-6d", i, r.Interrupts)
		for _, a := range list {
			fmt.Printf(" %02X", r.RAM[a-reftrace.RAMBase])
		}
		fmt.Println()
	}
}

func span(s string) (int, int) {
	a, b, ok := strings.Cut(s, "-")
	x, _ := strconv.ParseInt(a, 16, 32)
	if !ok {
		if strings.ContainsAny(s, "ABCDEFabcdef") || len(s) == 4 {
			return int(x), int(x)
		}
		v, _ := strconv.Atoi(s)
		return v, v
	}
	y, _ := strconv.ParseInt(b, 16, 32)
	if !strings.ContainsAny(s, "ABCDEFabcdefE") && len(a) < 4 {
		v1, _ := strconv.Atoi(a)
		v2, _ := strconv.Atoi(b)
		return v1, v2
	}
	return int(x), int(y)
}
