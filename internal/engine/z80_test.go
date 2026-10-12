package engine

import "testing"

// DAA against values worked out from the Z80 documentation.
func TestDAA(t *testing.T) {
	cases := []struct {
		a, n   byte
		sub    bool
		want   byte
		wantCY bool
	}{
		{0x09, 0x01, false, 0x10, false},
		{0x99, 0x01, false, 0x00, true},
		{0x45, 0x38, false, 0x83, false},
		{0x50, 0x60, false, 0x10, true},
		{0x10, 0x01, true, 0x09, false},
		{0x00, 0x01, true, 0x99, true},
		{0x99, 0x10, false, 0x09, true},
		{0x15, 0x15, false, 0x30, false},
	}
	for _, c := range cases {
		var r byte
		var f alu
		if c.sub {
			r, f = sub8(c.a, c.n, false)
		} else {
			r, f = add8(c.a, c.n, false)
		}
		r, f = daa(r, f)
		if r != c.want || f.C != c.wantCY {
			t.Errorf("%02X %v %02X: got %02X cy %v, want %02X cy %v", c.a, map[bool]string{true: "-", false: "+"}[c.sub], c.n, r, f.C, c.want, c.wantCY)
		}
	}
}
