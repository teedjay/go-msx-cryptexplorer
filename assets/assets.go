package assets

import (
	"embed"
	"fmt"
	"sort"
	"sync"
)

//go:embed all:raw
var raw embed.FS

// Bytes returns the extracted asset name. It panics if the asset is
// missing: run `go run ./cmd/extract` first.
func Bytes(name string) []byte {
	b, err := raw.ReadFile("raw/" + name + ".bin")
	if err != nil {
		panic(fmt.Sprintf("assets: %s missing; run `go run ./cmd/extract` (%v)", name, err))
	}
	return b
}

type span struct {
	lo, hi int // [lo, hi)
	name   string
}

var (
	spansOnce sync.Once
	spans     []span
)

// ROMByte returns the byte at a CPU address inside any Raw entry. It
// panics on any other address, so a read of something never extracted
// fails loudly with its address instead of returning a plausible zero.
// When it panics, ask the reference oracle who reads it before changing
// anything: so far that has always been genuine original behaviour.
func ROMByte(addr int) byte {
	spansOnce.Do(func() {
		for _, e := range Manifest {
			if e.Method == Raw {
				spans = append(spans, span{e.Addr, e.Addr + e.Len, e.Name})
			}
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i].lo < spans[j].lo })
	})
	for _, s := range spans {
		if addr >= s.lo && addr < s.hi {
			return Bytes(s.name)[addr-s.lo]
		}
	}
	panic(fmt.Sprintf("assets: ROM read at $%04X is not in any Raw manifest entry", addr))
}

// ROMWord returns the little-endian word at addr through ROMByte.
func ROMWord(addr int) uint16 {
	return uint16(ROMByte(addr)) | uint16(ROMByte(addr+1))<<8
}
