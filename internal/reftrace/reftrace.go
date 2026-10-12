// Package reftrace reads reference traces recorded from the original by
// scripts/refdump.tcl. Traces are ROM-derived and live outside the repo.
package reftrace

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	magic = "CXTRACE1"
	// RAMBase and RAMSize describe the game RAM in each record.
	RAMBase = 0xE000
	RAMSize = 0x1000
	// VRAMSize is the TMS9918A's 16KB.
	VRAMSize   = 0x4000
	headerSize = 8 + 4 + 4
	recordSize = 12 + 8 + 16 + RAMSize + VRAMSize
)

// Record is the original's state at the start of one game tick ($403E).
type Record struct {
	Tick       uint32
	Interrupts uint32 // H.KEYI entries so far
	Work       uint32 // $49C0 entries so far
	Regs       [8]byte
	PSG        [16]byte
	RAM        [RAMSize]byte
	VRAM       [VRAMSize]byte
}

// Trace is an open trace file.
type Trace struct {
	f *os.File
	// InitInterrupts and InitWork are the counts before INIT's final EI.
	InitInterrupts, InitWork uint32
	Len                      int // number of records
	Events                   []Event
}

// Event is one line of the .events file.
type Event struct {
	Kind byte // 'I' interrupt, 'W' work entered, 'R' LD A,R, 'K' input
	Tick int  // tick in progress (-1 before tick 0); for K, the tick it precedes
	// I: VRAM writes since the tick's record (since INIT entry when Tick
	// is -2), and the interrupted PC (in Ret).
	// W: VRAM writes since the tick's record, and the return address.
	// R: the value read. K: row, mask, down.
	VW   int
	Ret  uint16
	A    byte
	Row  int
	Mask byte
	Down bool
	// Busy is the busy flag at an I event (0: the interrupt starts a
	// tick), or -1 in traces recorded before it was logged. SS counts the
	// sound starts before the interrupt in its tick (0 when not logged).
	Busy int
	SS   int
}

// Open opens a trace and its events file (if present).
func Open(path string) (*Trace, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	var h [headerSize]byte
	if _, err := io.ReadFull(f, h[:]); err != nil || string(h[:8]) != magic {
		f.Close()
		return nil, fmt.Errorf("%s: not a trace", path)
	}
	t := &Trace{
		f:              f,
		InitInterrupts: binary.LittleEndian.Uint32(h[8:]),
		InitWork:       binary.LittleEndian.Uint32(h[12:]),
		Len:            int((st.Size() - headerSize) / recordSize),
	}
	if ev, err := os.Open(path + ".events"); err == nil {
		defer ev.Close()
		if t.Events, err = ParseEvents(ev); err != nil {
			f.Close()
			return nil, fmt.Errorf("%s.events: %w", path, err)
		}
	}
	return t, nil
}

// Close closes the trace.
func (t *Trace) Close() error { return t.f.Close() }

// Read returns record i.
func (t *Trace) Read(i int) (*Record, error) {
	if i < 0 || i >= t.Len {
		return nil, fmt.Errorf("record %d out of range 0-%d", i, t.Len-1)
	}
	var b [recordSize]byte
	if _, err := t.f.ReadAt(b[:], headerSize+int64(i)*recordSize); err != nil {
		return nil, err
	}
	r := &Record{
		Tick:       binary.LittleEndian.Uint32(b[0:]),
		Interrupts: binary.LittleEndian.Uint32(b[4:]),
		Work:       binary.LittleEndian.Uint32(b[8:]),
	}
	copy(r.Regs[:], b[12:20])
	copy(r.PSG[:], b[20:36])
	copy(r.RAM[:], b[36:36+RAMSize])
	copy(r.VRAM[:], b[36+RAMSize:])
	return r, nil
}

// ParseEvents parses the events format written by refdump.tcl.
func ParseEvents(r io.Reader) ([]Event, error) {
	var out []Event
	sc := bufio.NewScanner(r)
	for ln := 1; sc.Scan(); ln++ {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		bad := fmt.Errorf("line %d: %q", ln, sc.Text())
		num := func(i int) int {
			if i >= len(f) {
				panic(bad)
			}
			v, err := strconv.Atoi(f[i])
			if err != nil {
				panic(bad)
			}
			return v
		}
		ev, err := func() (ev Event, err error) {
			defer func() {
				if p := recover(); p != nil {
					err = bad
				}
			}()
			ev.Kind = f[0][0]
			ev.Tick = num(1)
			switch ev.Kind {
			case 'W', 'I':
				ev.VW = num(2)
				ev.Busy = -1
				if ev.Kind == 'I' && len(f) > 4 {
					ev.Busy = num(4)
				}
				if ev.Kind == 'I' && len(f) > 5 {
					ev.SS = num(5)
				}
				ret, perr := strconv.ParseUint(f[3], 16, 16)
				if perr != nil {
					return ev, bad
				}
				ev.Ret = uint16(ret)
			case 'R':
				ev.A = byte(num(2))
			case 'K':
				ev.Row, ev.Mask, ev.Down = num(2), byte(num(3)), f[4] == "down"
			default:
				return ev, bad
			}
			return ev, nil
		}()
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, sc.Err()
}
