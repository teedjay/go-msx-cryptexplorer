package assets

import (
	"os"
	"strings"
	"testing"
)

func TestROMBytePanicsOutsideManifest(t *testing.T) {
	defer func() {
		p := recover()
		if p == nil || !strings.Contains(p.(string), "$4000") {
			t.Errorf("want a panic naming $4000, got %v", p)
		}
	}()
	ROMByte(0x4000) // the header is not a Raw entry
}

func TestROMByteServesRawEntries(t *testing.T) {
	if _, err := os.Stat("raw/vdpregs.bin"); err != nil {
		t.Skip("assets not extracted; run go run ./cmd/extract")
	}
	if got := ROMByte(0x45C0); got != Bytes("vdpregs")[0] {
		t.Errorf("ROMByte($45C0) = %02X", got)
	}
}
