// Package z80dis decodes single Z80 instructions, including the DD/FD
// index prefixes, CB/ED pages, DDCB/FDCB and the common undocumented forms
// (IXH/IXL, SLL, the register-copy DDCB variants, IN (C)/OUT (C),0).
//
// The decoder knows nothing about labels. Addresses appear in Text as
// tokens that a formatter replaces:
//
//	{T:XXXX}  a jump or call target
//	{M:XXXX}  a memory operand, inside the parentheses of (nn)
//	{I:XXXX}  a 16-bit immediate
//
// Untouched by a formatter, Plain removes the braces and gives $XXXX.
package z80dis

import (
	"fmt"
	"regexp"
	"strings"
)

// Flow says how control leaves an instruction.
type Flow int

const (
	FlowNext         Flow = iota // falls through only
	FlowJump                     // JP nn, JR e: goes to Target only
	FlowCondJump                 // JP cc, JR cc, DJNZ: Target or fall-through
	FlowCall                     // CALL nn: Target, then fall-through
	FlowCondCall                 // CALL cc
	FlowRet                      // RET, RETI, RETN
	FlowCondRet                  // RET cc
	FlowJumpIndirect             // JP (HL), JP (IX), JP (IY)
	FlowRst                      // RST p: like CALL Target
	FlowHalt                     // HALT
)

func (f Flow) String() string {
	return [...]string{"next", "jump", "condjump", "call", "condcall", "ret", "condret", "jumpind", "rst", "halt"}[f]
}

// Ends reports whether control never falls through to the next instruction.
func (f Flow) Ends() bool {
	return f == FlowJump || f == FlowRet || f == FlowJumpIndirect || f == FlowHalt
}

// Inst is one decoded instruction.
type Inst struct {
	Addr  uint16
	Len   int
	Text  string // mnemonic and operands, with address tokens
	Flow  Flow
	Valid bool // false for undefined ED opcodes and lone prefixes (emitted as DB)

	Target    uint16 // for jumps, calls and RST
	HasTarget bool
	Mem       uint16 // for (nn) operands
	HasMem    bool
	Imm16     uint16 // for 16-bit immediates
	HasImm16  bool

	// Op is the bare mnemonic ("LD", "PUSH"). Dst/Src are the first and
	// second operand texts as written (with tokens), for pattern checks
	// such as LD rr,nn followed by PUSH rr.
	Op       string
	Dst, Src string
}

var tokenRE = regexp.MustCompile(`\{[TMI]:([0-9A-F]{4})\}`)

// Plain returns Text with every address token written as $XXXX.
func (in *Inst) Plain() string { return tokenRE.ReplaceAllString(in.Text, "$$$1") }

// Format returns Text with each token replaced by fn(kind, addr), where kind
// is 'T', 'M' or 'I'.
func (in *Inst) Format(fn func(kind byte, addr uint16) string) string {
	return tokenRE.ReplaceAllStringFunc(in.Text, func(t string) string {
		var a uint16
		fmt.Sscanf(t[3:7], "%04X", &a)
		return fn(t[1], a)
	})
}

// Reader returns the byte at an address. Decode never reads more than four
// bytes past addr.
type Reader func(addr uint16) byte

var (
	r8   = [8]string{"B", "C", "D", "E", "H", "L", "(HL)", "A"}
	rp   = [4]string{"BC", "DE", "HL", "SP"}
	rp2  = [4]string{"BC", "DE", "HL", "AF"}
	cc   = [8]string{"NZ", "Z", "NC", "C", "PO", "PE", "P", "M"}
	alu  = [8]string{"ADD A,", "ADC A,", "SUB ", "SBC A,", "AND ", "XOR ", "OR ", "CP "}
	rot  = [8]string{"RLC", "RRC", "RL", "RR", "SLA", "SRA", "SLL", "SRL"}
	x0z7 = [8]string{"RLCA", "RRCA", "RLA", "RRA", "DAA", "CPL", "SCF", "CCF"}
	imMd = [8]string{"0", "0", "1", "2", "0", "0", "1", "2"}
	edz7 = [8]string{"LD I,A", "LD R,A", "LD A,I", "LD A,R", "RRD", "RLD", "", ""}
	bli  = [4][4]string{
		{"LDI", "CPI", "INI", "OUTI"},
		{"LDD", "CPD", "IND", "OUTD"},
		{"LDIR", "CPIR", "INIR", "OTIR"},
		{"LDDR", "CPDR", "INDR", "OTDR"},
	}
)

type dec struct {
	rd   Reader
	addr uint16
	pc   uint16 // next byte to read
	idx  string // "", "IX" or "IY"
	disp int8
	in   Inst
}

func (d *dec) byte() byte {
	b := d.rd(d.pc)
	d.pc++
	return b
}

