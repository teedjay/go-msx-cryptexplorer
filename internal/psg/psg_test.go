package psg

import (
	"math"
	"testing"
)

// A tone of period 254 is 1789772.5/(16*254) = 440.4 Hz: count the
// rising edges over one second.
func TestTonePitch(t *testing.T) {
	c := New()
	var r [16]byte
	r[0], r[1] = 254, 0
	r[7] = 0xBE // tone A on, all else off
	r[8] = 15
	c.SetRegs(r, false)
	s := c.Render(nil, 44100, 44100)
	edges := 0
	for i := 1; i < len(s); i++ {
		if s[i-1] < 0 && s[i] >= 0 {
			edges++
		}
	}
	want := Clock / (16 * 254)
	if math.Abs(float64(edges)-want) > 3 {
		t.Errorf("%d edges, want about %.0f", edges, want)
	}
}

func TestSilence(t *testing.T) {
	c := New()
	var r [16]byte
	r[7] = 0xBF
	c.SetRegs(r, false)
	for _, v := range c.Render(nil, 1000, 44100) {
		if v != 0 {
			t.Fatal("output with all channels off")
		}
	}
	// Tone and noise off with a volume: a constant level, which the DC
	// blocker turns to silence.
	r[8] = 15
	c.SetRegs(r, false)
	s := c.Render(nil, 44100, 44100)
	if v := s[len(s)-1]; v > 0.001 || v < -0.001 {
		t.Errorf("constant level not blocked: %f", v)
	}
}
