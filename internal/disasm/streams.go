package disasm

import (
	"fmt"
	"sort"

	"go-msx-cryptexplorer/internal/rom"
	"go-msx-cryptexplorer/internal/streams"
	"go-msx-cryptexplorer/internal/z80dis"
)

// Stream formats for the streamcall and stream directives.
var streamFormats = map[string]func(rd streams.Reader, addr int) (int, error){
	"rle": func(rd streams.Reader, a int) (int, error) {
		_, n, err := streams.RLE(rd, a, true, false)
		return n, err
	},
	"rle-noaddr": func(rd streams.Reader, a int) (int, error) {
		_, n, err := streams.RLE(rd, a, false, false)
		return n, err
	},
	"rle-ram": func(rd streams.Reader, a int) (int, error) {
		_, n, err := streams.RLERAM(rd, a)
		return n, err
	},
	"text": func(rd streams.Reader, a int) (int, error) {
		_, n, err := streams.Text(rd, a, true)
		return n, err
	},
	"text-noaddr": func(rd streams.Reader, a int) (int, error) {
		_, n, err := streams.Text(rd, a, false)
		return n, err
	},
}

// Stream is a data stream found passed to a stream routine.
type Stream struct {
	Addr   uint16
	Len    int
	Format string
	From   []uint16 // the LD DE,nn instructions (or 0 for a directive)
	Shared bool     // overlaps another stream (a shared tail)
}

// claimStreams walks every stream passed in DE to a streamcall routine,
// plus the stream directives, and claims their bytes as data.
func (a *Analysis) claimStreams() {
	found := map[uint16]*Stream{}
	add := func(addr uint16, format string, from uint16) {
		if s, ok := found[addr]; ok {
			if s.Format != format {
				a.Conflicts = append(a.Conflicts, fmt.Sprintf("stream $%04X used as %s and %s", addr, s.Format, format))
			}
			s.From = append(s.From, from)
			return
		}
		found[addr] = &Stream{Addr: addr, Format: format, From: []uint16{from}}
	}
	for _, s := range a.ent.Streams {
		add(s.Addr, s.Format, 0)
	}
	for _, st := range a.ent.StreamTables {
		for i := 0; i < st.N; i++ {
			slot := int(st.Addr) + 2*i
			if !rom.Contains(slot+1) || a.kind(slot) != Unclaimed && a.kind(slot) != Data {
				a.Conflicts = append(a.Conflicts, fmt.Sprintf("streamtable $%04X entry %d at $%04X not free", st.Addr, i, slot))
				continue
			}
			a.Kind[slot-rom.Base] = Data
			a.Kind[slot+1-rom.Base] = Data
			v := uint16(a.ROM.Byte(slot)) | uint16(a.ROM.Byte(slot+1))<<8
			a.Refs[v] = append(a.Refs[v], Ref{uint16(slot), RefTable})
			add(v, st.Format, uint16(slot))
		}
	}
	for pc, in := range a.Insts {
		if in.Op != "LD" || in.Dst != "DE" || !in.HasImm16 || !rom.Contains(int(in.Imm16)) {
			continue
		}
		if f := a.streamCallAfter(pc + uint16(in.Len)); f != "" {
			add(in.Imm16, f, pc)
		}
	}
	rd := func(p int) byte {
		if rom.Contains(p) {
			return a.ROM.Byte(p)
		}
		return 0xFF
	}
	var list []*Stream
	for _, s := range found {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Addr < list[j].Addr })
	for _, s := range list {
		n, err := streamFormats[s.Format](rd, int(s.Addr))
		if err != nil {
			a.Conflicts = append(a.Conflicts, err.Error())
			continue
		}
		s.Len = n
		for i := 0; i < n; i++ {
			p := int(s.Addr) + i
			if !rom.Contains(p) {
				a.Conflicts = append(a.Conflicts, fmt.Sprintf("%s stream $%04X runs past the ROM", s.Format, s.Addr))
				break
			}
			switch a.kind(p) {
			case Unclaimed:
				a.Kind[p-rom.Base] = Data
			case Data:
				s.Shared = true
			default:
				a.Conflicts = append(a.Conflicts, fmt.Sprintf("%s stream $%04X (%d bytes) runs into %s at $%04X", s.Format, s.Addr, n, kindName(a.kind(p)), p))
				i = n
			}
		}
		sort.Slice(s.From, func(i, j int) bool { return s.From[i] < s.From[j] })
		a.Streams = append(a.Streams, s)
	}
}

// streamCallAfter follows straight-line code from pc for a few
// instructions and returns the format if it reaches a CALL or JP to a
// streamcall routine without DE being overwritten.
func (a *Analysis) streamCallAfter(pc uint16) string {
	for n := 0; n < 8; n++ {
		in, ok := a.Insts[pc]
		if !ok {
			return ""
		}
		if in.HasTarget && (in.Flow == z80dis.FlowCall || in.Flow == z80dis.FlowJump || in.Flow == z80dis.FlowCondCall || in.Flow == z80dis.FlowCondJump) {
			if f := a.ent.StreamCalls[in.Target]; f != "" {
				return f
			}
		}
		if writesReg(in, "DE") || in.Flow != z80dis.FlowNext {
			return ""
		}
		pc += uint16(in.Len)
	}
	return ""
}
