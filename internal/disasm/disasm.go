// Package disasm is a recursive-descent disassembler for the cartridge with
// byte-level coverage tracking: every ROM byte is claimed at most once, as
// part of one instruction or as data, so a misaligned decode is reported as
// a conflict instead of quietly overwriting confirmed code.
package disasm

import (
	"fmt"
	"sort"

	"go-msx-cryptexplorer/internal/rom"
	"go-msx-cryptexplorer/internal/z80dis"
)

// Byte claim kinds.
const (
	Unclaimed byte = iota
	CodeStart      // first byte of an instruction
	CodeCont       // later byte of an instruction
	Data           // declared data
	TableWord      // a pointer in a jump table
)

// RefKind says how an address is referenced.
type RefKind byte

const (
	RefJump  RefKind = 'J' // JP/JR/DJNZ target
	RefCall  RefKind = 'C' // CALL or RST target
	RefMem   RefKind = 'M' // (nn) memory operand
	RefImm   RefKind = 'I' // 16-bit immediate
	RefTable RefKind = 'T' // entry of a jump table
	RefPush  RefKind = 'P' // pushed continuation
)

// Ref is one reference to an address.
type Ref struct {
	From uint16
	Kind RefKind
}

// Table is a run of little-endian code pointers. Unless Fixed, its length
// is unknown and it is grown one entry at a time (see Run).
type Table struct {
	Addr      uint16
	N         int
	Inline    bool   // follows a CALL to a trampoline
	Caller    uint16 // the CALL (inline tables) or JP (rr) that uses it
	HasCaller bool
	Fixed     bool   // length given in the entries file
	Status    string // for grown tables: "tight", or "heuristic: <why it stopped>"
	min       int    // lowest in-ROM target seen
}

// Analysis is the result of a descent over the ROM.
type Analysis struct {
	ROM   *rom.ROM
	Kind  []byte // claim kind per ROM byte
	Insts map[uint16]z80dis.Inst
	Refs  map[uint16][]Ref
	// Roots are the code entry points from the entries file plus
	// everything discovered (call targets, table entries, continuations).
	Roots     map[uint16]bool
	Routines  map[uint16]bool // call, RST and table targets and entries
	Tables    []*Table
	Conflicts []string
	Notes     []string            // things a human should look at
	Indirect  []uint16            // JP (rr) sites
	External  map[uint16][]uint16 // targets outside the ROM -> callers
	Streams   []*Stream

	ent  *Entries
	work []uint16
	seen map[string]bool // notes already emitted
}

func (a *Analysis) note(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	if !a.seen[s] {
		a.seen[s] = true
		a.Notes = append(a.Notes, s)
	}
}

func (a *Analysis) kind(addr int) byte { return a.Kind[addr-rom.Base] }

// Read is a z80dis.Reader over the ROM that returns $FF outside it.
func (a *Analysis) Read(addr uint16) byte {
	if rom.Contains(int(addr)) {
		return a.ROM.Byte(int(addr))
	}
	return 0xFF
}

