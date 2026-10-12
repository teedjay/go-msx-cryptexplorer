package engine

// Exact models of the Z80 arithmetic whose flags the game's code depends
// on. Byte arithmetic elsewhere is plain Go byte arithmetic, which wraps
// like the Z80.

// alu holds the flags the game tests after arithmetic.
type alu struct {
	C, H, N, Z, S bool
}

// add8 is ADD A,b / ADC A,b (with carry in).
func add8(a, b byte, cin bool) (byte, alu) {
	ci := 0
	if cin {
		ci = 1
	}
	r := int(a) + int(b) + ci
	f := alu{
		C: r > 0xFF,
		H: int(a&0x0F)+int(b&0x0F)+ci > 0x0F,
		Z: byte(r) == 0,
		S: byte(r)&0x80 != 0,
	}
	return byte(r), f
}

// sub8 is SUB b / SBC A,b / CP b (with borrow in).
func sub8(a, b byte, bin bool) (byte, alu) {
	bi := 0
	if bin {
		bi = 1
	}
	r := int(a) - int(b) - bi
	f := alu{
		C: r < 0,
		H: int(a&0x0F)-int(b&0x0F)-bi < 0,
		N: true,
		Z: byte(r) == 0,
		S: byte(r)&0x80 != 0,
	}
	return byte(r), f
}

// daa is DAA after an add (f.N false) or subtract (f.N true), from the
// accumulator and the H and C flags, valid for any operands.
func daa(a byte, f alu) (byte, alu) {
	corr := byte(0)
	carry := f.C
	if f.H || a&0x0F > 9 {
		corr |= 0x06
	}
	if f.C || a > 0x99 {
		corr |= 0x60
		carry = true
	}
	var r byte
	var h bool
	if f.N {
		r = a - corr
		h = f.H && a&0x0F < 6
	} else {
		r = a + corr
		h = a&0x0F > 9
	}
	return r, alu{C: carry, H: h, N: f.N, Z: r == 0, S: r&0x80 != 0}
}

// bcdAdd is LD A,(x) / ADD|ADC A,n / DAA.
func bcdAdd(a, n byte, cin bool) (byte, bool) {
	r, f := add8(a, n, cin)
	r, f = daa(r, f)
	return r, f.C
}
