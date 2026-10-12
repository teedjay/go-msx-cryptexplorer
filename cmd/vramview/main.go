// Command vramview renders VRAM, from a reference trace or a raw 16KB dump,
// through internal/msxrender. Output is ROM-derived: keep it out of the repo.
//
//	go run ./cmd/vramview -trace T -ticks 0-3000/100 -out DIR
//	go run ./cmd/vramview -trace T -ticks 120 -sheet patterns -out DIR
//	go run ./cmd/vramview -vram dump.bin -regs 02E20E7F077603E4 -out DIR
//
// -sheet: screen (default), patterns (the three Graphic 2 pattern thirds
// with their colours), sprites (the 64 16x16 sprite patterns in white).
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go-msx-cryptexplorer/internal/msxrender"
	"go-msx-cryptexplorer/internal/reftrace"
)

func main() {
	tracePath := flag.String("trace", "", "reference trace")
	ticks := flag.String("ticks", "0", "ticks: N, A-B, or A-B/STEP")
	vramPath := flag.String("vram", "", "raw 16KB VRAM dump instead of a trace")
	regsHex := flag.String("regs", "02E20E7F077603E4", "R0-R7 as hex, with -vram")
	sheet := flag.String("sheet", "screen", "screen, patterns or sprites")
	scale := flag.Int("scale", 2, "integer scale")
	out := flag.String("out", ".", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}

	if *vramPath != "" {
		b, err := os.ReadFile(*vramPath)
		if err != nil || len(b) != 0x4000 {
			fail(fmt.Errorf("%s: want 16384 bytes (%v)", *vramPath, err))
		}
		regs, err := hex.DecodeString(*regsHex)
		if err != nil || len(regs) != 8 {
			fail(fmt.Errorf("-regs: want 16 hex digits"))
		}
		v := &msxrender.VDP{VRAM: (*[0x4000]byte)(b)}
		copy(v.Regs[:], regs)
		write(filepath.Join(*out, strings.TrimSuffix(filepath.Base(*vramPath), filepath.Ext(*vramPath))+"-"+*sheet+".png"), draw(v, *sheet), *scale)
		return
	}
	if *tracePath == "" {
		fail(fmt.Errorf("need -trace or -vram"))
	}
	t, err := reftrace.Open(*tracePath)
	if err != nil {
		fail(err)
	}
	defer t.Close()
	list, err := parseTicks(*ticks, t.Len)
	if err != nil {
		fail(err)
	}
	for _, i := range list {
		r, err := t.Read(i)
		if err != nil {
			fail(err)
		}
		v := &msxrender.VDP{VRAM: &r.VRAM, Regs: r.Regs}
		write(filepath.Join(*out, fmt.Sprintf("tick%05d-%s.png", i, *sheet)), draw(v, *sheet), *scale)
	}
	fmt.Printf("%d image(s) in %s\n", len(list), *out)
}

func draw(v *msxrender.VDP, sheet string) *image.RGBA {
	switch sheet {
	case "screen":
		return v.Render(nil)
	case "patterns":
		return patterns(v)
	case "sprites":
		return sprites(v)
	}
	fail(fmt.Errorf("unknown sheet %q", sheet))
	return nil
}

// patterns draws the three 256-character Graphic 2 pattern thirds, 32
// tiles per row, with their colour bytes (unmasked, at the R3/R4 bases).
func patterns(v *msxrender.VDP) *image.RGBA {
	colBase := int(v.Regs[3]&0x80) << 6
	patBase := int(v.Regs[4]&0x04) << 11
	return msxrender.TileSheet(v.VRAM[patBase:patBase+0x1800], v.VRAM[colBase:colBase+0x1800], 32, v.Backdrop())
}

// sprites draws the 64 16x16 sprite patterns.
func sprites(v *msxrender.VDP) *image.RGBA {
	spt := int(v.Regs[6]&0x07) << 11
	return msxrender.SpriteSheet(v.VRAM[spt:spt+0x800], 16)
}

func write(path string, img *image.RGBA, scale int) {
	img = msxrender.Scale(img, scale)
	f, err := os.Create(path)
	if err != nil {
		fail(err)
	}
	if err := png.Encode(f, img); err != nil {
		fail(err)
	}
	f.Close()
}

func parseTicks(s string, n int) ([]int, error) {
	step := 1
	if a, b, ok := strings.Cut(s, "/"); ok {
		var err error
		if step, err = strconv.Atoi(b); err != nil || step < 1 {
			return nil, fmt.Errorf("bad step in %q", s)
		}
		s = a
	}
	lo, hi := s, s
	if a, b, ok := strings.Cut(s, "-"); ok {
		lo, hi = a, b
	}
	l, err1 := strconv.Atoi(lo)
	h, err2 := strconv.Atoi(hi)
	if err1 != nil || err2 != nil || l < 0 || h < l {
		return nil, fmt.Errorf("bad ticks %q", s)
	}
	if h >= n {
		h = n - 1
	}
	var out []int
	for i := l; i <= h; i += step {
		out = append(out, i)
	}
	return out, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "vramview:", err)
	os.Exit(1)
}