func (d *dec) word() uint16 {
	lo := uint16(d.byte())
	return lo | uint16(d.byte())<<8
}

func (d *dec) imm16() string {
	v := d.word()
	d.in.Imm16, d.in.HasImm16 = v, true
	return fmt.Sprintf("{I:%04X}", v)
}

func (d *dec) mem16() string {
	v := d.word()
	d.in.Mem, d.in.HasMem = v, true
	return fmt.Sprintf("({M:%04X})", v)
}

func (d *dec) target(v uint16) string {
	d.in.Target, d.in.HasTarget = v, true
	return fmt.Sprintf("{T:%04X}", v)
}

func hex8(b byte) string { return fmt.Sprintf("$%02X", b) }

// idxMem formats (IX+d) from the already-read displacement.
func (d *dec) idxMem() string {
	if d.disp < 0 {
		return fmt.Sprintf("(%s-$%02X)", d.idx, -int(d.disp))
	}
	return fmt.Sprintf("(%s+$%02X)", d.idx, d.disp)
}

// reg returns r[i]. With an index prefix, H/L become IXH/IXL unless
// noHalf (the other operand is (IX+d)), and (HL) becomes (IX+d).
func (d *dec) reg(i int, noHalf bool) string {
	if d.idx != "" {
		switch {
		case i == 6:
			return d.idxMem()
		case (i == 4 || i == 5) && !noHalf:
			return d.idx + [2]string{"H", "L"}[i-4]
		}
	}
	return r8[i]
}

func (d *dec) rpn(p int) string {
	if p == 2 && d.idx != "" {
		return d.idx
	}
	return rp[p]
}

func (d *dec) rp2n(p int) string {
	if p == 2 && d.idx != "" {
		return d.idx
	}
	return rp2[p]
}

func (d *dec) hl() string {
	if d.idx != "" {
		return d.idx
	}
	return "HL"
}

// Decode decodes the instruction at addr.
func Decode(rd Reader, addr uint16) Inst {
	d := &dec{rd: rd, addr: addr, pc: addr}
	d.in.Addr = addr
	d.in.Valid = true
	op := d.byte()
	if op == 0xDD || op == 0xFD {
		idx := "IX"
		if op == 0xFD {
			idx = "IY"
		}
		next := rd(d.pc)
		if next == 0xDD || next == 0xFD || next == 0xED {
			// A prefix followed by another prefix acts as a NOP.
			return d.finish("DB "+hex8(op), false)
		}
		d.idx = idx
		op = d.byte()
	}
	var text string
	switch op {
	case 0xCB:
		text = d.cb()
	case 0xED:
		text = d.ed()
		if text == "" {
			return d.finish(fmt.Sprintf("DB $ED,%s", hex8(rd(addr+1))), false)
		}
	default:
		text = d.main(op)
	}
	return d.finish(text, true)
}

func (d *dec) finish(text string, valid bool) Inst {
	if !valid {
		d.pc = d.addr + 1
		if strings.HasPrefix(text, "DB $ED") {
			d.pc = d.addr + 2
		}
		d.in.Flow = FlowNext
	}
	d.in.Text = text
	d.in.Valid = valid
	d.in.Len = int(d.pc - d.addr)
	op, rest, _ := strings.Cut(text, " ")
	d.in.Op = op
	if rest != "" {
		dst, src, _ := strings.Cut(rest, ",")
		d.in.Dst, d.in.Src = dst, src
	}
	return d.in
}

// usesMemHL reports whether an unprefixed opcode has an (HL) operand, so
// that with DD/FD the displacement byte comes straight after it.
func usesMemHL(op byte) bool {
	x, y, z := op>>6, (op>>3)&7, op&7
	switch x {
	case 0:
		return (z == 4 || z == 5 || z == 6) && y == 6
	case 1:
		return (y == 6) != (z == 6)
	case 2:
		return z == 6
	}
	return false
}

