// Command extract rebuilds every asset in assets.Manifest from the
// user-supplied ROM into assets/raw. With -check it re-derives everything
// and diffs against the files already there, writing nothing.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"go-msx-cryptexplorer/assets"
	"go-msx-cryptexplorer/internal/rom"
	"go-msx-cryptexplorer/internal/streams"
)

func main() {
	romPath := flag.String("rom", rom.DefaultPath, "cartridge image")
	dir := flag.String("dir", "assets/raw", "output directory")
	check := flag.Bool("check", false, "re-derive and diff without writing")
	flag.Parse()

	r, err := rom.Load(*romPath)
	if err != nil {
		fail(err)
	}
	r.Warn(os.Stderr)
	built, err := build(r)
	if err != nil {
		fail(err)
	}
	bad := 0
	for _, e := range assets.Manifest {
		path := filepath.Join(*dir, e.Name+".bin")
		b := built[e.Name]
		if *check {
			old, err := os.ReadFile(path)
			switch {
			case err != nil:
				fmt.Printf("MISSING  %s\n", e.Name)
				bad++
			case !bytes.Equal(old, b):
				fmt.Printf("DIFFERS  %s (%d bytes on disk, %d derived)\n", e.Name, len(old), len(b))
				bad++
			}
			continue
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			fail(err)
		}
	}
	if *check {
		if bad > 0 {
			fmt.Printf("%d of %d assets do not match\n", bad, len(assets.Manifest))
			os.Exit(1)
		}
		fmt.Printf("all %d assets match the ROM\n", len(assets.Manifest))
		return
	}
	fmt.Printf("wrote %d assets to %s\n", len(assets.Manifest), *dir)
}

// build produces every entry, in manifest order, so Derived entries can
// use earlier ones.
func build(r *rom.ROM) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, e := range assets.Manifest {
		if _, dup := out[e.Name]; dup {
			return nil, fmt.Errorf("duplicate entry %s", e.Name)
		}
		switch e.Method {
		case assets.Raw:
			if !rom.Contains(e.Addr) || !rom.Contains(e.Addr+e.Len-1) || e.Len <= 0 {
				return nil, fmt.Errorf("%s: $%04X+%d outside the ROM", e.Name, e.Addr, e.Len)
			}
			b := make([]byte, e.Len)
			for i := range b {
				b[i] = r.Byte(e.Addr + i)
			}
			if e.Stream != "" {
				if err := checkStream(e, b); err != nil {
					return nil, err
				}
			}
			out[e.Name] = b
		case assets.Derived:
			src, ok := out[e.From]
			if !ok {
				return nil, fmt.Errorf("%s: source %s not built before it", e.Name, e.From)
			}
			b := e.Derive(src)
			if e.Len > 0 && len(b) != e.Len {
				return nil, fmt.Errorf("%s: derived %d bytes, manifest says %d", e.Name, len(b), e.Len)
			}
			out[e.Name] = b
		default:
			return nil, fmt.Errorf("%s: unknown method", e.Name)
		}
	}
	return out, nil
}

// checkStream decodes a stream entry from its own bytes (so it must not
// run past its declared length) and compares the segments with the
// manifest's.
func checkStream(e assets.Entry, b []byte) error {
	rd := func(p int) byte {
		if p-e.Addr < 0 || p-e.Addr >= len(b) {
			panic(fmt.Sprintf("%s: decode reads $%04X, past its %d bytes", e.Name, p, len(b)))
		}
		return b[p-e.Addr]
	}
	var segs []streams.Segment
	var n int
	var err error
	func() {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("%v", p)
			}
		}()
		switch e.Stream {
		case "rle":
			segs, n, err = streams.RLE(rd, e.Addr, true, false)
		case "rle-noaddr":
			segs, n, err = streams.RLE(rd, e.Addr, false, false)
		case "text":
			segs, n, err = streams.Text(rd, e.Addr, true)
		case "text-noaddr":
			segs, n, err = streams.Text(rd, e.Addr, false)
		case "rle-ram":
			var out []byte
			out, n, err = streams.RLERAM(rd, e.Addr)
			segs = []streams.Segment{{VRAM: -1, Data: out}}
		default:
			err = fmt.Errorf("unknown stream format %q", e.Stream)
		}
	}()
	if err != nil {
		return fmt.Errorf("%s: %w", e.Name, err)
	}
	if n != e.Len {
		return fmt.Errorf("%s: stream is %d bytes, manifest says %d", e.Name, n, e.Len)
	}
	if len(segs) != len(e.Segments) {
		return fmt.Errorf("%s: %d segments decoded, manifest declares %d", e.Name, len(segs), len(e.Segments))
	}
	for i, g := range segs {
		if g.VRAM != e.Segments[i].VRAM || len(g.Data) != e.Segments[i].Len {
			return fmt.Errorf("%s: segment %d is {$%04X, %d}, manifest says {$%04X, %d}",
				e.Name, i, g.VRAM, len(g.Data), e.Segments[i].VRAM, e.Segments[i].Len)
		}
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "extract:", err)
	os.Exit(1)
}