// Run analyses r using the entries.
func Run(r *rom.ROM, e *Entries) *Analysis {
	a := &Analysis{
		ROM:      r,
		Kind:     make([]byte, rom.Size),
		Insts:    map[uint16]z80dis.Inst{},
		Refs:     map[uint16][]Ref{},
		Roots:    map[uint16]bool{},
		Routines: map[uint16]bool{},
		External: map[uint16][]uint16{},
		ent:      e,
		seen:     map[string]bool{},
	}
	for _, d := range e.Data {
		for i := 0; i < d.Len; i++ {
			p := int(d.Addr) + i
			if rom.Contains(p) {
				a.Kind[p-rom.Base] = Data
			}
		}
	}
	for _, t := range e.Tables {
		a.Tables = append(a.Tables, &Table{Addr: t.Addr, Fixed: t.N >= 0, Caller: t.From, HasCaller: t.HasFrom, min: 1 << 30})
		if t.N >= 0 {
			for i := 0; i < t.N; i++ {
				a.addTableEntry(a.Tables[len(a.Tables)-1])
			}
		}
	}
	for _, tt := range e.TableTables {
		for i := 0; i < tt.N; i++ {
			slot := int(tt.Addr) + 2*i
			if !rom.Contains(slot + 1) {
				a.Conflicts = append(a.Conflicts, fmt.Sprintf("tables $%04X entry %d outside ROM", tt.Addr, i))
				break
			}
			a.Kind[slot-rom.Base] = TableWord
			a.Kind[slot+1-rom.Base] = TableWord
			v := uint16(a.ROM.Byte(slot)) | uint16(a.ROM.Byte(slot+1))<<8
			a.Refs[v] = append(a.Refs[v], Ref{uint16(slot), RefTable})
			if !rom.Contains(int(v)) {
				a.Conflicts = append(a.Conflicts, fmt.Sprintf("tables $%04X entry %d = $%04X outside ROM", tt.Addr, i, v))
				continue
			}
			dup := false
			for _, t := range a.Tables {
				dup = dup || t.Addr == v
			}
			if !dup {
				a.Tables = append(a.Tables, &Table{Addr: v, min: 1 << 30})
			}
		}
	}
	for _, c := range e.Code {
		a.root(c)
		a.Routines[c] = true
	}
	a.drain()
	// Grow open tables one entry at a time, running the descent to
	// completion after each, so code found from earlier entries stops a
	// table from overrunning into it.
	for {
		grown := false
		for _, t := range a.Tables {
			if t.Fixed || t.Status != "" {
				continue
			}
			if why := a.canGrow(t); why != "" {
				end := int(t.Addr) + 2*t.N
				if end == t.min {
					t.Status = "tight"
				} else if rom.Contains(end) && a.kind(end) == CodeStart {
					t.Status = "tight (ends at code)"
				} else {
					t.Status = "heuristic: " + why
				}
				continue
			}
			a.addTableEntry(t)
			a.drain()
			grown = true
			break
		}
		if !grown {
			break
		}
	}
	a.claimStreams()
	a.trampolineHints()
	sort.Slice(a.Indirect, func(i, j int) bool { return a.Indirect[i] < a.Indirect[j] })
	return a
}

func (a *Analysis) canGrow(t *Table) string {
	slot := int(t.Addr) + 2*t.N
	if slot >= t.min {
		return "reached first target"
	}
	if !rom.Contains(slot) || !rom.Contains(slot+1) {
		return "end of ROM"
	}
	if a.kind(slot) != Unclaimed || a.kind(slot+1) != Unclaimed {
		return fmt.Sprintf("slot $%04X already claimed", slot)
	}
	v := int(a.ROM.Byte(slot)) | int(a.ROM.Byte(slot+1))<<8
	if !rom.Contains(v) {
		return fmt.Sprintf("entry $%04X points outside ROM", v)
	}
	if k := a.kind(v); k != Unclaimed && k != CodeStart {
		return fmt.Sprintf("entry $%04X points into claimed non-code", v)
	}
	return ""
}

func (a *Analysis) addTableEntry(t *Table) {
	slot := int(t.Addr) + 2*t.N
	if !rom.Contains(slot + 1) {
		a.Conflicts = append(a.Conflicts, fmt.Sprintf("table $%04X entry %d outside ROM", t.Addr, t.N))
		return
	}
	a.Kind[slot-rom.Base] = TableWord
	a.Kind[slot+1-rom.Base] = TableWord
	v := uint16(a.ROM.Byte(slot)) | uint16(a.ROM.Byte(slot+1))<<8
	t.N++
	a.Refs[v] = append(a.Refs[v], Ref{uint16(slot), RefTable})
	if rom.Contains(int(v)) {
		// Only targets after the table bound its length (tight packing).
		if int(v) > int(t.Addr) && int(v) < t.min {
			t.min = int(v)
		}
		a.Routines[v] = true
		a.root(v)
	}
}

func (a *Analysis) root(v uint16) {
	a.Roots[v] = true
	a.work = append(a.work, v)
}

func (a *Analysis) drain() {
	for len(a.work) > 0 {
		pc := a.work[len(a.work)-1]
		a.work = a.work[:len(a.work)-1]
		a.trace(pc)
	}
}

