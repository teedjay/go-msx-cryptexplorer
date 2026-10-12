// Package streams decodes the two VRAM stream formats the ROM writes
// through, as flat segments. Values are decoded as stored: the runtime
// tile substitution controlled by $E661 (in $443B) is not applied here,
// it belongs to the ported routine.
//
// RLE (decoder $440C/$4412/$4474, High: read instruction by instruction):
//
//	[addr lo, addr hi]   VRAM address (only when the entry reads it)
//	ctrl $00             end
//	ctrl $80             a new VRAM address word follows (and, through
//	                     $440C -> $4412, mirroring is switched off)
//	ctrl $01-$7F         repeat: the next byte, ctrl times
//	ctrl $81-$FF         literal: the next ctrl&$7F bytes
//
// The two run kinds differ only in whether $443B is called once before
// the OUT loop ($4430, repeat) or inside it ($4425, literal).
//
// RLE to RAM (decoder $561B, entry $5638 with C=1, High): the same run
// kinds, written to (HL), but there is no $80 address control: $80 takes
// the literal path with B=0, so DJNZ copies 256 bytes.
//
// Text (writer $43F1/$4408/$43F3/$43F9, High):
//
//	[addr lo, addr hi]   VRAM address (only when the entry reads it)
//	$FF                  end
//	$FE                  a new VRAM address word follows
//	other                written with WRTVRM (ANDed with C: $FF writes,
//	                     $00 erases)
package streams

import "fmt"

// Reader returns the ROM byte at a CPU address.
type Reader func(addr int) byte

// Segment is a run of bytes written from one VRAM address.
type Segment struct {
	VRAM int // -1 when the stream starts without an address
	Data []byte
}

// RLE decodes an RLE stream at addr. withAddr says whether it starts with
// a VRAM address word (entry $440C) or not ($4412, $4474). mirror
// bit-reverses each byte (entry $4474) until a $80 control. It returns the
// segments and the number of stream bytes consumed.
func RLE(rd Reader, addr int, withAddr, mirror bool) ([]Segment, int, error) {
	p := addr
	var segs []Segment
	start := func() {
		v := int(rd(p)) | int(rd(p+1))<<8
		p += 2
		segs = append(segs, Segment{VRAM: v})
	}
	if withAddr {
		start()
	} else {
		segs = append(segs, Segment{VRAM: -1})
	}
	for steps := 0; ; steps++ {
		if steps > 0x10000 {
			return nil, 0, fmt.Errorf("rle $%04X: no end after %d controls", addr, steps)
		}
		ctrl := rd(p)
		p++
		cur := &segs[len(segs)-1]
		switch {
		case ctrl == 0x00:
			return segs, p - addr, nil
		case ctrl == 0x80:
			start()
			mirror = false
		case ctrl&0x80 == 0:
			v := rd(p)
			p++
			if mirror {
				v = reverse(v)
			}
			for i := 0; i < int(ctrl); i++ {
				cur.Data = append(cur.Data, v)
			}
		default:
			for i := 0; i < int(ctrl&0x7F); i++ {
				v := rd(p)
				p++
				if mirror {
					v = reverse(v)
				}
				cur.Data = append(cur.Data, v)
			}
		}
	}
}

// RLERAM decodes a RAM RLE stream at addr (see the package comment).
func RLERAM(rd Reader, addr int) ([]byte, int, error) {
	p := addr
	var out []byte
	for steps := 0; steps <= 0x10000; steps++ {
		ctrl := rd(p)
		p++
		switch {
		case ctrl == 0x00:
			return out, p - addr, nil
		case ctrl&0x80 == 0:
			v := rd(p)
			p++
			for i := 0; i < int(ctrl); i++ {
				out = append(out, v)
			}
		default:
			n := int(ctrl & 0x7F)
			if n == 0 {
				n = 256 // DJNZ from B=0
			}
			for i := 0; i < n; i++ {
				out = append(out, rd(p))
				p++
			}
		}
	}
	return nil, 0, fmt.Errorf("rle-ram $%04X: no end", addr)
}

// Text decodes a text stream at addr (see the package comment).
func Text(rd Reader, addr int, withAddr bool) ([]Segment, int, error) {
	p := addr
	var segs []Segment
	start := func() {
		v := int(rd(p)) | int(rd(p+1))<<8
		p += 2
		segs = append(segs, Segment{VRAM: v})
	}
	if withAddr {
		start()
	} else {
		segs = append(segs, Segment{VRAM: -1})
	}
	for n := 0; n < 0x4000; n++ {
		b := rd(p)
		p++
		switch b {
		case 0xFF:
			return segs, p - addr, nil
		case 0xFE:
			start()
		default:
			segs[len(segs)-1].Data = append(segs[len(segs)-1].Data, b)
		}
	}
	return nil, 0, fmt.Errorf("text $%04X: no end", addr)
}

func reverse(b byte) byte {
	var r byte
	for i := 0; i < 8; i++ {
		r = r<<1 | b&1
		b >>= 1
	}
	return r
}
