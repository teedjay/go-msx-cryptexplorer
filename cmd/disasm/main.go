// Command disasm disassembles the cartridge by recursive descent from the
// entries in docs/entries.txt, tracking which bytes each instruction claims.
//
//	go run ./cmd/disasm -o docs/listing.asm   full listing (git-ignored)
//	go run ./cmd/disasm -stats                coverage, conflicts, notes, tables
//	go run ./cmd/disasm -gaps                 unclaimed ranges and what references them
//	go run ./cmd/disasm -peek 4000-4040       linear decode without claiming
//	go run ./cmd/disasm -list 40D3-4270       listing for a range
//	go run ./cmd/disasm -reach 407E,402E      routines reachable from roots
//	go run ./cmd/disasm -indirect             JP (rr) sites with the code before them
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"go-msx-cryptexplorer/internal/disasm"
	"go-msx-cryptexplorer/internal/rom"
	"go-msx-cryptexplorer/internal/z80dis"
)

func main() {
	romPath := flag.String("rom", rom.DefaultPath, "cartridge image")
	entPath := flag.String("entries", "docs/entries.txt", "entries file")
	out := flag.String("o", "", "write the listing to this file")
	stats := flag.Bool("stats", false, "print coverage, conflicts, notes and tables")
	gaps := flag.Bool("gaps", false, "list unclaimed ranges with the instructions that reference into each")
	peek := flag.String("peek", "", "linear decode of A-B (hex) without claiming")
	reach := flag.String("reach", "", "comma-separated roots: list reachable routines")
	indirect := flag.Bool("indirect", false, "list JP (rr) sites with preceding instructions")
	list := flag.String("list", "", "print the listing for A-B (hex)")
	streamsFlag := flag.Bool("streams", false, "list claimed streams and what follows each")
	codelike := flag.Bool("codelike", false, "list gaps that decode cleanly as code, with ROM words equal to their start")
	datamap := flag.Bool("datamap", false, "split gaps at referenced addresses: a first-pass data map")
	manifest := flag.Bool("manifest", false, "print Go manifest entries for every stream, stream table and referenced gap piece")
	flag.Parse()

	r, err := rom.Load(*romPath)
	if err != nil {
		fail(err)
	}
	r.Warn(os.Stderr)
	ent, err := disasm.LoadEntries(*entPath)
	if err != nil {
		fail(err)
	}
	a := disasm.Run(r, ent)

	did := false
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fail(err)
		}
		bw := bufio.NewWriter(f)
		a.WriteListing(bw)
		if err := bw.Flush(); err != nil {
			fail(err)
		}
		f.Close()
		did = true
	}
	if *peek != "" {
		lo, hi, err := parseRange(*peek)
		if err != nil {
			fail(err)
		}
		for pc := lo; pc < hi; {
			in := z80dis.Decode(a.Read, uint16(pc))
			text, cmt := a.FormatInst(in)
			claim := ""
			if rom.Contains(pc) && a.Kind[pc-rom.Base] != disasm.Unclaimed {
				claim = "  [claimed]"
			}
			if cmt != "" {
				cmt = " ; " + cmt
			}
			fmt.Printf("  %04X  %-12s  %s%s%s\n", pc, hexBytes(r, pc, in.Len), text, cmt, claim)
			pc += in.Len
		}
		did = true
	}
	if *list != "" {
		lo, hi, err := parseRange(*list)
		if err != nil {
			fail(err)
		}
		a.WriteRange(os.Stdout, lo, hi)
		did = true
	}
	if *streamsFlag {
		for _, st := range a.Streams {
			end := int(st.Addr) + st.Len
			next := "end of ROM"
			if rom.Contains(end) {
				next = map[byte]string{disasm.Unclaimed: "unclaimed", disasm.CodeStart: "code", disasm.CodeCont: "INSIDE CODE", disasm.Data: "data", disasm.TableWord: "table"}[a.Kind[end-rom.Base]]
			}
			shared := ""
			if st.Shared {
				shared = " shared"
			}
			fmt.Printf("$%04X-$%04X %-11s %5d bytes, then %-9s from %04X%s\n", st.Addr, end-1, st.Format, st.Len, next, st.From, shared)
		}
		did = true
	}
	if *manifest {
		writeManifest(os.Stdout, a, r)
		did = true
	}
	if *datamap {
		printDatamap(a, r)
		did = true
	}
	if *codelike {
		printCodelike(a, r)
		did = true
	}
	if *gaps {
		printGaps(a)
		did = true
	}
	if *reach != "" {
		var roots []uint16
		for _, s := range strings.Split(*reach, ",") {
			v, err := disasm.ParseAddr(strings.TrimSpace(s))
			if err != nil {
				fail(fmt.Errorf("bad root %q", s))
			}
			roots = append(roots, v)
		}
		rs, n := a.Reach(roots)
		for _, t := range rs {
			name := a.Label(t)
			fmt.Printf("  %04X  %s\n", t, name)
		}
		fmt.Printf("%d routines, %d instructions\n", len(rs), n)
		did = true
	}
	if *indirect {
		for _, s := range a.Indirect {
			fmt.Printf("$%04X:\n", s)
			// Show up to six instructions before, walking back through
			// claimed starts.
			var before []uint16
			for pc := int(s) - 1; pc >= rom.Base && len(before) < 6 && int(s)-pc < 24; pc-- {
				if a.Kind[pc-rom.Base] == disasm.CodeStart {
					before = append([]uint16{uint16(pc)}, before...)
				}
			}
			for _, pc := range append(before, s) {
				text, _ := a.FormatInst(a.Insts[pc])
				fmt.Printf("    %04X  %s\n", pc, text)
			}
		}
		did = true
	}
	if *stats || !did {
		printStats(a)
	}
}

