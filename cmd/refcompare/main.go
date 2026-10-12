// Command refcompare runs the port in lockstep with a reference trace of
// the original and stops at the first differing byte.
//
//	go run ./cmd/refcompare -trace /tmp/attract.trace [-out DIR]
//
// For every recorded tick it takes the frame interrupt, compares game RAM
// (minus the stack), all VRAM, R0-R7, PSG R0-R13 and the interrupt count
// with the record, then runs the tick with that tick's key events and its
// nested interrupts placed by VRAM write count. Input comes from the
// trace's own events file.
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"go-msx-cryptexplorer/internal/engine"
	"go-msx-cryptexplorer/internal/msxrender"
	"go-msx-cryptexplorer/internal/reftrace"
)

// Idle is the PC of INIT's JR $: an interrupt there starts a tick.
const idle = 0x409A

func main() {
	tracePath := flag.String("trace", "", "reference trace")
	out := flag.String("out", "", "write ref/port PNGs of the first differing tick here")
	debugTick := flag.Int("debugtick", -1, "print the nested-interrupt schedule and counts around this tick")
	pacing := flag.String("pacing", "", "write per-tick frames, VRAM address set-ups and writes (CSV) here")
	stackLo := flag.Int("stack", 0xE680, "lowest game-RAM address excluded as stack (to $E6FF; the stack starts at $E700)")
	flag.Parse()
	t, err := reftrace.Open(*tracePath)
	if err != nil {
		fail(err)
	}
	defer t.Close()
	if t.Len == 0 {
		fail(fmt.Errorf("empty trace"))
	}

	// Events by tick.
	nested := map[int][]engine.NestPoint{}
	keys := map[int][]reftrace.Event{}
	pokes := map[int][]reftrace.Event{}
	exitStart := map[int]bool{} // ticks started inside the previous one's exit path
	for _, e := range t.Events {
		switch e.Kind {
		case 'I':
			// Nested: taken while the busy flag is set. Older traces
			// lack the flag; for them, anything but the idle JR $.
			if e.Busy == 1 || e.Busy < 0 && e.Ret != idle {
				nested[e.Tick] = append(nested[e.Tick], engine.NestPoint{VW: e.VW, SS: e.SS})
			} else if e.Busy == 0 && e.Ret != idle {
				exitStart[e.Tick+1] = true
			}
		case 'K':
			keys[e.Tick] = append(keys[e.Tick], e)
		case 'P':
			pokes[e.Tick] = append(pokes[e.Tick], e)
		case 'W':
			switch e.Ret {
			case 0x4021:
			case 0x403C: // the handler's exit path ($4034)
				nested[e.Tick] = append(nested[e.Tick], engine.NestPoint{VW: e.VW, SS: e.SS, Exit: true})
			default:
				fail(fmt.Errorf("tick %d: $7B36 entered from $%04X", e.Tick, e.Ret))
			}
		}
	}

	c := engine.New()
	first, err := t.Read(0)
	if err != nil {
		fail(err)
	}
	// Power-on contents of the RAM INIT does not clear.
	copy(c.RAM[0x700:], first.RAM[0x700:])
	c.NestAt = nested[-2]
	tick := -1
	defer func() {
		if p := recover(); p != nil {
			fmt.Printf("tick %d: panic: %v\n%s", tick, p, engineStack())
			os.Exit(1)
		}
	}()
	c.Boot()
	if c.Interrupts != int(t.InitInterrupts) {
		fmt.Printf("INIT: %d interrupts, reference %d\n", c.Interrupts, t.InitInterrupts)
		os.Exit(1)
	}

	var pace *os.File
	if *pacing != "" {
		if pace, err = os.Create(*pacing); err != nil {
			fail(err)
		}
		defer pace.Close()
	}
	for tick = 0; tick < t.Len; tick++ {
		rec, err := t.Read(tick)
		if err != nil {
			fail(err)
		}
		_ = exitStart[tick] // same handler entry either way: no IY-like state is modelled
		c.Interrupt()
		for _, p := range pokes[tick] {
			c.Poke(p.Addr, p.A)
		}
		if msg := compare(c, rec, *stackLo); msg != "" {
			fmt.Printf("ticks 0-%d match; tick %d differs: %s\n", tick-1, tick, msg)
			if *out != "" {
				shots(c, rec, *out, tick)
			}
			n := 0
			for i := range rec.RAM {
				if a := engine.RAMBase + i; (a < *stackLo || a >= 0xE700) && c.RAM[i] != rec.RAM[i] {
					if n < 32 {
						fmt.Printf("  RAM $%04X: port $%02X, reference $%02X\n", a, c.RAM[i], rec.RAM[i])
					}
					n++
				}
			}
			fmt.Printf("  %d RAM bytes differ\n", n)
			os.Exit(1)
		}
		for _, k := range keys[tick] {
			if k.Down {
				c.Keys[k.Row] &^= k.Mask
			} else {
				c.Keys[k.Row] |= k.Mask
			}
		}
		c.NestAt = nested[tick]
		if tick == *debugTick {
			fmt.Printf("tick %d: NestAt %v, interrupts before %d\n", tick, c.NestAt, c.Interrupts)
		}
		c.Tick()
		if tick == *debugTick {
			fmt.Printf("tick %d: after: interrupts %d, VW %d, set-ups %d\n", tick, c.Interrupts, c.VW, c.SetAddrs)
		}
		if pace != nil && tick+1 < t.Len {
			next, err := t.Read(tick + 1)
			if err != nil {
				fail(err)
			}
			fmt.Fprintf(pace, "%d,%d,%d,%d,%d,%d,%d,%d,%d\n", tick, next.Interrupts-rec.Interrupts, c.SetAddrs, c.VW,
				c.Cost[0], c.Cost[1], c.Cost[2], c.Cost[3], c.Cost[4])
		}
		if c.Unported != "" {
			fmt.Printf("ticks 0-%d match; tick %d reached unported %s\n", tick, tick, c.Unported)
			os.Exit(2)
		}
	}
	fmt.Printf("all %d ticks match\n", t.Len)
}