// trace decodes and claims code from pc until control cannot fall through.
func (a *Analysis) trace(pc uint16) {
	for {
		if !rom.Contains(int(pc)) {
			return
		}
		switch a.kind(int(pc)) {
		case CodeStart:
			return
		case Unclaimed:
		default:
			a.Conflicts = append(a.Conflicts, fmt.Sprintf("code flow reaches $%04X, already claimed as %s", pc, kindName(a.kind(int(pc)))))
			return
		}
		in := z80dis.Decode(a.Read, pc)
		for i := 1; i < in.Len; i++ {
			p := int(pc) + i
			if !rom.Contains(p) || a.kind(p) != Unclaimed {
				a.Conflicts = append(a.Conflicts, fmt.Sprintf("instruction at $%04X (%s) overlaps claimed byte $%04X", pc, in.Plain(), p))
				return
			}
		}
		if !in.Valid {
			a.note("$%04X: undefined opcode %s on a code path", pc, in.Plain())
		}
		a.Kind[int(pc)-rom.Base] = CodeStart
		for i := 1; i < in.Len; i++ {
			a.Kind[int(pc)+i-rom.Base] = CodeCont
		}
		a.Insts[pc] = in
		a.refs(in)
		next := pc + uint16(in.Len)
		switch in.Flow {
		case z80dis.FlowJump, z80dis.FlowCondJump:
			a.follow(in.Target, pc)
		case z80dis.FlowCall, z80dis.FlowCondCall, z80dis.FlowRst:
			a.follow(in.Target, pc)
			if rom.Contains(int(in.Target)) {
				a.Routines[in.Target] = true
			}
			if in.Flow == z80dis.FlowRst {
				switch in.Target {
				case 0x00:
					a.note("$%04X: RST $00 (reset) on a code path", pc)
					return
				case 0x08, 0x30:
					a.note("$%04X: %s takes inline bytes; descent stopped here", pc, in.Plain())
					return
				}
			}
			if a.ent.Trampolines[in.Target] && in.Flow != z80dis.FlowRst {
				a.inlineTable(next, pc)
				return
			}
			if a.ent.NoReturn[in.Target] && in.Flow == z80dis.FlowCall {
				return
			}
		case z80dis.FlowJumpIndirect:
			a.Indirect = append(a.Indirect, pc)
		}
		if in.Op == "LD" && in.HasImm16 && rom.Contains(int(in.Imm16)) {
			a.pushCheck(in)
		}
		if in.Flow.Ends() {
			return
		}
		pc = next
	}
}

func (a *Analysis) follow(t, from uint16) {
	if rom.Contains(int(t)) {
		a.work = append(a.work, t)
		return
	}
	a.External[t] = append(a.External[t], from)
}

func (a *Analysis) refs(in z80dis.Inst) {
	if in.HasTarget {
		k := RefJump
		if in.Flow == z80dis.FlowCall || in.Flow == z80dis.FlowCondCall || in.Flow == z80dis.FlowRst {
			k = RefCall
		}
		a.Refs[in.Target] = append(a.Refs[in.Target], Ref{in.Addr, k})
	}
	if in.HasMem {
		a.Refs[in.Mem] = append(a.Refs[in.Mem], Ref{in.Addr, RefMem})
		a.Refs[in.Mem+1] = append(a.Refs[in.Mem+1], Ref{in.Addr, RefMem}) // 16-bit accesses touch two bytes
	}
	if in.HasImm16 {
		a.Refs[in.Imm16] = append(a.Refs[in.Imm16], Ref{in.Addr, RefImm})
	}
}

func (a *Analysis) inlineTable(at, caller uint16) {
	for _, t := range a.Tables {
		if t.Addr == at {
			return
		}
	}
	a.Tables = append(a.Tables, &Table{Addr: at, Inline: true, Caller: caller, HasCaller: true, min: 1 << 30})
}

// pushCheck decides whether LD rr,nn at in pushes nn as a continuation
// that a later RET jumps to, by simulating the stack forward.
func (a *Analysis) pushCheck(ld z80dis.Inst) {
	reg := ld.Dst
	switch reg {
	case "BC", "DE", "HL", "IX", "IY":
	default:
		return
	}
	if a.ent.PushData[ld.Addr] {
		return
	}
	v := ld.Imm16
	verdict, why := simulatePush(a.Read, ld.Addr+uint16(ld.Len), reg, a.ent.Trampolines)
	switch verdict {
	case pushContinuation:
		a.Refs[v] = append(a.Refs[v], Ref{ld.Addr, RefPush})
		a.note("$%04X: %s pushed as a continuation (%s)", ld.Addr, ld.Plain(), why)
		a.root(v)
	case pushUnknown:
		a.note("$%04X: %s is pushed, fate unknown (%s); add 'code' or 'pushdata' to the entries", ld.Addr, ld.Plain(), why)
	}
}