func printStats(a *disasm.Analysis) {
	code, data, table, un := a.Stats()
	pct := func(n int) float64 { return 100 * float64(n) / rom.Size }
	fmt.Printf("code %d (%.1f%%), data %d (%.1f%%), tables %d, unclaimed %d (%.1f%%)\n",
		code, pct(code), data, pct(data), table, un, pct(un))
	fmt.Printf("%d instructions, %d routines, %d JP (rr) sites\n", len(a.Insts), len(a.Routines), len(a.Indirect))
	for _, t := range a.Tables {
		kind := "table"
		if t.Inline {
			kind = fmt.Sprintf("inline table after CALL $%04X", t.Caller)
		}
		st := t.Status
		if t.Fixed {
			st = "fixed"
		}
		fmt.Printf("%s $%04X: %d entries (%s)\n", kind, t.Addr, t.N, st)
	}
	sb := 0
	for _, s := range a.Streams {
		sb += s.Len
	}
	fmt.Printf("%d streams, %d bytes\n", len(a.Streams), sb)
	if len(a.Conflicts) > 0 {
		fmt.Printf("\n%d CONFLICTS:\n", len(a.Conflicts))
		for _, c := range a.Conflicts {
			fmt.Println("  " + c)
		}
	}
	if len(a.Notes) > 0 {
		fmt.Printf("\nnotes:\n")
		for _, n := range a.Notes {
			fmt.Println("  " + n)
		}
	}
	if len(a.External) > 0 {
		fmt.Printf("\ntargets outside the ROM:\n")
		for _, t := range sortedKeys(a.External) {
			name, _ := a.FormatInst(z80dis.Inst{Text: fmt.Sprintf("{T:%04X}", t)})
			fmt.Printf("  %-8s from %d site(s)\n", name, len(a.External[t]))
		}
	}
}