func compare(c *engine.Console, r *reftrace.Record, stackLo int) string {
	if c.Interrupts != int(r.Interrupts) {
		return fmt.Sprintf("interrupts %d, reference %d", c.Interrupts, r.Interrupts)
	}
	for i := range r.RAM {
		a := engine.RAMBase + i
		if a >= stackLo && a < 0xE700 {
			continue
		}
		if c.RAM[i] != r.RAM[i] {
			return fmt.Sprintf("RAM $%04X: port $%02X, reference $%02X", a, c.RAM[i], r.RAM[i])
		}
	}
	for i := range r.Regs {
		if c.VDP.Regs[i] != r.Regs[i] {
			return fmt.Sprintf("VDP R%d: port $%02X, reference $%02X", i, c.VDP.Regs[i], r.Regs[i])
		}
	}
	for i := 0; i < 14; i++ {
		if c.PSG[i] != r.PSG[i] {
			return fmt.Sprintf("PSG R%d: port $%02X, reference $%02X", i, c.PSG[i], r.PSG[i])
		}
	}
	for i := range r.VRAM {
		if c.VDP.VRAM[i] != r.VRAM[i] {
			return fmt.Sprintf("VRAM $%04X: port $%02X, reference $%02X", i, c.VDP.VRAM[i], r.VRAM[i])
		}
	}
	return ""
}

func shots(c *engine.Console, r *reftrace.Record, dir string, tick int) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}
	for _, s := range []struct {
		name string
		v    *msxrender.VDP
	}{
		{"ref", &msxrender.VDP{VRAM: &r.VRAM, Regs: r.Regs}},
		{"port", &msxrender.VDP{VRAM: &c.VDP.VRAM, Regs: c.VDP.Regs}},
	} {
		p := filepath.Join(dir, fmt.Sprintf("tick%05d-%s.png", tick, s.name))
		f, err := os.Create(p)
		if err != nil {
			fail(err)
		}
		png.Encode(f, msxrender.Scale(s.v.Render(nil), 2))
		f.Close()
		fmt.Println("wrote", p)
	}
}

// engineStack keeps the stack frames inside the engine package.
func engineStack() string {
	var keep []string
	lines := strings.Split(string(debug.Stack()), "\n")
	for i := 0; i+1 < len(lines); i++ {
		if strings.Contains(lines[i], "internal/engine.") {
			keep = append(keep, "  "+strings.TrimSpace(lines[i]), "    "+strings.TrimSpace(lines[i+1]))
		}
	}
	return strings.Join(keep, "\n") + "\n"
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "refcompare:", err)
	os.Exit(1)
}