func (d *dec) main(op byte) string {
	if d.idx != "" && usesMemHL(op) {
		d.disp = int8(d.byte())
	}
	x, y, z := int(op>>6), int((op>>3)&7), int(op&7)
	p, q := y>>1, y&1
	switch x {
	case 0:
		switch z {
		case 0:
			switch y {
			case 0:
				return "NOP"
			case 1:
				return "EX AF,AF'"
			case 2, 3:
				e := int8(d.byte())
				t := d.target(uint16(int(d.pc) + int(e)))
				if y == 2 {
					d.in.Flow = FlowCondJump
					return "DJNZ " + t
				}
				d.in.Flow = FlowJump
				return "JR " + t
			default:
				e := int8(d.byte())
				d.in.Flow = FlowCondJump
				return "JR " + cc[y-4] + "," + d.target(uint16(int(d.pc)+int(e)))
			}
		case 1:
			if q == 0 {
				return "LD " + d.rpn(p) + "," + d.imm16()
			}
			return "ADD " + d.hl() + "," + d.rpn(p)
		case 2:
			switch y {
			case 0:
				return "LD (BC),A"
			case 1:
				return "LD A,(BC)"
			case 2:
				return "LD (DE),A"
			case 3:
				return "LD A,(DE)"
			case 4:
				return "LD " + d.mem16() + "," + d.hl()
			case 5:
				return "LD " + d.hl() + "," + d.mem16()
			case 6:
				return "LD " + d.mem16() + ",A"
			default:
				return "LD A," + d.mem16()
			}
		case 3:
			return [2]string{"INC ", "DEC "}[q] + d.rpn(p)
		case 4:
			return "INC " + d.reg(y, false)
		case 5:
			return "DEC " + d.reg(y, false)
		case 6:
			r := d.reg(y, false)
			return "LD " + r + "," + hex8(d.byte())
		default:
			return x0z7[y]
		}
	case 1:
		if y == 6 && z == 6 {
			d.in.Flow = FlowHalt
			return "HALT"
		}
		mem := y == 6 || z == 6
		return "LD " + d.reg(y, mem) + "," + d.reg(z, mem)
	case 2:
		return alu[y] + d.reg(z, false)
	}
	// x == 3
	switch z {
	case 0:
		d.in.Flow = FlowCondRet
		return "RET " + cc[y]
	case 1:
		if q == 0 {
			return "POP " + d.rp2n(p)
		}
		switch p {
		case 0:
			d.in.Flow = FlowRet
			return "RET"
		case 1:
			return "EXX"
		case 2:
			d.in.Flow = FlowJumpIndirect
			return "JP (" + d.hl() + ")"
		default:
			return "LD SP," + d.hl()
		}
	case 2:
		d.in.Flow = FlowCondJump
		return "JP " + cc[y] + "," + d.target(d.word())
	case 3:
		switch y {
		case 0:
			d.in.Flow = FlowJump
			return "JP " + d.target(d.word())
		case 2:
			return "OUT (" + hex8(d.byte()) + "),A"
		case 3:
			return "IN A,(" + hex8(d.byte()) + ")"
		case 4:
			return "EX (SP)," + d.hl()
		case 5:
			return "EX DE,HL"
		case 6:
			return "DI"
		default:
			return "EI"
		}
	case 4:
		d.in.Flow = FlowCondCall
		return "CALL " + cc[y] + "," + d.target(d.word())
	case 5:
		if q == 0 {
			return "PUSH " + d.rp2n(p)
		}
		d.in.Flow = FlowCall
		return "CALL " + d.target(d.word())
	case 6:
		return alu[y] + hex8(d.byte())
	default:
		d.in.Flow = FlowRst
		d.target(uint16(y * 8))
		return fmt.Sprintf("RST $%02X", y*8)
	}
}

func (d *dec) cb() string {
	if d.idx != "" {
		d.disp = int8(d.byte())
	}
	op := d.byte()
	x, y, z := int(op>>6), int((op>>3)&7), int(op&7)
	var operand string
	if d.idx != "" {
		operand = d.idxMem()
		if z != 6 && x != 1 {
			// Undocumented: the result is also copied to r[z].
			operand += "," + r8[z]
		}
	} else {
		operand = r8[z]
	}
	switch x {
	case 0:
		return rot[y] + " " + operand
	case 1:
		return fmt.Sprintf("BIT %d,%s", y, operand)
	case 2:
		return fmt.Sprintf("RES %d,%s", y, operand)
	default:
		return fmt.Sprintf("SET %d,%s", y, operand)
	}
}

func (d *dec) ed() string {
	d.idx = "" // DD ED is handled as a lone prefix before we get here
	op := d.byte()
	x, y, z := int(op>>6), int((op>>3)&7), int(op&7)
	p, q := y>>1, y&1
	if x == 1 {
		switch z {
		case 0:
			if y == 6 {
				return "IN F,(C)"
			}
			return "IN " + r8[y] + ",(C)"
		case 1:
			if y == 6 {
				return "OUT (C),0"
			}
			return "OUT (C)," + r8[y]
		case 2:
			return [2]string{"SBC HL,", "ADC HL,"}[q] + rp[p]
		case 3:
			if q == 0 {
				return "LD " + d.mem16() + "," + rp[p]
			}
			return "LD " + rp[p] + "," + d.mem16()
		case 4:
			return "NEG"
		case 5:
			d.in.Flow = FlowRet
			if y == 1 {
				return "RETI"
			}
			return "RETN"
		case 6:
			return "IM " + imMd[y]
		default:
			return edz7[y]
		}
	}
	if x == 2 && z <= 3 && y >= 4 {
		return bli[y-4][z]
	}
	return ""
}
