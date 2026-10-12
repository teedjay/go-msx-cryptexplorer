// Command mapview replays a key schedule on the port and renders the
// whole pyramid (its three screens side by side) at a tick, from the map
// in RAM through the game's own cell-to-tile table. Output is
// ROM-derived: keep it out of the repo.
//
//	go run ./cmd/mapview -keys run.keys -tick 5000 -o pyramid.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"

	"go-msx-cryptexplorer/internal/engine"
	"go-msx-cryptexplorer/internal/keys"
	"go-msx-cryptexplorer/internal/msxrender"
)

func main() {
	keysPath := flag.String("keys", "", "key schedule")
	tick := flag.Int("tick", 0, "tick to render at")
	out := flag.String("o", "pyramid.png", "output PNG")
	cells := flag.String("cells", "", "also print map cells X0-X1,Y0-Y1 (decimal columns and rows)")
	flag.Parse()
	ev := keys.Schedule{}
	if *keysPath != "" {
		var err error
		if ev, err = keys.Read(*keysPath); err != nil {
			fail(err)
		}
	}
	m := engine.NewMachine()
	t := 0
	for t < *tick {
		t0 := t
		if m.FrameWith(func() { ev.Apply(m.Console, t0) }) {
			t++
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, 3*256, 192))
	for s := 0; s < 3; s++ {
		v := m.MapScreen(s)
		r := (&msxrender.VDP{VRAM: &v, Regs: m.VDP.Regs}).Render(nil)
		draw.Draw(img, image.Rect(s*256, 0, s*256+256, 192), r, image.Point{}, draw.Src)
	}
	f, err := os.Create(*out)
	if err != nil {
		fail(err)
	}
	png.Encode(f, img)
	f.Close()
	fmt.Printf("tick %d: pyramid %d, player screen %d X %d Y %d, jewels %d/%d\n", t, m.RAMAt(0xE054), m.RAMAt(0xE13A), m.RAMAt(0xE139), m.RAMAt(0xE137), m.RAMAt(0xE1F2), m.RAMAt(0xE1F3))
	if *cells != "" {
		var x0, x1, y0, y1 int
		fmt.Sscanf(*cells, "%d-%d,%d-%d", &x0, &x1, &y0, &y1)
		fmt.Printf("     ")
		for x := x0; x <= x1; x++ {
			fmt.Printf(" %3d", x)
		}
		fmt.Println()
		for y := y0; y <= y1; y++ {
			fmt.Printf("  %2d:", y)
			for x := x0; x <= x1; x++ {
				fmt.Printf("  %02X", m.MapCell(x, y))
			}
			fmt.Println()
		}
	}
	for n := 0; n < int(m.RAMAt(0xE1F3)); n++ {
		j := uint16(0xE1F5 + 9*n)
		fmt.Printf("  jewel %d: present %v Y %d X %d screen %d\n", n, m.RAMAt(j)&0x0F != 0, m.RAMAt(j+2), m.RAMAt(j+4), m.RAMAt(j+5))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mapview:", err)
	os.Exit(1)
}
