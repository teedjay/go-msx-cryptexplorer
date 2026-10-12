package disasm

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Entries is the hand-maintained knowledge the descent starts from
// (docs/entries.txt). It holds addresses, lengths and names only, never
// ROM bytes. Format, one directive per line, ';' starts a comment that is
// shown in the listing next to that address:
//
//	code ADDR [NAME]            descent root (and routine)
//	label ADDR NAME             name an address without claiming it
//	data ADDR LEN [NAME]        claim LEN bytes as data
//	table ADDR N|? [NAME] [from=ADDR]
//	                            N code pointers ('?' grows heuristically);
//	                            from= names the JP (rr) that uses it
//	tables ADDR N [NAME]        N pointers to code tables (each grown like
//	                            table ?), e.g. per-object-type handler tables
//	trampoline ADDR [NAME]      a CALL to ADDR is followed by an inline table
//	noreturn ADDR [NAME]        a CALL to ADDR never returns to the caller
//	pushdata ADDR               the LD rr,nn at ADDR pushes data, not code
//	streamcall ADDR FORMAT      LD DE,nn followed by a CALL/JP to ADDR passes
//	                            a stream at nn (formats: rle, rle-noaddr,
//	                            text, text-noaddr; see internal/streams)
//	stream ADDR FORMAT [NAME]   a stream reached some other way
//	streamtable ADDR N FORMAT [NAME]
//	                            N pointers to streams of FORMAT
//
// Addresses are hex, with or without '$' or '0x'.
type Entries struct {
	Code         []uint16
	Names        map[uint16]string
	Comments     map[uint16]string
	Data         []DataEntry
	Tables       []TableEntry
	TableTables  []TableEntry
	Trampolines  map[uint16]bool
	NoReturn     map[uint16]bool
	PushData     map[uint16]bool
	StreamCalls  map[uint16]string
	Streams      []StreamEntry
	StreamTables []StreamEntry
}

type StreamEntry struct {
	Addr   uint16
	Format string
	N      int // streamtable only
}

type DataEntry struct {
	Addr uint16
	Len  int
}

type TableEntry struct {
	Addr    uint16
	N       int // -1: grow
	From    uint16
	HasFrom bool
}

// NewEntries returns an empty set.
func NewEntries() *Entries {
	return &Entries{
		Names: map[uint16]string{}, Comments: map[uint16]string{},
		Trampolines: map[uint16]bool{}, NoReturn: map[uint16]bool{}, PushData: map[uint16]bool{},
		StreamCalls: map[uint16]string{},
	}
}

// LoadEntries reads an entries file.
func LoadEntries(path string) (*Entries, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	e, err := ParseEntries(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return e, nil
}

// ParseAddr parses a hex address such as $407E, 0x407E or 407E.
func ParseAddr(s string) (uint16, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(s, "$"), "0x"), "0X")
	v, err := strconv.ParseUint(s, 16, 16)
	return uint16(v), err
}

// ParseEntries parses the entries format.
func ParseEntries(r io.Reader) (*Entries, error) {
	e := NewEntries()
	sc := bufio.NewScanner(r)
	for ln := 1; sc.Scan(); ln++ {
		line, comment, _ := strings.Cut(sc.Text(), ";")
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		bad := func(why string) error { return fmt.Errorf("line %d: %s: %q", ln, why, sc.Text()) }
		if len(f) < 2 {
			return nil, bad("missing address")
		}
		addr, err := ParseAddr(f[1])
		if err != nil {
			return nil, bad("bad address")
		}
		if c := strings.TrimSpace(comment); c != "" {
			if old := e.Comments[addr]; old != "" {
				c = old + "; " + c
			}
			e.Comments[addr] = c
		}
		rest := f[2:]
		name := func(args []string) {
			for _, a := range args {
				if !strings.Contains(a, "=") {
					e.Names[addr] = a
					return
				}
			}
		}
		switch f[0] {
		case "code":
			e.Code = append(e.Code, addr)
			name(rest)
		case "label":
			if len(rest) == 0 {
				return nil, bad("label needs a name")
			}
			name(rest)
		case "data":
			if len(rest) == 0 {
				return nil, bad("data needs a length")
			}
			n, err := strconv.ParseInt(strings.TrimPrefix(rest[0], "$"), 0, 32)
			if strings.HasPrefix(rest[0], "$") {
				n, err = strconv.ParseInt(rest[0][1:], 16, 32)
			}
			if err != nil || n <= 0 {
				return nil, bad("bad length (decimal, or $hex)")
			}
			e.Data = append(e.Data, DataEntry{addr, int(n)})
			name(rest[1:])
		case "table":
			if len(rest) == 0 {
				return nil, bad("table needs a count or ?")
			}
			t := TableEntry{Addr: addr, N: -1}
			if rest[0] != "?" {
				n, err := strconv.Atoi(rest[0])
				if err != nil || n <= 0 {
					return nil, bad("bad count")
				}
				t.N = n
			}
			for _, a := range rest[1:] {
				if v, ok := strings.CutPrefix(a, "from="); ok {
					if t.From, err = ParseAddr(v); err != nil {
						return nil, bad("bad from=")
					}
					t.HasFrom = true
				}
			}
			e.Tables = append(e.Tables, t)
			name(rest[1:])
		case "tables":
			if len(rest) == 0 {
				return nil, bad("tables needs a count")
			}
			n, err := strconv.Atoi(rest[0])
			if err != nil || n <= 0 {
				return nil, bad("bad count")
			}
			e.TableTables = append(e.TableTables, TableEntry{Addr: addr, N: n})
			name(rest[1:])
		case "trampoline":
			e.Trampolines[addr] = true
			e.Code = append(e.Code, addr)
			name(rest)
		case "noreturn":
			e.NoReturn[addr] = true
			name(rest)
		case "pushdata":
			e.PushData[addr] = true
		case "streamcall", "stream":
			if len(rest) == 0 || streamFormats[rest[0]] == nil {
				return nil, bad("needs a format: rle, rle-noaddr, text or text-noaddr")
			}
			if f[0] == "streamcall" {
				e.StreamCalls[addr] = rest[0]
			} else {
				e.Streams = append(e.Streams, StreamEntry{Addr: addr, Format: rest[0]})
				name(rest[1:])
			}
		case "streamtable":
			if len(rest) < 2 || streamFormats[rest[1]] == nil {
				return nil, bad("streamtable ADDR N FORMAT")
			}
			n, err := strconv.Atoi(rest[0])
			if err != nil || n <= 0 {
				return nil, bad("bad count")
			}
			e.StreamTables = append(e.StreamTables, StreamEntry{Addr: addr, Format: rest[1], N: n})
			name(rest[2:])
		default:
			return nil, bad("unknown directive")
		}
	}
	return e, sc.Err()
}
