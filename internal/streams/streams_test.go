package streams

import (
	"bytes"
	"testing"
)

func rd(b []byte) Reader { return func(a int) byte { return b[a] } }

func TestRLE(t *testing.T) {
	// addr $3800; repeat 3 x $AA; literal 2: $01 $02; new addr $3900;
	// repeat 1 x $55; end.
	s := []byte{0x00, 0x38, 0x03, 0xAA, 0x82, 0x01, 0x02, 0x80, 0x00, 0x39, 0x01, 0x55, 0x00, 0xEE}
	segs, n, err := RLE(rd(s), 0, true, false)
	if err != nil || n != 13 {
		t.Fatalf("n %d err %v", n, err)
	}
	if len(segs) != 2 || segs[0].VRAM != 0x3800 || !bytes.Equal(segs[0].Data, []byte{0xAA, 0xAA, 0xAA, 0x01, 0x02}) ||
		segs[1].VRAM != 0x3900 || !bytes.Equal(segs[1].Data, []byte{0x55}) {
		t.Errorf("segs %+v", segs)
	}
}

func TestRLEMirror(t *testing.T) {
	// No address; mirrored repeat and literal, then $80 switches it off.
	s := []byte{0x02, 0x01, 0x81, 0xC0, 0x80, 0x00, 0x20, 0x81, 0x01, 0x00}
	segs, n, err := RLE(rd(s), 0, false, true)
	if err != nil || n != len(s) {
		t.Fatalf("n %d err %v", n, err)
	}
	if segs[0].VRAM != -1 || !bytes.Equal(segs[0].Data, []byte{0x80, 0x80, 0x03}) || !bytes.Equal(segs[1].Data, []byte{0x01}) {
		t.Errorf("segs %+v", segs)
	}
}

func TestText(t *testing.T) {
	s := []byte{0x4A, 0x39, 'H', 'I', 0xFE, 0x00, 0x3A, '!', 0xFF}
	segs, n, err := Text(rd(s), 0, true)
	if err != nil || n != len(s) || len(segs) != 2 || string(segs[0].Data) != "HI" || segs[1].VRAM != 0x3A00 {
		t.Errorf("segs %+v n %d err %v", segs, n, err)
	}
}

func TestRLERAM(t *testing.T) {
	s := []byte{0x02, 0x07, 0x80}
	for i := 0; i < 256; i++ {
		s = append(s, byte(i))
	}
	s = append(s, 0x00)
	out, n, err := RLERAM(rd(s), 0)
	if err != nil || n != len(s) || len(out) != 258 || out[0] != 7 || out[2] != 0 || out[257] != 255 {
		t.Errorf("len %d n %d err %v", len(out), n, err)
	}
}
