// Package msxrender composites TMS9918A VRAM into an image. It is the one
// renderer: the game, cmd/screenshot and cmd/vramview all draw through it,
// so a still cannot show something the running game would not.
package msxrender

import (
	"image"
	"image/color"
)

const (
	Width  = 256
	Height = 192
)

// Palette is the TMS9918A palette (index 0 is transparent and never drawn
// directly: it shows the backdrop).
var Palette = [16]color.RGBA{
	{0, 0, 0, 255}, {0, 0, 0, 255}, {33, 200, 66, 255}, {94, 220, 120, 255},
	{84, 85, 237, 255}, {125, 118, 252, 255}, {212, 82, 77, 255}, {66, 235, 245, 255},
	{252, 85, 84, 255}, {255, 121, 120, 255}, {212, 193, 84, 255}, {230, 206, 128, 255},
	{33, 176, 59, 255}, {201, 91, 186, 255}, {204, 204, 204, 255}, {255, 255, 255, 255},
}

// VDP holds what the compositor needs: VRAM and registers R0-R7.
type VDP struct {
	VRAM *[0x4000]byte
	Regs [8]byte
}

// Mode returns the screen mode from M1-M3: 0 Graphic 1, 1 text, 2
// Graphic 2, 3 multicolour; anything else returns -1.
func (v *VDP) Mode() int {
	m1 := v.Regs[1]>>4&1 == 1
	m2 := v.Regs[1]>>3&1 == 1
	m3 := v.Regs[0]>>1&1 == 1
	switch {
	case !m1 && !m2 && !m3:
		return 0
	case m1 && !m2 && !m3:
		return 1
	case !m1 && !m2 && m3:
		return 2
	case !m1 && m2 && !m3:
		return 3
	}
	return -1
}

// Backdrop is the R7 low nibble.
func (v *VDP) Backdrop() byte { return v.Regs[7] & 0x0F }

// Render composites the 256x192 display into img (allocated if nil).
func (v *VDP) Render(img *image.RGBA) *image.RGBA {
	if img == nil {
		img = image.NewRGBA(image.Rect(0, 0, Width, Height))
	}
	var idx [Height][Width]byte
	if v.Regs[1]&0x40 != 0 { // display enabled
		v.background(&idx)
		v.sprites(&idx)
	}
	bd := v.Backdrop()
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			c := idx[y][x]
			if c == 0 {
				c = bd
			}
			img.SetRGBA(x, y, Palette[c])
		}
	}
	return img
}

func (v *VDP) background(idx *[Height][Width]byte) {
	vr := v.VRAM
	name := int(v.Regs[2]&0x0F) << 10
	switch v.Mode() {
	case 2:
		// Graphic 2: R3/R4 select the tables and act as masks on the
		// character number (the "hybrid" modes fall out of this).
		colBase := int(v.Regs[3]&0x80) << 6
		colMask := int(v.Regs[3]&0x7F)<<6 | 0x3F
		patBase := int(v.Regs[4]&0x04) << 11
		patMask := int(v.Regs[4]&0x03)<<11 | 0x7FF
		for row := 0; row < 24; row++ {
			for col := 0; col < 32; col++ {
				ch := int(vr[name+row*32+col]) | (row/8)<<8
				for py := 0; py < 8; py++ {
					off := ch<<3 | py
					p := vr[patBase|off&patMask]
					c := vr[colBase|off&colMask]
					for px := 0; px < 8; px++ {
						ci := c & 0x0F
						if p&(0x80>>px) != 0 {
							ci = c >> 4
						}
						idx[row*8+py][col*8+px] = ci
					}
				}
			}
		}
	case 0:
		colBase := int(v.Regs[3]) << 6
		patBase := int(v.Regs[4]&0x07) << 11
		for row := 0; row < 24; row++ {
			for col := 0; col < 32; col++ {
				ch := int(vr[name+row*32+col])
				c := vr[colBase+ch>>3]
				for py := 0; py < 8; py++ {
					p := vr[patBase+ch*8+py]
					for px := 0; px < 8; px++ {
						ci := c & 0x0F
						if p&(0x80>>px) != 0 {
							ci = c >> 4
						}
						idx[row*8+py][col*8+px] = ci
					}
				}
			}
		}
	case 1:
		// Text mode: 40x24 of 6x8, with an 8-pixel border each side.
		patBase := int(v.Regs[4]&0x07) << 11
		fg, bg := v.Regs[7]>>4, v.Regs[7]&0x0F
		for y := 0; y < Height; y++ {
			for x := 0; x < Width; x++ {
				idx[y][x] = bg
			}
		}
		for row := 0; row < 24; row++ {
			for col := 0; col < 40; col++ {
				ch := int(vr[name+row*40+col])
				for py := 0; py < 8; py++ {
					p := vr[patBase+ch*8+py]
					for px := 0; px < 6; px++ {
						if p&(0x80>>px) != 0 {
							idx[row*8+py][8+col*6+px] = fg
						}
					}
				}
			}
		}
	}
}

// sprites draws the 32 hardware sprites: Y=$D0 ends the list, the
// displayed row is Y+1, at most four per line (lower numbers win), bit 7
// of the colour byte (early clock) shifts left by 32, colour 0 is not drawn.
func (v *VDP) sprites(idx *[Height][Width]byte) {
	if v.Mode() == 1 {
		return
	}
	vr := v.VRAM
	sat := int(v.Regs[5]&0x7F) << 7
	spt := int(v.Regs[6]&0x07) << 11
	big := v.Regs[1]&0x02 != 0
	mag := 1
	if v.Regs[1]&0x01 != 0 {
		mag = 2
	}
	size := 8
	if big {
		size = 16
	}
	var perLine [Height]int
	var drawn [Height][Width]bool
	for s := 0; s < 32; s++ {
		a := sat + s*4
		y := int(vr[a])
		if y == 0xD0 {
			break
		}
		x := int(vr[a+1])
		pat := int(vr[a+2])
		col := vr[a+3]
		if big {
			pat &^= 3
		}
		if col&0x80 != 0 {
			x -= 32
		}
		c := col & 0x0F
		top := y + 1
		if y >= 0xE0 { // wraps: rows above the screen
			top = y - 255
		}
		for row := 0; row < size*mag; row++ {
			sy := top + row
			if sy < 0 || sy >= Height {
				continue
			}
			if perLine[sy] >= 4 {
				continue
			}
			perLine[sy]++
			if c == 0 {
				continue
			}
			r := row / mag
			for px := 0; px < size*mag; px++ {
				sx := x + px
				if sx < 0 || sx >= Width || drawn[sy][sx] {
					continue
				}
				pc := px / mag
				// 16x16: quadrants TL, BL, TR, BR in 8-byte units.
				b := vr[spt+pat*8+(pc/8)*16+r]
				if b&(0x80>>(pc%8)) != 0 {
					idx[sy][sx] = c
					drawn[sy][sx] = true
				}
			}
		}
	}
}
