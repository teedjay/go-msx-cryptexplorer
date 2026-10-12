package z80dis

import "testing"

func reader(b []byte) Reader {
	return func(a uint16) byte {
		if int(a-0x4000) < len(b) {
			return b[a-0x4000]
		}
		return 0
	}
}

func TestDecode(t *testing.T) {
	cases := []struct {
		bytes []byte
		text  string
		len   int
		flow  Flow
	}{
		{[]byte{0x00}, "NOP", 1, FlowNext},
		{[]byte{0xF3}, "DI", 1, FlowNext},
		{[]byte{0xED, 0x56}, "IM 1", 2, FlowNext},
		{[]byte{0x31, 0x00, 0xE8}, "LD SP,$E800", 3, FlowNext},
		{[]byte{0xCD, 0x38, 0x01}, "CALL $0138", 3, FlowCall},
		{[]byte{0x18, 0xFE}, "JR $4000", 2, FlowJump},
		{[]byte{0x20, 0x0C}, "JR NZ,$400E", 2, FlowCondJump},
		{[]byte{0x10, 0x02}, "DJNZ $4004", 2, FlowCondJump},
		{[]byte{0x32, 0x9A, 0xFD}, "LD ($FD9A),A", 3, FlowNext},
		{[]byte{0x22, 0x9B, 0xFD}, "LD ($FD9B),HL", 3, FlowNext},
		{[]byte{0x2A, 0x00, 0xE0}, "LD HL,($E000)", 3, FlowNext},
		{[]byte{0xED, 0x4B, 0x00, 0xE0}, "LD BC,($E000)", 4, FlowNext},
		{[]byte{0xCB, 0x46}, "BIT 0,(HL)", 2, FlowNext},
		{[]byte{0xCB, 0x38}, "SRL B", 2, FlowNext},
		{[]byte{0xDD, 0x36, 0x06, 0x80}, "LD (IX+$06),$80", 4, FlowNext},
		{[]byte{0xDD, 0x7E, 0xFE}, "LD A,(IX-$02)", 3, FlowNext},
		{[]byte{0xDD, 0x66, 0x01}, "LD H,(IX+$01)", 3, FlowNext},
		{[]byte{0xDD, 0x74, 0x01}, "LD (IX+$01),H", 3, FlowNext},
		{[]byte{0xDD, 0x7C}, "LD A,IXH", 2, FlowNext},
		{[]byte{0xFD, 0x21, 0x34, 0x12}, "LD IY,$1234", 4, FlowNext},
		{[]byte{0xFD, 0xE9}, "JP (IY)", 2, FlowJumpIndirect},
		{[]byte{0xE9}, "JP (HL)", 1, FlowJumpIndirect},
		{[]byte{0xDD, 0xCB, 0x05, 0x1E}, "RR (IX+$05)", 4, FlowNext},
		{[]byte{0xDD, 0xCB, 0x05, 0x46}, "BIT 0,(IX+$05)", 4, FlowNext},
		{[]byte{0xDD, 0xCB, 0x05, 0x00}, "RLC (IX+$05),B", 4, FlowNext},
		{[]byte{0xDD, 0xE5}, "PUSH IX", 2, FlowNext},
		{[]byte{0xDD, 0xE3}, "EX (SP),IX", 2, FlowNext},
		{[]byte{0xEB}, "EX DE,HL", 1, FlowNext},
		{[]byte{0xED, 0xB0}, "LDIR", 2, FlowNext},
		{[]byte{0xED, 0x5F}, "LD A,R", 2, FlowNext},
		{[]byte{0xED, 0x52}, "SBC HL,DE", 2, FlowNext},
		{[]byte{0xED, 0x45}, "RETN", 2, FlowRet},
		{[]byte{0xED, 0x00}, "DB $ED,$00", 2, FlowNext},
		{[]byte{0xDD, 0xDD, 0x00}, "DB $DD", 1, FlowNext},
		{[]byte{0xFC, 0xC0, 0x49}, "CALL M,$49C0", 3, FlowCondCall},
		{[]byte{0xC8}, "RET Z", 1, FlowCondRet},
		{[]byte{0xC9}, "RET", 1, FlowRet},
		{[]byte{0xF7}, "RST $30", 1, FlowRst},
		{[]byte{0x76}, "HALT", 1, FlowHalt},
		{[]byte{0x08}, "EX AF,AF'", 1, FlowNext},
		{[]byte{0xD3, 0x99}, "OUT ($99),A", 2, FlowNext},
		{[]byte{0x27}, "DAA", 1, FlowNext},
		{[]byte{0xFE, 0x03}, "CP $03", 2, FlowNext},
		{[]byte{0x96}, "SUB (HL)", 1, FlowNext},
		{[]byte{0xED, 0x70}, "IN F,(C)", 2, FlowNext},
		{[]byte{0xED, 0x71}, "OUT (C),0", 2, FlowNext},
	}
	for _, c := range cases {
		in := Decode(reader(c.bytes), 0x4000)
		if got := in.Plain(); got != c.text || in.Len != c.len || in.Flow != c.flow {
			t.Errorf("% X: got %q len %d flow %v, want %q len %d flow %v",
				c.bytes, got, in.Len, in.Flow, c.text, c.len, c.flow)
		}
	}
}

// Every opcode sequence decodes to 1-4 bytes without panicking.
func TestDecodeAllLengths(t *testing.T) {
	for _, pre := range [][]byte{nil, {0xCB}, {0xED}, {0xDD}, {0xFD}, {0xDD, 0xCB, 0x00}, {0xFD, 0xCB, 0x00}} {
		for op := 0; op < 256; op++ {
			b := append(append([]byte{}, pre...), byte(op), 0x12, 0x34, 0x56)
			in := Decode(reader(b), 0x4000)
			if in.Len < 1 || in.Len > 4 {
				t.Errorf("% X: len %d", b[:len(pre)+1], in.Len)
			}
		}
	}
}

func TestFormat(t *testing.T) {
	in := Decode(reader([]byte{0xCD, 0x38, 0x01}), 0x4000)
	got := in.Format(func(k byte, a uint16) string {
		if k == 'T' && a == 0x0138 {
			return "RSLREG"
		}
		return "?"
	})
	if got != "CALL RSLREG" || in.Op != "CALL" || in.Dst != "{T:0138}" {
		t.Errorf("got %q op %q dst %q", got, in.Op, in.Dst)
	}
}