type pushVerdict int

const (
	pushNone pushVerdict = iota // never pushed, or popped again: data
	pushContinuation
	pushUnknown
)

// simulatePush follows straight-line code from pc (unconditional jumps
// followed, conditional ones assumed not taken, calls assumed balanced).
//
// A CALL to a trampoline dispatches to a handler whose RET pops whatever is
// on the stack, so it counts as a RET at the current depth.
func simulatePush(rd z80dis.Reader, pc uint16, reg string, tramp map[uint16]bool) (pushVerdict, string) {
	pushed := false
	depth := 0
	for n := 0; n < 128; n++ {
		if !rom.Contains(int(pc)) {
			return pushNone, ""
		}
		in := z80dis.Decode(rd, pc)
		if !pushed {
			switch {
			case in.Op == "PUSH" && in.Dst == reg:
				pushed, depth = true, 1
			case writesReg(in, reg):
				return pushNone, ""
			case in.Flow.Ends() && in.Flow != z80dis.FlowJump:
				return pushNone, ""
			}
		} else {
			switch {
			case in.Op == "PUSH":
				depth++
			case in.Op == "POP":
				depth--
				if depth == 0 {
					return pushNone, fmt.Sprintf("popped at $%04X", pc)
				}
			case in.Op == "EX" && in.Dst == "(SP)", in.Op == "LD" && in.Dst == "SP", in.Op == "INC" && in.Dst == "SP", in.Op == "DEC" && in.Dst == "SP":
				return pushUnknown, fmt.Sprintf("%s at $%04X", in.Plain(), pc)
			case in.Flow == z80dis.FlowRet || in.Flow == z80dis.FlowCondRet ||
				(in.Flow == z80dis.FlowCall || in.Flow == z80dis.FlowCondCall) && tramp[in.Target]:
				if depth == 1 {
					return pushContinuation, fmt.Sprintf("on top at RET $%04X", pc)
				}
				return pushUnknown, fmt.Sprintf("depth %d at RET $%04X", depth, pc)
			case in.Flow == z80dis.FlowJumpIndirect:
				if depth == 1 {
					return pushContinuation, fmt.Sprintf("on top at %s $%04X", in.Plain(), pc)
				}
				return pushUnknown, fmt.Sprintf("depth %d at %s $%04X", depth, in.Plain(), pc)
			case in.Flow == z80dis.FlowHalt:
				return pushUnknown, fmt.Sprintf("HALT at $%04X", pc)
			}
		}
		if in.Flow == z80dis.FlowJump {
			pc = in.Target
			continue
		}
		if !pushed && in.Flow == z80dis.FlowCall && tramp[in.Target] {
			return pushNone, ""
		}
		pc += uint16(in.Len)
	}
	if pushed {
		return pushUnknown, "simulation limit"
	}
	return pushNone, ""
}

// writesReg reports whether in overwrites the 16-bit register reg (or a
// half of it).
func writesReg(in z80dis.Inst, reg string) bool {
	halves := map[string][2]string{"BC": {"B", "C"}, "DE": {"D", "E"}, "HL": {"H", "L"}, "IX": {"IXH", "IXL"}, "IY": {"IYH", "IYL"}}[reg]
	switch in.Op {
	case "LD", "POP", "INC", "DEC", "ADD", "ADC", "SBC", "IN":
		return in.Dst == reg || in.Dst == halves[0] || in.Dst == halves[1]
	case "EX":
		switch in.Dst {
		case "DE": // EX DE,HL
			return reg == "DE" || reg == "HL"
		case "(SP)":
			return in.Src == reg
		}
		return false
	case "EXX":
		return reg == "BC" || reg == "DE" || reg == "HL"
	case "LDIR", "LDDR", "LDI", "LDD", "CPIR", "CPDR", "INIR", "OTIR", "INDR", "OTDR":
		return reg != "IX" && reg != "IY"
	case "DJNZ":
		return reg == "BC"
	}
	return false
}

