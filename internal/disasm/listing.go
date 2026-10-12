package disasm

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"go-msx-cryptexplorer/internal/msxnames"
	"go-msx-cryptexplorer/internal/rom"
	"go-msx-cryptexplorer/internal/z80dis"
)

// Label returns the name of a ROM address: the entries name, or Lxxxx for
// code that is jumped to or called, or Dxxxx for other referenced bytes.
// It returns "" for addresses that need no label.
func (a *Analysis) Label(addr uint16) string {
	if n := a.ent.Names[addr]; n != "" {
		return n
	}
	if !rom.Contains(int(addr)) {
		return ""
	}
	if a.kind(int(addr)) == CodeStart {
		if a.Roots[addr] || a.Routines[addr] {
			return fmt.Sprintf("L%04X", addr)
		}
		for _, r := range a.Refs[addr] {
			if r.Kind == RefJump || r.Kind == RefCall || r.Kind == RefTable || r.Kind == RefPush {
				return fmt.Sprintf("L%04X", addr)
			}
		}
		return ""
	}
	for _, r := range a.Refs[addr] {
		if r.Kind == RefMem || r.Kind == RefImm || r.Kind == RefTable {
			return fmt.Sprintf("D%04X", addr)
		}
	}
	return ""
}

// FormatInst writes an instruction with labels for code targets, BIOS
// names for calls into the BIOS and system-variable names for memory
// operands. Immediates stay numeric; a comment names a matching label.
func (a *Analysis) FormatInst(in z80dis.Inst) (text, comment string) {
	var notes []string
	text = in.Format(func(kind byte, v uint16) string {
		hex := fmt.Sprintf("$%04X", v)
		switch kind {
		case 'T':
			if l := a.Label(v); l != "" {
				return l
			}
			if n := msxnames.BIOS[v]; n != "" && v < 0x4000 {
				return n
			}
		case 'M':
			if n := msxnames.SysVar(v); n != "" {
				return n
			}
			if l := a.Label(v); l != "" {
				return l
			}
		case 'I':
			if l := a.Label(v); l != "" {
				notes = append(notes, "="+l)
			} else if n := msxnames.SysVar(v); n != "" {
				notes = append(notes, "="+n)
			}
		}
		return hex
	})
	return text, strings.Join(notes, " ")
}

// WriteListing writes the whole ROM: code as instructions, everything
// else as DB/DW rows, with labels, entry comments and cross-references.
func (a *Analysis) WriteListing(w io.Writer) {
	code, data, table, un := a.Stats()
	fmt.Fprintf(w, "; Listing of %s, $%04X-$%04X\n", rom.DefaultPath, rom.Base, rom.End-1)
	fmt.Fprintf(w, "; ROM-derived: never commit this file.\n")
	fmt.Fprintf(w, "; code %d, data %d, tables %d, unclaimed %d bytes\n\n", code, data, table, un)
	a.WriteRange(w, rom.Base, rom.End)
}

// WriteRange writes the listing for [lo, hi). lo is moved back to the
// start of the item that contains it.
func (a *Analysis) WriteRange(w io.Writer, lo, hi int) {
	for lo > rom.Base && (a.kind(lo) == CodeCont || a.kind(lo) == TableWord && (lo-rom.Base)%2 == 1 && a.kind(lo-1) == TableWord) {
		lo--
	}
	addr := lo
	for addr < hi {
		u := uint16(addr)
		if l := a.Label(u); l != "" {
			fmt.Fprintf(w, "\n%s:%s\n", l, a.xrefs(u))
		}
		if c := a.ent.Comments[u]; c != "" {
			fmt.Fprintf(w, "        ; %s\n", c)
		}
		switch a.kind(addr) {
		case CodeStart:
			in := a.Insts[u]
			text, cmt := a.FormatInst(in)
			a.row(w, addr, in.Len, text, cmt)
			addr += in.Len
		case TableWord:
			v := uint16(a.ROM.Byte(addr)) | uint16(a.ROM.Byte(addr+1))<<8
			name := a.Label(v)
			if name == "" {
				name = fmt.Sprintf("$%04X", v)
			}
			a.row(w, addr, 2, "DW "+name, "")
			addr += 2
		default:
			// A DB row runs up to 8 bytes of one kind, stopping at labels.
			k := a.kind(addr)
			n := 1
			for n < 8 && addr+n < rom.End && a.kind(addr+n) == k && a.Label(uint16(addr+n)) == "" && a.ent.Comments[uint16(addr+n)] == "" {
				n++
			}
			var b []string
			for i := 0; i < n; i++ {
				b = append(b, fmt.Sprintf("$%02X", a.ROM.Byte(addr+i)))
			}
			cmt := ""
			if k == Unclaimed {
				cmt = "unclaimed"
			}
			a.row(w, addr, n, "DB "+strings.Join(b, ","), cmt)
			addr += n
		}
	}
}

func (a *Analysis) row(w io.Writer, addr, n int, text, cmt string) {
	var hb []string
	for i := 0; i < n && i < 4; i++ {
		hb = append(hb, fmt.Sprintf("%02X", a.ROM.Byte(addr+i)))
	}
	line := fmt.Sprintf("  %04X  %-12s  %s", addr, strings.Join(hb, " "), text)
	if cmt != "" {
		line = fmt.Sprintf("%-52s ; %s", line, cmt)
	}
	fmt.Fprintln(w, strings.TrimRight(line, " "))
}

func (a *Analysis) xrefs(addr uint16) string {
	rs := a.Refs[addr]
	if len(rs) == 0 {
		return ""
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].From < rs[j].From })
	var s []string
	for i, r := range rs {
		if i == 8 {
			s = append(s, fmt.Sprintf("+%d more", len(rs)-8))
			break
		}
		s = append(s, fmt.Sprintf("%c%04X", r.Kind, r.From))
	}
	return "    ; refs " + strings.Join(s, " ")
}
