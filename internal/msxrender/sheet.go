package msxrender

import (
	"image"
	"image/color"
)

// TileSheet draws 8x8 tiles from pattern bytes (8 per tile), cols tiles
// per row with a 1-pixel gap. colours, if non-nil, holds one Graphic 2
// colour byte per pattern row (fg high nibble, bg low; 0 shows backdrop);
// otherwise tiles are white on backdrop.
func TileSheet(patterns, colours []byte, cols int, backdrop byte) *image.RGBA {
	n := (len(patterns) + 7) / 8
	rows := (n + cols - 1) / cols
	img := image.NewRGBA(image.Rect(0, 0, cols*9, rows*9))
	fill(img, color.RGBA{0x30, 0x30, 0x30, 255})
	for t := 0; t < n; t++ {
		ox, oy := (t%cols)*9, (t/cols)*9
		for py := 0; py < 8; py++ {
			i := t*8 + py
			if i >= len(patterns) {
				break
			}
			p := patterns[i]
			c := byte(0xF0)
			if colours != nil && i < len(colours) {
				c = colours[i]
			}
			for px := 0; px < 8; px++ {
				ci := c & 0x0F
				if p&(0x80>>px) != 0 {
					ci = c >> 4
				}
				if ci == 0 {
					ci = backdrop
				}
				img.SetRGBA(ox+px, oy+py, Palette[ci])
			}
		}
	}
	return img
}

// SpriteSheet draws 16x16 hardware sprite patterns (32 bytes each, in the
// quadrant order TL, BL, TR, BR), cols per row, white on dark grey.
func SpriteSheet(patterns []byte, cols int) *image.RGBA {
	n := (len(patterns) + 31) / 32
	rows := (n + cols - 1) / cols
	img := image.NewRGBA(image.Rect(0, 0, cols*17, rows*17))
	fill(img, color.RGBA{0x30, 0x30, 0x30, 255})
	for s := 0; s < n; s++ {
		ox, oy := (s%cols)*17, (s/cols)*17
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				i := s*32 + (x/8)*16 + y
				if i < len(patterns) && patterns[i]&(0x80>>(x%8)) != 0 {
					img.SetRGBA(ox+x, oy+y, Palette[15])
				}
			}
		}
	}
	return img
}

func fill(img *image.RGBA, c color.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// Scale returns img enlarged by an integer factor.
func Scale(img *image.RGBA, k int) *image.RGBA {
	if k <= 1 {
		return img
	}
	b := img.Bounds()
	big := image.NewRGBA(image.Rect(0, 0, b.Dx()*k, b.Dy()*k))
	for y := 0; y < b.Dy()*k; y++ {
		for x := 0; x < b.Dx()*k; x++ {
			big.SetRGBA(x, y, img.RGBAAt(b.Min.X+x/k, b.Min.Y+y/k))
		}
	}
	return big
}