// trampolineHints notes call targets whose first instructions pop the
// return address: candidates for inline-table trampolines or for routines
// that do not return to their caller.
func (a *Analysis) trampolineHints() {
	var ts []uint16
	for t := range a.Routines {
		ts = append(ts, t)
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
	for _, t := range ts {
		if a.ent.Trampolines[t] || a.ent.NoReturn[t] {
			continue
		}
		pc := t
		for i := 0; i < 4; i++ {
			in, ok := a.Insts[pc]
			if !ok {
				break
			}
			if in.Op == "PUSH" || in.Flow != z80dis.FlowNext {
				break
			}
			if in.Op == "POP" || (in.Op == "EX" && in.Dst == "(SP)") {
				a.note("$%04X: starts by touching its return address (%s at $%04X); trampoline or no-return?", t, in.Plain(), pc)
				break
			}
			pc += uint16(in.Len)
		}
	}
}

func kindName(k byte) string {
	return [...]string{"unclaimed", "code", "instruction interior", "data", "table"}[k]
}

// Stats summarises claimed bytes.
func (a *Analysis) Stats() (code, data, table, unclaimed int) {
	for _, k := range a.Kind {
		switch k {
		case CodeStart, CodeCont:
			code++
		case Data:
			data++
		case TableWord:
			table++
		default:
			unclaimed++
		}
	}
	return
}

// Gap is a run of unclaimed bytes.
type Gap struct{ Start, End int } // [Start, End)

// Gaps lists the unclaimed ranges.
func (a *Analysis) Gaps() []Gap {
	var gs []Gap
	for i := 0; i < rom.Size; {
		if a.Kind[i] != Unclaimed {
			i++
			continue
		}
		j := i
		for j < rom.Size && a.Kind[j] == Unclaimed {
			j++
		}
		gs = append(gs, Gap{i + rom.Base, j + rom.Base})
		i = j
	}
	return gs
}

// RefsInto lists references to addresses in [start, end), sorted by target.
func (a *Analysis) RefsInto(start, end int) []struct {
	To uint16
	Ref
} {
	var out []struct {
		To uint16
		Ref
	}
	for to, rs := range a.Refs {
		if int(to) >= start && int(to) < end {
			for _, r := range rs {
				out = append(out, struct {
					To uint16
					Ref
				}{to, r})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].From < out[j].From
	})
	return out
}

// Reach lists the routines reachable from roots (following calls, jumps,
// tables and continuations) and counts the distinct instructions.
func (a *Analysis) Reach(roots []uint16) (routines []uint16, insts int) {
	seenInst := map[uint16]bool{}
	seenRoutine := map[uint16]bool{}
	var work []uint16
	push := func(t uint16, isRoutine bool) {
		if isRoutine && !seenRoutine[t] && rom.Contains(int(t)) {
			seenRoutine[t] = true
			routines = append(routines, t)
		}
		if !seenInst[t] {
			work = append(work, t)
		}
	}
	for _, r := range roots {
		push(r, true)
	}
	for len(work) > 0 {
		pc := work[len(work)-1]
		work = work[:len(work)-1]
		for {
			in, ok := a.Insts[pc]
			if !ok || seenInst[pc] {
				break
			}
			seenInst[pc] = true
			if in.HasTarget {
				isCall := in.Flow == z80dis.FlowCall || in.Flow == z80dis.FlowCondCall || in.Flow == z80dis.FlowRst
				push(in.Target, isCall)
			}
			for _, r := range a.refsFrom(pc) {
				push(r, true)
			}
			if in.Flow.Ends() {
				break
			}
			pc += uint16(in.Len)
		}
	}
	sort.Slice(routines, func(i, j int) bool { return routines[i] < routines[j] })
	return routines, len(seenInst)
}

// refsFrom returns continuation and table targets attributable to the
// instruction at pc (a pushed LD, or a CALL to a trampoline).
func (a *Analysis) refsFrom(pc uint16) []uint16 {
	var out []uint16
	in := a.Insts[pc]
	if in.HasImm16 {
		for _, r := range a.Refs[in.Imm16] {
			if r.From == pc && r.Kind == RefPush {
				out = append(out, in.Imm16)
			}
		}
	}
	for _, t := range a.Tables {
		if t.HasCaller && t.Caller == pc {
			for i := 0; i < t.N; i++ {
				s := int(t.Addr) + 2*i
				out = append(out, uint16(a.ROM.Byte(s))|uint16(a.ROM.Byte(s+1))<<8)
			}
		}
	}
	return out
}
