// Command machinecheck runs the paced Machine (the frontend's runner,
// which places owed interrupts after a tick instead of between the exact
// VRAM writes) with the trace's keys, and compares every tick start with
// the reference: it measures how far the frontend's timing model departs
// from the original, separately for RAM, VRAM, registers and timing.
package main

import (
	"flag"
	"fmt"
	"os"

	"go-msx-cryptexplorer/internal/engine"
	"go-msx-cryptexplorer/internal/reftrace"
)

func main() {
	tracePath := flag.String("trace", "", "reference trace")
	flag.Parse()
	t, err := reftrace.Open(*tracePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	keys := map[int][]reftrace.Event{}
	pokes := map[int][]reftrace.Event{}
	for _, e := range t.Events {
		switch e.Kind {
		case 'K':
			keys[e.Tick] = append(keys[e.Tick], e)
		case 'P':
			pokes[e.Tick] = append(pokes[e.Tick], e)
		}
	}
	m := engine.NewMachine()
	tick, frames := 0, 0
	var diffTicks, timingOff, firstRAM, firstVRAM int = 0, 0, -1, -1
	for tick < t.Len {
		// Before the frame that starts tick `tick`, apply its keys.
		for _, k := range keys[tick] {
			if k.Down {
				m.Keys[k.Row] &^= k.Mask
			} else {
				m.Keys[k.Row] |= k.Mask
			}
		}
		delete(keys, tick)
		// Peek: will this frame start a tick? Compare at the tick start.
		rec, err := t.Read(tick)
		if err != nil {
			break
		}
		startFrames := frames
		ticked := false
		for !ticked {
			frames++
			ticked = m.FrameWith(func() { // called at the tick start
				for _, p := range pokes[tick] {
					m.Poke(p.Addr, p.A)
				}
				ram, vram := 0, 0
				for i := range rec.RAM {
					if a := engine.RAMBase + i; a >= 0xE680 && a < 0xE700 {
						continue
					}
					if m.RAM[i] != rec.RAM[i] {
						ram++
					}
				}
				for i := range rec.VRAM {
					if m.VDP.VRAM[i] != rec.VRAM[i] {
						vram++
					}
				}
				if ram > 0 && firstRAM < 0 {
					firstRAM = tick
				}
				if vram > 0 && firstVRAM < 0 {
					firstVRAM = tick
				}
				if ram+vram > 0 {
					diffTicks++
				}
				if m.Interrupts != int(rec.Interrupts) {
					timingOff++
				}
			})
		}
		_ = startFrames
		tick++
	}
	fmt.Printf("%d ticks: %d differ in RAM/VRAM (first RAM diff at tick %d, first VRAM diff at tick %d); %d start on a different frame\n",
		tick, diffTicks, firstRAM, firstVRAM, timingOff)
}
