// Command gfxpreview renders a ROM byte range (or an extracted asset) as
// 8x8 tiles or 16x16 sprites through internal/msxrender, to judge whether
// a block is graphics and whether it is compressed. Output is ROM-derived:
// keep it out of the repo.
//
//	go run ./cmd/gfxpreview -range 7000-7800 -o /tmp/x.png
//	go run ./cmd/gfxpreview -range 7000-7800 -colors 7800 -o /tmp/x.png
//	go run ./cmd/gfxpreview -asset font -layout sprite16 -o /tmp/x.png
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"strings"

	"go-msx-cryptexplorer/assets"
	"go-msx-cryptexplorer/internal/disasm"
	"go-msx-cryptexplorer/internal/msxrender"
	"go-msx-cryptexplorer/internal/rom"
)

func main() {
	romPath := flag.String("rom", rom.DefaultPath, "cartridge image")
	rng := flag.String("range", "", "ROM range A-B (hex, B exclusive)")
	asset := flag.String("asset", "", "extracted asset name instead of -range")
	colors := flag.String("colors", "", "ROM address of matching Graphic 2 colour bytes")
	layout := flag.String("layout", "tile8", "tile8 or sprite16")
	cols := flag.Int("cols", 32, "tiles (or sprites) per row")
	scale := flag.Int("scale", 3, "integer scale")
	out := flag.String("o", "gfx.png", "output PNG")
	flag.Parse()

	var data, col []byte
	switch {
	case *asset != "":
		data = assets.Bytes(*asset)
	case *rng != "":
		r, err := rom.Load(*romPath)
		if err != nil {
			fail(err)
		}
		r.Warn(os.Stderr)
		a, b, ok := strings.Cut(*rng, "-")
		lo, err1 := disasm.ParseAddr(a)
		hi, err2 := disasm.ParseAddr(b)
		if !ok || err1 != nil || err2 != nil || hi <= lo || !rom.Contains(int(lo)) || int(hi) > rom.End {
			fail(fmt.Errorf("bad -range %q", *rng))
		}
		data = r.Data[int(lo)-rom.Base : int(hi)-rom.Base]
		if *colors != "" {
			c, err := disasm.ParseAddr(*colors)
			if err != nil || !rom.Contains(int(c)) || int(c)+len(data) > rom.End {
				fail(fmt.Errorf("bad -colors %q", *colors))
			}
			col = r.Data[int(c)-rom.Base : int(c)-rom.Base+len(data)]
		}
	default:
		fail(fmt.Errorf("need -range or -asset"))
	}

	img := msxrender.TileSheet(data, col, *cols, 1)
	if *layout == "sprite16" {
		img = msxrender.SpriteSheet(data, *cols)
	}
	f, err := os.Create(*out)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := png.Encode(f, msxrender.Scale(img, *scale)); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gfxpreview:", err)
	os.Exit(1)
}
