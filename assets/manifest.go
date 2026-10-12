// Package assets holds the manifest of everything the port reads from the
// original ROM, and serves the extracted bytes. The bytes themselves are
// rebuilt locally by cmd/extract into assets/raw (git-ignored); nothing
// ROM-derived is committed.
package assets

// Method says how an entry is produced from the ROM.
type Method int

const (
	// Raw copies Len bytes from Addr. Raw entries also back ROMByte, so a
	// routine walking a pointer table can be ported with its original
	// address arithmetic.
	Raw Method = iota
	// Derived computes an asset from another entry (load-time mirrors,
	// derived colours) with Derive.
	Derived
)

// Entry is one manifest line.
type Entry struct {
	Name   string
	Method Method
	Addr   int // CPU address ($4000-$7FFF), for Raw
	Len    int
	From   string              // source entry, for Derived
	Derive func([]byte) []byte // for Derived
	// Stream, for a Raw entry that is one of the ROM's streams, names its
	// format (see internal/streams) and Segments the decode it must
	// produce: the extractor fails if it does not.
	Stream   string
	Segments []Segment
	// Note says what the data is, what references it, and how sure we
	// are (High/Medium/Low).
	Note string
}

// Segment is one run of a decoded stream: its VRAM address (-1 when the
// stream has none) and decoded length.
type Segment struct{ VRAM, Len int }

// Manifest lists every asset, in address order.
var Manifest = []Entry{}
