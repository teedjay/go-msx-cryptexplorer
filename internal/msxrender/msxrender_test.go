package msxrender

import "testing"

// The layout this ROM programs: colour $0000, patterns $2000, names $3800,
// sprite attributes $3B00, sprite patterns $1800, 16x16 sprites.
var regs = [8]byte{0x02, 0xE2, 0x0E, 0x7F, 0x07, 0x76, 0x03, 0xE4}

func TestGraphic2(t *testing.T) {
	var vram [0x4000]byte
	vram[0x3B00] = 0xD0 // no sprites
	// Character 1 in the middle third: top row left half set, fg 15 bg 0.
	vram[0x3800+8*32+0] = 1
	vram[0x2000+0x800+8] = 0xF0
	vram[0x0000+0x800+8] = 0xF0
	v := &VDP{VRAM: &vram, Regs: regs}
	img := v.Render(nil)
	if got := img.RGBAAt(0, 64); got != Palette[15] {
		t.Errorf("fg pixel: %v", got)
	}
	if got := img.RGBAAt(4, 64); got != Palette[4] { // colour 0 shows backdrop
		t.Errorf("bg pixel shows %v, want backdrop", got)
	}
	// The same character number in the top third uses the top third's
	// tables, which are empty.
	vram[0x3800] = 1
	if got := v.Render(nil).RGBAAt(0, 0); got != Palette[4] {
		t.Errorf("top third used middle third's pattern: %v", got)
	}
}

func TestSprites(t *testing.T) {
	var vram [0x4000]byte
	v := &VDP{VRAM: &vram, Regs: regs}
	// Sprite 0 at Y=9 (row 10), X=20, pattern 4 (16x16: low bits ignored
	// when given 5), colour 8; top-right quadrant row 0 = $80.
	copy(vram[0x3B00:], []byte{9, 20, 5, 8, 0xD0})
	vram[0x1800+4*8+16] = 0x80
	img := v.Render(nil)
	if got := img.RGBAAt(28, 10); got != Palette[8] {
		t.Errorf("sprite pixel at (28,10): %v", got)
	}
	if got := img.RGBAAt(28, 9); got != Palette[4] {
		t.Errorf("row above Y+1 drawn: %v", got)
	}
	// $D0 terminates: sprite 1 after the terminator is not drawn.
	copy(vram[0x3B00:], []byte{0xD0, 0, 0, 0, 9, 20, 4, 8})
	if got := v.Render(nil).RGBAAt(28, 10); got != Palette[4] {
		t.Errorf("sprite after $D0 drawn: %v", got)
	}
}

func TestFifthSpriteHidden(t *testing.T) {
	var vram [0x4000]byte
	v := &VDP{VRAM: &vram, Regs: regs}
	for s := 0; s < 5; s++ {
		copy(vram[0x3B00+s*4:], []byte{0, byte(s * 20), 0, 15})
	}
	vram[0x3B00+20] = 0xD0
	vram[0x1800] = 0xFF
	img := v.Render(nil)
	if got := img.RGBAAt(60, 1); got != Palette[15] {
		t.Errorf("4th sprite missing: %v", got)
	}
	if got := img.RGBAAt(80, 1); got != Palette[4] {
		t.Errorf("5th sprite on the line drawn: %v", got)
	}
}
