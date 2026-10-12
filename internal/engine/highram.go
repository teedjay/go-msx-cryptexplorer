package engine

// The RAM above the game's, $F000-$F37F, as the reference machine
// (openMSX, C-BIOS_MSX1_JP) has it. The game never writes it except
// $F000 (the map fill at $6D9A runs one byte long), but the map probes
// read it: (Y + offset) >> 3 can reach rows 24-31 when an object is near
// the top or bottom of the screen, and row 31 of the third screen is at
// $F31F. Measured from a reference dump (tick 2000; identical at ticks
// 10 and 9000 apart from $F000): the power-on pattern ($F0 at each
// 256-byte page start, $FF elsewhere) up to $F2BF, C-BIOS work data at
// $F2C0-$F2FF, and $C9 from $F300. This is C-BIOS's, not the game's;
// another BIOS would leave other values there.

// HighBase is the first address of the modelled high RAM.
const HighBase = 0xF000

// HighSize covers $F000-$F37F.
const HighSize = 0x380

// cbiosWork is $F2C0-$F2FF under C-BIOS.
var cbiosWork = [0x40]byte{
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x03, 0x19, 0x10, 0x80,
	0x00, 0x00, 0x55, 0xF0, 0xCF, 0x00, 0xFF, 0x00, 0x67, 0x24, 0x0C, 0x59, 0x00, 0x01, 0x41, 0x42,
	0xC2, 0x10, 0xC2, 0x10, 0xDA, 0x23, 0xE2, 0x23, 0xEC, 0x23, 0xA0, 0x40, 0x92, 0xF3, 0x43, 0xF0,
	0x67, 0x24, 0xFF, 0x01, 0xC1, 0x0E, 0x04, 0x40, 0x08, 0x1F, 0x3E, 0x0E, 0xC2, 0xFC, 0xF8, 0x0D,
}

// powerOnHigh returns $F000-$F37F as the reference machine has it at
// power-on.
func powerOnHigh() [HighSize]byte {
	var h [HighSize]byte
	for i := range h {
		switch {
		case i >= 0x300:
			h[i] = 0xC9
		case i >= 0x2C0:
			h[i] = cbiosWork[i-0x2C0]
		case i&0xFF == 0:
			h[i] = 0xF0
		default:
			h[i] = 0xFF
		}
	}
	return h
}
