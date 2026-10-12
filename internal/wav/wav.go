// Package wav reads and writes mono 16-bit PCM WAV files.
package wav

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// Write writes samples (-1..1) as 16-bit mono PCM.
func Write(path string, rate int, s []float32) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	n := uint32(len(s) * 2)
	hdr := []any{
		[4]byte{'R', 'I', 'F', 'F'}, 36 + n, [4]byte{'W', 'A', 'V', 'E'},
		[4]byte{'f', 'm', 't', ' '}, uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate * 2), uint16(2), uint16(16),
		[4]byte{'d', 'a', 't', 'a'}, n,
	}
	for _, h := range hdr {
		if err := binary.Write(f, binary.LittleEndian, h); err != nil {
			return err
		}
	}
	buf := make([]int16, len(s))
	for i, v := range s {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		buf[i] = int16(v * 32767)
	}
	return binary.Write(f, binary.LittleEndian, buf)
}

// Read reads a PCM WAV (8 or 16 bit, any channel count), mixing channels
// to mono, and returns the rate and samples.
func Read(path string) (int, []float32, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, err
	}
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return 0, nil, errors.New("not a WAV file")
	}
	var rate, ch, bits int
	p := 12
	for p+8 <= len(b) {
		id := string(b[p : p+4])
		sz := int(binary.LittleEndian.Uint32(b[p+4:]))
		body := b[p+8:]
		if sz > len(body) {
			sz = len(body)
		}
		switch id {
		case "fmt ":
			ch = int(binary.LittleEndian.Uint16(body[2:]))
			rate = int(binary.LittleEndian.Uint32(body[4:]))
			bits = int(binary.LittleEndian.Uint16(body[14:]))
		case "data":
			if ch == 0 {
				return 0, nil, errors.New("data before fmt")
			}
			step := ch * bits / 8
			out := make([]float32, 0, sz/step)
			for i := 0; i+step <= sz; i += step {
				var sum float32
				for c := 0; c < ch; c++ {
					if bits == 16 {
						sum += float32(int16(binary.LittleEndian.Uint16(body[i+c*2:]))) / 32768
					} else {
						sum += (float32(body[i+c]) - 128) / 128
					}
				}
				out = append(out, sum/float32(ch))
			}
			return rate, out, nil
		}
		p += 8 + sz + sz&1
	}
	return 0, nil, io.ErrUnexpectedEOF
}
