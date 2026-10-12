package disasm

import (
	"strings"
	"testing"

	"go-msx-cryptexplorer/internal/rom"
	"go-msx-cryptexplorer/internal/z80dis"
)

func reader(b []byte) z80dis.Reader {
	return func(a uint16) byte {
		if int(a) >= rom.Base && int(a)-rom.Base < len(b) {
			return b[int(a)-rom.Base]
		}
		return 0
	}
}

func TestParseEntries(t *testing.T) {
	e, err := ParseEntries(strings.NewReader(`
; comment
code $407E INIT ; boot
data 4000 16 HEADER
data 0x4100 $10
table $5000 ? TBL from=$4FF0
table $5100 3
trampoline 406C JUMP_A
noreturn $4200
pushdata $4300
label $4400 NAME
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Code) != 2 || e.Code[0] != 0x407E || e.Names[0x407E] != "INIT" || e.Comments[0x407E] != "boot" {
		t.Errorf("code: %+v %v %v", e.Code, e.Names, e.Comments)
	}
	if len(e.Data) != 2 || e.Data[0] != (DataEntry{0x4000, 16}) || e.Data[1] != (DataEntry{0x4100, 16}) {
		t.Errorf("data: %+v", e.Data)
	}
	if len(e.Tables) != 2 || e.Tables[0] != (TableEntry{0x5000, -1, 0x4FF0, true}) || e.Tables[1].N != 3 {
		t.Errorf("tables: %+v", e.Tables)
	}
	if !e.Trampolines[0x406C] || !e.NoReturn[0x4200] || !e.PushData[0x4300] || e.Names[0x4400] != "NAME" {
		t.Errorf("flags: %+v", e)
	}
	if _, err := ParseEntries(strings.NewReader("bogus $4000")); err == nil {
		t.Error("unknown directive accepted")
	}
}

func TestSimulatePush(t *testing.T) {
	tramp := map[uint16]bool{0x406C: true}
	cases := []struct {
		name  string
		bytes []byte
		want  pushVerdict
	}{
		// PUSH HL / RET: the pushed value is the return address.
		{"continuation", []byte{0xE5, 0xC9}, pushContinuation},
		// PUSH HL / CALL trampoline: the handler's RET pops it.
		{"trampoline", []byte{0xE5, 0xCD, 0x6C, 0x40}, pushContinuation},
		// PUSH HL / POP HL / RET: a data pointer.
		{"popped", []byte{0xE5, 0xE1, 0xC9}, pushNone},
		// LD HL is overwritten before any push.
		{"overwritten", []byte{0x21, 0x00, 0x00, 0xE5, 0xC9}, pushNone},
		// PUSH HL / EX (SP),HL: needs a human.
		{"exsp", []byte{0xE5, 0xE3, 0xC9}, pushUnknown},
		// PUSH HL / PUSH DE / RET: not on top at the RET.
		{"depth", []byte{0xE5, 0xD5, 0xC9}, pushUnknown},
		// JR forward is followed.
		{"jr", []byte{0x18, 0x01, 0x00, 0xE5, 0xC9}, pushContinuation},
	}
	for _, c := range cases {
		got, why := simulatePush(reader(c.bytes), 0x4000, "HL", tramp)
		if got != c.want {
			t.Errorf("%s: got %v (%s), want %v", c.name, got, why, c.want)
		}
	}
}

// A synthetic ROM: CALL trampoline with an inline table of two entries that
// ends exactly at the first handler.
func TestInlineTableTight(t *testing.T) {
	b := make([]byte, rom.Size)
	for i := range b {
		b[i] = 0xFF
	}
	put := func(a int, bs ...byte) { copy(b[a-rom.Base:], bs) }
	put(0x4010, 0xCD, 0x00, 0x41) // CALL $4100 (trampoline)
	put(0x4013, 0x17, 0x40, 0x18, 0x40)
	put(0x4017, 0xC9)       // handler 0: RET
	put(0x4018, 0xC9)       // handler 1: RET
	put(0x4100, 0xE1, 0xE9) // POP HL / JP (HL) (shape only)
	e := NewEntries()
	e.Code = []uint16{0x4010}
	e.Trampolines[0x4100] = true
	e.Code = append(e.Code, 0x4100)
	a := Run(&rom.ROM{Data: b}, e)
	if len(a.Tables) != 1 || a.Tables[0].N != 2 || a.Tables[0].Status != "tight" {
		t.Fatalf("tables: %+v", a.Tables[0])
	}
	if len(a.Conflicts) != 0 {
		t.Errorf("conflicts: %v", a.Conflicts)
	}
}
