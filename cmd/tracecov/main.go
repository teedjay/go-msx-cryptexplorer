// Command tracecov reports what a set of reference traces exercised: the
// original's own timeline (state, pyramid, lives, score) and which values
// the main state variables took, so a matching trace is not mistaken
// for coverage it does not have.
//
//	go run ./cmd/tracecov -timeline T1 [T2 ...]
//	go run ./cmd/tracecov T1 T2 ...
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"go-msx-cryptexplorer/internal/reftrace"
)

type variable struct {
	name string
	addr int
	hi   bool // high nibble only
}

var vars = []variable{
	{"state $E000", 0xE000, false},
	{"game state/sub $E000:$E001", -1, false},
	{"player $E134", 0xE134, false},
	{"item $E144", 0xE144, true},
	{"door seq $E130 (in $E132)", -2, false},
	{"pyramid $E054", 0xE054, false},
	{"loop $E058", 0xE058, false},
	{"enemy states", -3, false},
	{"enemy types", -4, false},
	{"dagger states", -5, false},
	{"door states", -6, true},
	{"traps", -7, false},
	{"walls active", -8, false},
}

func main() {
	timeline := flag.Bool("timeline", false, "print state changes with tick, pyramid, lives and score")
	flag.Parse()
	seen := map[string]map[int]int{}
	for _, v := range vars {
		seen[v.name] = map[int]int{}
	}
	for _, path := range flag.Args() {
		t, err := reftrace.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if *timeline {
			fmt.Println(path)
		}
		last := ""
		for i := 0; i < t.Len; i++ {
			r, err := t.Read(i)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			m := func(a int) int { return int(r.RAM[a-0xE000]) }
			game := m(0xE002)&0x40 != 0
			for _, v := range vars {
				s := seen[v.name]
				switch v.addr {
				case -1:
					if game {
						s[m(0xE000)<<8|m(0xE001)]++
					}
				case -2:
					if m(0xE132) != 0 {
						s[m(0xE130)]++
					}
				case -3:
					for n := 0; n < m(0xE164) && n < 8; n++ {
						s[m(0xE16A+22*n)]++
					}
				case -4:
					for n := 0; n < m(0xE164) && n < 8; n++ {
						s[m(0xE16A+22*n+0x14)]++
					}
				case -5:
					for n := 0; n < m(0xE263) && n < 8; n++ {
						s[m(0xE264+17*n)]++
					}
				case -6:
					for n := 0; n < 4; n++ {
						if m(0xE1C5+7*n) != 0xFF {
							s[m(0xE1C5+7*n+4)>>4]++
						}
					}
				case -7:
					for n := 0; n < m(0xE3FE) && n < 8; n++ {
						s[m(0xE3FF+5*n)]++
					}
				case -8:
					for n := 0; n < m(0xE31D) && n < 8; n++ {
						s[m(0xE31E+7*n)&1]++
					}
				default:
					val := m(v.addr)
					if v.hi {
						val >>= 4
					}
					s[val]++
				}
			}
			if *timeline {
				cur := fmt.Sprintf("game=%v state=%d pyr=%d lives=%d phase=%d", game, m(0xE000), m(0xE054), m(0xE050), m(0xE051))
				if cur != last {
					fmt.Printf("%7d  %s score=%02X%02X%02X jewels=%d/%d\n", i, cur, m(0xE04B), m(0xE04A), m(0xE049), m(0xE1F2), m(0xE1F3))
					last = cur
				}
			}
		}
		t.Close()
	}
	if *timeline {
		return
	}
	for _, v := range vars {
		var keys []int
		for k := range seen[v.name] {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		fmt.Printf("%-30s", v.name)
		for _, k := range keys {
			if v.addr == -1 {
				fmt.Printf(" %d:%d", k>>8, k&0xFF)
			} else {
				fmt.Printf(" %X", k)
			}
		}
		fmt.Println()
	}
}
