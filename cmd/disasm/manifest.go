package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"go-msx-cryptexplorer/internal/disasm"
	"go-msx-cryptexplorer/internal/rom"
	"go-msx-cryptexplorer/internal/streams"
)

// writeManifest prints Go source for assets.Manifest entries covering
// every claimed stream, every stream-table word run and every referenced
// gap piece, as a starting point to curate by hand.
func writeManifest(w io.Writer, a *disasm.Analysis, r *rom.ROM) {
	type ent struct {
		addr, n int
		text    string
	}
	var es []ent
	rd := func(p int) byte {
		if rom.Contains(p) {
			return r.Byte(p)
		}
		return 0xFF
	}
	inStream := map[int]bool{}
	for _, s := range a.Streams {
		for i := 0; i < s.Len; i++ {
			inStream[int(s.Addr)+i] = true
		}
		var segs []streams.Segment
		switch s.Format {
		case "rle":
			segs, _, _ = streams.RLE(rd, int(s.Addr), true, false)
		case "rle-noaddr":
			segs, _, _ = streams.RLE(rd, int(s.Addr), false, false)
		case "text":
			segs, _, _ = streams.Text(rd, int(s.Addr), true)
		case "text-noaddr":
			segs, _, _ = streams.Text(rd, int(s.Addr), false)
		case "rle-ram":
			out, _, _ := streams.RLERAM(rd, int(s.Addr))
			segs = []streams.Segment{{VRAM: -1, Data: out}}
		}
		var sg []string
		for _, g := range segs {
			sg = append(sg, fmt.Sprintf("{%s, %d}", vram(g.VRAM), len(g.Data)))
		}
		name := a.Label(s.Addr)
		if name == "" || strings.HasPrefix(name, "D") {
			name = fmt.Sprintf("s%04X", s.Addr)
		}
		shared := ""
		if s.Shared {
			shared = " Shares bytes with another stream."
		}
		es = append(es, ent{int(s.Addr), s.Len, fmt.Sprintf(
			"\t{Name: %q, Method: Raw, Addr: 0x%04X, Len: %d, Stream: %q,\n\t\tSegments: []Segment{%s},\n\t\tNote: %q},\n",
			strings.ToLower(name), s.Addr, s.Len, s.Format, strings.Join(sg, ", "),
			fmt.Sprintf("%s stream used from %s.%s", s.Format, hexList(s.From), shared))})
	}
	// Runs of data bytes outside streams (stream-table words), excluding
	// the cartridge header.
	for p := rom.Base + 16; p < rom.End; {
		if a.Kind[p-rom.Base] != disasm.Data || inStream[p] {
			p++
			continue
		}
		q := p
		for q < rom.End && a.Kind[q-rom.Base] == disasm.Data && !inStream[q] {
			q++
		}
		name := a.Label(uint16(p))
		if name == "" || strings.HasPrefix(name, "D") {
			name = fmt.Sprintf("t%04X", p)
		}
		es = append(es, ent{p, q - p, fmt.Sprintf("\t{Name: %q, Method: Raw, Addr: 0x%04X, Len: %d,\n\t\tNote: %q},\n",
			strings.ToLower(name), p, q-p, "pointer table (stream pointers)")})
		p = q
	}
	// Referenced gap pieces.
	for _, g := range a.Gaps() {
		refs := a.RefsInto(g.Start, g.End)
		cuts := []int{}
		by := map[int][]uint16{}
		for _, x := range refs {
			if len(cuts) == 0 || int(x.To) != cuts[len(cuts)-1] {
				cuts = append(cuts, int(x.To))
			}
			by[int(x.To)] = append(by[int(x.To)], x.From)
		}
		cuts = append(cuts, g.End)
		for i := 0; i+1 < len(cuts); i++ {
			lo, hi := cuts[i], cuts[i+1]
			if lo == hi {
				continue
			}
			es = append(es, ent{lo, hi - lo, fmt.Sprintf("\t{Name: %q, Method: Raw, Addr: 0x%04X, Len: %d,\n\t\tNote: %q},\n",
				fmt.Sprintf("d%04x", lo), lo, hi-lo, fmt.Sprintf("Referenced from %s. Extent: to the next reference or claimed byte (Low).", hexList(by[lo])))})
		}
	}
	sort.Slice(es, func(i, j int) bool { return es[i].addr < es[j].addr })
	for _, e := range es {
		fmt.Fprint(w, e.text)
	}
}

func vram(v int) string {
	if v < 0 {
		return "-1"
	}
	return fmt.Sprintf("0x%04X", v)
}

func hexList(xs []uint16) string {
	var s []string
	for _, x := range xs {
		if x == 0 {
			continue
		}
		s = append(s, fmt.Sprintf("$%04X", x))
	}
	if len(s) == 0 {
		return "entries"
	}
	return strings.Join(s, ", ")
}
