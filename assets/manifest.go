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
var Manifest = []Entry{
	{Name: "d4323", Method: Raw, Addr: 0x4323, Len: 7,
		Note: "Referenced from $4317. Extent: to the next reference or claimed byte (Low)."},
	{Name: "vdpregs", Method: Raw, Addr: 0x45C0, Len: 8,
		Note: "R0-R7 written by $45AF via WRTVDP. High: layout confirmed by rendering the attract trace."},
	{Name: "s467d", Method: Raw, Addr: 0x467D, Len: 298, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 352}},
		Note:     "rle-noaddr stream used from $44CF."},
	{Name: "s47a7", Method: Raw, Addr: 0x47A7, Len: 3, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 8}},
		Note:     "rle-noaddr stream used from $42D5."},
	{Name: "s47aa", Method: Raw, Addr: 0x47AA, Len: 23, Stream: "text",
		Segments: []Segment{{0x3818, 5}, {0x3801, 6}, {0x380E, 3}},
		Note:     "text stream used from $4407."},
	{Name: "s47c1", Method: Raw, Addr: 0x47C1, Len: 32, Stream: "text",
		Segments: []Segment{{0x39AA, 12}, {0x3A49, 14}},
		Note:     "text stream used from $43F0."},
	{Name: "s47e1", Method: Raw, Addr: 0x47E1, Len: 17, Stream: "text",
		Segments: []Segment{{0x3A49, 14}},
		Note:     "text stream used from $42A9."},
	{Name: "s47f2", Method: Raw, Addr: 0x47F2, Len: 12, Stream: "text",
		Segments: []Segment{{0x396B, 9}},
		Note:     "text stream used from $4226."},
	{Name: "s47fe", Method: Raw, Addr: 0x47FE, Len: 16, Stream: "rle",
		Segments: []Segment{{0x394A, 42}},
		Note:     "rle stream used from $40DB."},
	{Name: "s480e", Method: Raw, Addr: 0x480E, Len: 21, Stream: "text",
		Segments: []Segment{{0x3AE1, 7}, {0x3AEB, 8}},
		Note:     "text stream used from $436B."},
	{Name: "s4874", Method: Raw, Addr: 0x4874, Len: 151, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 216}},
		Note:     "rle-noaddr stream used from $482E."},
	{Name: "s490b", Method: Raw, Addr: 0x490B, Len: 396, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 456}},
		Note:     "rle-noaddr stream used from $42DB."},
	{Name: "s4a97", Method: Raw, Addr: 0x4A97, Len: 20, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 88}},
		Note:     "rle-noaddr stream used from $42E4."},
	{Name: "s4aab", Method: Raw, Addr: 0x4AAB, Len: 17, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 16}},
		Note:     "rle-noaddr stream used from $42F4."},
	{Name: "d4abc", Method: Raw, Addr: 0x4ABC, Len: 18,
		Note: "Referenced from $43F6. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d4ace", Method: Raw, Addr: 0x4ACE, Len: 62,
		Note: "Referenced from $412D. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d4c09", Method: Raw, Addr: 0x4C09, Len: 4,
		Note: "Referenced from $4BEB. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d4c0d", Method: Raw, Addr: 0x4C0D, Len: 4,
		Note: "Referenced from $4BFE. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d4c52", Method: Raw, Addr: 0x4C52, Len: 7,
		Note: "Referenced from $4C41. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d4d46", Method: Raw, Addr: 0x4D46, Len: 10,
		Note: "Referenced from $4D0E. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d5075", Method: Raw, Addr: 0x5075, Len: 9,
		Note: "Referenced from $505E. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d518d", Method: Raw, Addr: 0x518D, Len: 8,
		Note: "Referenced from $5179. Extent: to the next reference or claimed byte (Low)."},
	{Name: "s520e", Method: Raw, Addr: 0x520E, Len: 188, Stream: "rle",
		Segments: []Segment{{0x1800, 191}},
		Note:     "Player sprite patterns, no item ($5075 entry 0, loaded by $5056). High."},
	{Name: "s52ca", Method: Raw, Addr: 0x52CA, Len: 307, Stream: "rle",
		Segments: []Segment{{0x1800, 320}},
		Note:     "Player sprite patterns with the dagger ($5075 entry 1). High."},
	{Name: "s53fd", Method: Raw, Addr: 0x53FD, Len: 313, Stream: "rle",
		Segments: []Segment{{0x1800, 320}},
		Note:     "Player sprite patterns with the pick ($5075 entry 2). High."},
	{Name: "s5536", Method: Raw, Addr: 0x5536, Len: 96, Stream: "rle",
		Segments: []Segment{{0x1940, 96}},
		Note:     "rle stream used from $5045."},
	{Name: "s5596", Method: Raw, Addr: 0x5596, Len: 162, Stream: "rle",
		Segments: []Segment{{0x1EA0, 352}},
		Note:     "rle stream used from $503F."},
	{Name: "s5638", Method: Raw, Addr: 0x5638, Len: 311, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 440}},
		Note:     "rle-noaddr stream used from $4FFE."},
	{Name: "s576f", Method: Raw, Addr: 0x576F, Len: 10, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 8}},
		Note:     "rle-noaddr stream used from $5026."},
	{Name: "s5779", Method: Raw, Addr: 0x5779, Len: 121, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 400}},
		Note:     "rle-noaddr stream used from $5007."},
	{Name: "s57dd", Method: Raw, Addr: 0x57DD, Len: 21, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 120}},
		Note:     "rle-noaddr stream used from $5018. Shares bytes with another stream."},
	{Name: "s57f2", Method: Raw, Addr: 0x57F2, Len: 13, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 48}},
		Note:     "rle-noaddr stream used from $5036."},
	{Name: "d58b1", Method: Raw, Addr: 0x58B1, Len: 5,
		Note: "Referenced from $58A9. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d5a63", Method: Raw, Addr: 0x5A63, Len: 8,
		Note: "Referenced from $5A26. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d5ba9", Method: Raw, Addr: 0x5BA9, Len: 4,
		Note: "Referenced from $5B70. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d5c4d", Method: Raw, Addr: 0x5C4D, Len: 4,
		Note: "Referenced from $5CFA. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d5cf6", Method: Raw, Addr: 0x5CF6, Len: 4,
		Note: "Referenced from $5CD9. Extent: to the next reference or claimed byte (Low)."},
	{Name: "stagedata", Method: Raw, Addr: 0x5D8B, Len: 0x6518 - 0x5D8B,
		Note: "Map cell tiles: 16-entry rows per cell class from the pointers at $5D8B ($5D75); brick templates ($6D8E, 44 bytes each: 22 rows of 16 bits, $6AA3); pyramid data for the 15 pyramids ($6D70). Ends where code starts at $6518. High (tight)."},
	{Name: "d6588", Method: Raw, Addr: 0x6588, Len: 2,
		Note: "Referenced from $5865. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d658a", Method: Raw, Addr: 0x658A, Len: 9,
		Note: "Referenced from $4C46, $6567, $6578, $6756, $6A5A. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d6593", Method: Raw, Addr: 0x6593, Len: 19,
		Note: "Referenced from $6935. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d67d0", Method: Raw, Addr: 0x67D0, Len: 4,
		Note: "Referenced from $667D. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d67d4", Method: Raw, Addr: 0x67D4, Len: 4,
		Note: "Referenced from $66E4. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d67d8", Method: Raw, Addr: 0x67D8, Len: 51,
		Note: "Referenced from $67C8. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d6a7b", Method: Raw, Addr: 0x6A7B, Len: 40,
		Note: "Referenced from $6981. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d6d44", Method: Raw, Addr: 0x6D44, Len: 5,
		Note: "Referenced from $6D21. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d6d70", Method: Raw, Addr: 0x6D70, Len: 30,
		Note: "Referenced from $6ABD. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d6d8e", Method: Raw, Addr: 0x6D8E, Len: 8,
		Note: "Referenced from $6AD2. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d6d96", Method: Raw, Addr: 0x6D96, Len: 4,
		Note: "Referenced from $6BC5. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d6dcc", Method: Raw, Addr: 0x6DCC, Len: 36,
		Note: "Referenced from $6DB0. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d6f4f", Method: Raw, Addr: 0x6F4F, Len: 5,
		Note: "Referenced from $6F07. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d6f54", Method: Raw, Addr: 0x6F54, Len: 16,
		Note: "Referenced from $6ECE. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d7078", Method: Raw, Addr: 0x7078, Len: 5,
		Note: "Referenced from $6F9F. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d70be", Method: Raw, Addr: 0x70BE, Len: 11,
		Note: "Referenced from $7099. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d7179", Method: Raw, Addr: 0x7179, Len: 5,
		Note: "Referenced from $7153. Extent: to the next reference or claimed byte (Low)."},
	{Name: "jumptable", Method: Raw, Addr: 0x7556, Len: 14,
		Note: "Jump arc: $FF (fall at 4 a frame), then the rising steps from $7557 to the $FE turn; read forwards rising and backwards falling by $74F4 (from $70E4). High."},
	{Name: "d75e0", Method: Raw, Addr: 0x75E0, Len: 4,
		Note: "Referenced from $75D2. Extent: to the next reference or claimed byte (Low)."},
	{Name: "s76e2", Method: Raw, Addr: 0x76E2, Len: 49, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 64}},
		Note:     "rle-noaddr stream used from $764E."},
	{Name: "s7713", Method: Raw, Addr: 0x7713, Len: 28, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 64}},
		Note:     "rle-noaddr stream used from $7657."},
	{Name: "d772f", Method: Raw, Addr: 0x772F, Len: 6,
		Note: "Referenced from $7683. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d7735", Method: Raw, Addr: 0x7735, Len: 7,
		Note: "Referenced from $769D. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d773c", Method: Raw, Addr: 0x773C, Len: 12,
		Note: "Referenced from $75C4. Extent: to the next reference or claimed byte (Low)."},
	{Name: "s7841", Method: Raw, Addr: 0x7841, Len: 169, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 320}},
		Note:     "rle-noaddr stream used from $774B."},
	{Name: "s78ea", Method: Raw, Addr: 0x78EA, Len: 41, Stream: "rle-noaddr",
		Segments: []Segment{{-1, 320}},
		Note:     "rle-noaddr stream used from $7754."},
	{Name: "s7913", Method: Raw, Addr: 0x7913, Len: 312, Stream: "rle",
		Segments: []Segment{{0x1F20, 160}, {0x3867, 562}},
		Note:     "rle stream used from $775D."},
	{Name: "d7a4b", Method: Raw, Addr: 0x7A4B, Len: 8,
		Note: "Referenced from $77C8. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d7a53", Method: Raw, Addr: 0x7A53, Len: 30,
		Note: "Referenced from $7830. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d7a71", Method: Raw, Addr: 0x7A71, Len: 8,
		Note: "Referenced from $7785. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d7a79", Method: Raw, Addr: 0x7A79, Len: 4,
		Note: "Referenced from $7778. Extent: to the next reference or claimed byte (Low)."},
	{Name: "d7a7d", Method: Raw, Addr: 0x7A7D, Len: 4,
		Note: "Referenced from $77BE. Extent: to the next reference or claimed byte (Low)."},
	{Name: "s7a81", Method: Raw, Addr: 0x7A81, Len: 40, Stream: "rle",
		Segments: []Segment{{0x38C9, 81}},
		Note:     "rle stream used from $781D."},
	{Name: "d7b8d", Method: Raw, Addr: 0x7B8D, Len: 4,
		Note: "Falling-tone template copied to $E03B-$E03E by LDDR from $7B90 (the sound driver at $7B65). High."},
	{Name: "sound", Method: Raw, Addr: 0x7CE9, Len: 0x8000 - 0x7CE9,
		Note: "Note periods ($7CE9, 10 used; a rest reads on into the next table), the channel stream pointers per sound id ($7CF3, from $7AD0) and the sound streams, to the end of the ROM. Medium: extent to be tightened."},
}
