// Package session runs the game for the frontends: the paced machine,
// the key matrix (with recording), the frame's audio and the composited
// display with its Border. The window (cmd/play) and the headless tools
// (cmd/screenshot, -shot) share it, so a still shows exactly what the
// window would.
package session

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"go-msx-cryptexplorer/internal/engine"
	"go-msx-cryptexplorer/internal/msxrender"
	"go-msx-cryptexplorer/internal/psg"
)

const (
	// SampleRate is the audio rate.
	SampleRate = 44100
	// FrameRate is the MSX's NTSC frame rate: each frame renders
	// SampleRate/FrameRate samples (735 or 736).
	FrameRate = 59.922743
	// Border is the backdrop border around the 256x192 display.
	Border = 8
)

// MatrixKey is a key on the MSX keyboard matrix.
type MatrixKey struct {
	row  int
	mask byte
}

// Named are the keys scripts and recordings can use.
var Named = map[string]MatrixKey{
	"SPACE": {8, 0x01}, "LEFT": {8, 0x10}, "UP": {8, 0x20}, "DOWN": {8, 0x40}, "RIGHT": {8, 0x80},
	"SELECT": {7, 0x40}, "F1": {6, 0x20}, "F2": {6, 0x40},
}

// Session is the running game, shared by the window and headless runs.
type Session struct {
	M       *engine.Machine
	chip    *psg.Chip
	Frame   int
	Ticks   int
	applied [11]byte // key matrix as last given to the game
	Record  *bufio.Writer
	Samples []float32
	screen  *image.RGBA
	frac    float64 // sample remainder carried between frames

	startPyramid int // cheat.go: pyramid to start at, 0 when none pending
}

// New powers the machine on.
func New() *Session {
	s := &Session{M: engine.NewMachine(), chip: psg.New(), screen: image.NewRGBA(image.Rect(0, 0, 256+2*Border, 192+2*Border))}
	for i := range s.applied {
		s.applied[i] = 0xFF
	}
	return s
}

// Step runs one frame with the key matrix want, logging changes at the
// tick they reach the game, and renders that frame's audio.
func (s *Session) Step(want [11]byte) {
	s.pyramidCheat(&want) // cheat.go; does nothing unless StartAt was called
	if s.M.WillTick() {
		for row := range want {
			if d := want[row] ^ s.applied[row]; d != 0 {
				if s.Record != nil {
					if down := s.applied[row] & d; down != 0 {
						fmt.Fprintf(s.Record, "%d:%d:%d:down\n", s.Ticks, row, down)
					}
					if up := want[row] & d; up != 0 {
						fmt.Fprintf(s.Record, "%d:%d:%d:up\n", s.Ticks, row, up)
					}
				}
				s.applied[row] = want[row]
			}
		}
		s.M.Keys = s.applied
	}
	if s.M.Frame() {
		s.Ticks++
	}
	s.Frame++
	s.chip.SetRegs(s.M.PSG, false)
	s.frac += SampleRate / FrameRate
	n := int(s.frac)
	s.frac -= float64(n)
	s.Samples = s.chip.Render(s.Samples[:0], n, SampleRate)
}

// Draw composites the display with its backdrop border.
func (s *Session) Draw() *image.RGBA {
	v := &msxrender.VDP{VRAM: &s.M.VDP.VRAM, Regs: s.M.VDP.Regs}
	bd := msxrender.Palette[v.Backdrop()]
	if v.Backdrop() == 0 {
		bd = color.RGBA{0, 0, 0, 255}
	}
	for i := 0; i < len(s.screen.Pix); i += 4 {
		s.screen.Pix[i], s.screen.Pix[i+1], s.screen.Pix[i+2], s.screen.Pix[i+3] = bd.R, bd.G, bd.B, 255
	}
	img := v.Render(nil)
	for y := 0; y < 192; y++ {
		copy(s.screen.Pix[(y+Border)*s.screen.Stride+Border*4:], img.Pix[y*img.Stride:y*img.Stride+256*4])
	}
	return s.screen
}

// Script holds scripted keys: frame -> changes.
type Script map[int][]Change

// Change is one scripted key press or release.
type Change struct {
	Key  MatrixKey
	Down bool
}

// ParsePress parses "FRAME:KEY" (press) and "FRAME:-KEY" (release)
// items, comma separated.
func ParsePress(s string) (Script, error) {
	sc := Script{}
	if s == "" {
		return sc, nil
	}
	for _, item := range strings.Split(s, ",") {
		f, k, ok := strings.Cut(item, ":")
		n, err := strconv.Atoi(f)
		if !ok || err != nil {
			return nil, fmt.Errorf("bad -press item %q", item)
		}
		down := !strings.HasPrefix(k, "-")
		mk, ok := Named[strings.TrimPrefix(k, "-")]
		if !ok {
			return nil, fmt.Errorf("unknown key %q", k)
		}
		sc[n] = append(sc[n], Change{mk, down})
	}
	return sc, nil
}

// Apply applies the script's changes for the current frame to held.
func (sc Script) Apply(frame int, held *[11]byte) {
	for _, p := range sc[frame] {
		if p.Down {
			held[p.Key.row] &^= p.Key.mask
		} else {
			held[p.Key.row] |= p.Key.mask
		}
	}
}

// Released returns a key matrix with nothing pressed.
func Released() [11]byte {
	var k [11]byte
	for i := range k {
		k[i] = 0xFF
	}
	return k
}

// Row and Mask expose a key's place in the matrix.
func (k MatrixKey) Row() int   { return k.row }
func (k MatrixKey) Mask() byte { return k.mask }

// SoundOnly advances only the sound driver by a frame and returns that
// frame's samples (for previewing one sound).
func (s *Session) SoundOnly() []float32 {
	s.M.SoundFrame()
	s.Frame++
	s.chip.SetRegs(s.M.PSG, false)
	s.frac += SampleRate / FrameRate
	n := int(s.frac)
	s.frac -= float64(n)
	s.Samples = s.chip.Render(s.Samples[:0], n, SampleRate)
	return s.Samples
}