func printGaps(a *disasm.Analysis) {
	for _, g := range a.Gaps() {
		fmt.Printf("$%04X-$%04X (%d bytes)\n", g.Start, g.End-1, g.End-g.Start)
		for _, r := range a.RefsInto(g.Start, g.End) {
			text, _ := a.FormatInst(a.Insts[r.From])
			if r.Kind == disasm.RefTable {
				text = "table entry"
			}
			fmt.Printf("    -> $%04X  %c from %04X  %s\n", r.To, r.Kind, r.From, text)
		}
	}
}

// printCodelike linear-decodes each gap from its start. A gap is listed if
// it decodes without undefined opcodes and reaches RET, JP or JR inside
// the gap. Every 16-bit word in the ROM equal to its start is shown, since
// unreferenced code may still be reached through a pointer.
func printCodelike(a *disasm.Analysis, r *rom.ROM) {
	for _, g := range a.Gaps() {
		pc, n, ok := g.Start, 0, false
		for pc < g.End {
			in := z80dis.Decode(a.Read, uint16(pc))
			if !in.Valid || pc+in.Len > g.End {
				break
			}
			n++
			pc += in.Len
			if in.Flow == z80dis.FlowRet || in.Flow == z80dis.FlowJump || in.Flow == z80dis.FlowJumpIndirect {
				ok = true
				break
			}
		}
		if !ok {
			continue
		}
		var words []string
		for i := rom.Base; i+1 < rom.End; i++ {
			if int(r.Byte(i))|int(r.Byte(i+1))<<8 == g.Start {
				words = append(words, fmt.Sprintf("%04X", i))
			}
		}
		refs := len(a.RefsInto(g.Start, g.End))
		fmt.Printf("$%04X-$%04X (%d bytes): %d instructions to a terminal at $%04X; %d refs into the gap; word $%04X at %v\n",
			g.Start, g.End-1, g.End-g.Start, n, pc-1, refs, g.Start, words)
	}
}

// printDatamap splits each gap at every referenced address. A piece with
// no reference at its start is marked UNREFERENCED and shown with the ROM
// words (if any) equal to its start.
func printDatamap(a *disasm.Analysis, r *rom.ROM) {
	for _, g := range a.Gaps() {
		refs := a.RefsInto(g.Start, g.End)
		cuts := []int{g.Start}
		by := map[int][]uint16{}
		for _, x := range refs {
			if int(x.To) != cuts[len(cuts)-1] {
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
			if froms := by[lo]; len(froms) > 0 {
				fmt.Printf("$%04X-$%04X %5d  from %04X\n", lo, hi-1, hi-lo, froms)
				continue
			}
			var words []string
			for j := rom.Base; j+1 < rom.End; j++ {
				if int(r.Byte(j))|int(r.Byte(j+1))<<8 == lo {
					words = append(words, fmt.Sprintf("%04X", j))
				}
			}
			fmt.Printf("$%04X-$%04X %5d  UNREFERENCED  words at %v\n", lo, hi-1, hi-lo, words)
		}
	}
}

func sortedKeys(m map[uint16][]uint16) []uint16 {
	var ks []uint16
	for k := range m {
		ks = append(ks, k)
	}
	for i := range ks {
		for j := i + 1; j < len(ks); j++ {
			if ks[j] < ks[i] {
				ks[i], ks[j] = ks[j], ks[i]
			}
		}
	}
	return ks
}

func parseRange(s string) (int, int, error) {
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0, fmt.Errorf("range %q: want A-B", s)
	}
	lo, err1 := disasm.ParseAddr(a)
	hi, err2 := disasm.ParseAddr(b)
	if err1 != nil || err2 != nil || hi < lo {
		return 0, 0, fmt.Errorf("bad range %q", s)
	}
	return int(lo), int(hi), nil
}

func hexBytes(r *rom.ROM, pc, n int) string {
	var s []string
	for i := 0; i < n && i < 4; i++ {
		if rom.Contains(pc + i) {
			s = append(s, fmt.Sprintf("%02X", r.Byte(pc+i)))
		}
	}
	return strings.Join(s, " ")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "disasm:", err)
	os.Exit(1)
}
