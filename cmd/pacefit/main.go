// Command pacefit fits the tick pacing model: cycles = base + sum of
// coefficients times the engine's per-tick counters, frames = 1 +
// floor(cycles / 59659), to the frame counts of reference traces
// (CSV rows from refcompare -pacing: tick, frames, set-ups, writes,
// mirrored, ldir, fill, wrtvrm, rle, map tiles, probes, enemies). It minimises the number of
// mispredicted ticks by coordinate search and prints the coefficients.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
)

const frameCycles = 59659

type row struct {
	frames int
	x      [10]float64
}

func main() {
	restarts := flag.Int("restarts", 20, "random restarts of the search")
	flag.Parse()
	var rows []row
	for _, p := range flag.Args() {
		f, err := os.Open(p)
		if err != nil {
			fail(err)
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			v := strings.Split(sc.Text(), ",")
			if len(v) != 12 {
				continue
			}
			var r row
			r.frames, _ = strconv.Atoi(v[1])
			for i := 0; i < 10; i++ {
				r.x[i], _ = strconv.ParseFloat(v[2+i], 64)
			}
			rows = append(rows, r)
		}
		f.Close()
	}
	// p[0] base, p[1..7] per counter; start from rough Z80 estimates.
	p := []float64{4898, 44.6, 70.9, 391.4, 77.8, 4.1, 116.9, 7.7, 244.9, 150, 500}
	score := func(p []float64) (int, float64) {
		bad, sq := 0, 0.0
		for _, r := range rows {
			cyc := p[0]
			for i, x := range r.x {
				cyc += p[i+1] * x
			}
			pred := 1 + int(cyc/frameCycles)
			if pred != r.frames {
				bad++
				d := float64(pred - r.frames)
				sq += d * d
			}
		}
		return bad, sq
	}
	// Coordinate search from the start point and from random restarts
	// around the best so far; keep the best.
	search := func(p []float64) ([]float64, int, float64) {
		best, bsq := score(p)
		for step := 0.5; step > 0.001; step *= 0.7 {
			improved := true
			for improved {
				improved = false
				for i := range p {
					for _, m := range []float64{1 + step, 1 - step} {
						q := append([]float64(nil), p...)
						q[i] *= m
						if q[i] < 0 {
							continue
						}
						if b, sq := score(q); b < best || b == best && sq < bsq {
							p, best, bsq = q, b, sq
							improved = true
						}
					}
				}
			}
		}
		return p, best, bsq
	}
	p, best, bsq := search(p)
	rng := rand.New(rand.NewSource(1))
	for r := 0; r < *restarts; r++ {
		q := append([]float64(nil), p...)
		for i := range q {
			q[i] *= 0.5 + rng.Float64()
		}
		q, b, sq := search(q)
		if b < best || b == best && sq < bsq {
			p, best, bsq = q, b, sq
		}
	}
	fmt.Printf("%d of %d ticks mispredicted (squared frame error %.0f)\n", best, len(rows), bsq)
	names := []string{"base", "set-up", "write", "mirrored", "ldir", "fill", "wrtvrm", "rle", "maptile", "probe", "enemy"}
	for i, v := range p {
		fmt.Printf("  %-9s %8.1f\n", names[i], v)
	}
	// Show the mispredictions by frame count.
	miss := map[[2]int]int{}
	for _, r := range rows {
		cyc := p[0]
		for i, x := range r.x {
			cyc += p[i+1] * x
		}
		if pred := 1 + int(cyc/frameCycles); pred != r.frames {
			miss[[2]int{r.frames, pred}]++
		}
	}
	for k, n := range miss {
		fmt.Printf("  actual %d, predicted %d: %d ticks\n", k[0], k[1], n)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "pacefit:", err)
	os.Exit(1)
}
