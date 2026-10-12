// Package rom loads the original cartridge image supplied by the user and
// checks that it is the dump every address in this project refers to.
package rom

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

const (
	// DefaultPath is the file name the user is asked to place in the
	// repository root.
	DefaultPath = "King's Valley 1 (1985) Konami [RC-727] [8553].rom"

	// Size is the image size in bytes. The cartridge occupies $4000-$7FFF.
	Size = 0x4000
	// Base is the CPU address of the first ROM byte.
	Base = 0x4000
	// End is one past the CPU address of the last ROM byte.
	End = Base + Size

	// SHA256 identifies the supported dump.
	SHA256 = "01bd3179540c886ab658626ca0460c329d167327fabd43eb615883375e9e9a3e"
)

// ROM is the cartridge image, addressed by CPU address.
type ROM struct {
	Data []byte // Data[0] is CPU address $4000
	// Mismatch is non-empty when the image is not the supported dump.
	Mismatch string
}

// Load reads the image at path. A wrong size is an error; a wrong checksum
// is reported in Mismatch (and by Warn) so tools can still run on it.
func Load(path string) (*ROM, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) != Size {
		return nil, fmt.Errorf("%s: %d bytes, want %d", path, len(b), Size)
	}
	r := &ROM{Data: b}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != SHA256 {
		r.Mismatch = fmt.Sprintf("%s: SHA-256 %s, want %s; addresses in the notes will not match", path, got, SHA256)
	}
	return r, nil
}

// Warn writes the checksum mismatch, if any, to w.
func (r *ROM) Warn(w io.Writer) {
	if r.Mismatch != "" {
		fmt.Fprintln(w, "warning:", r.Mismatch)
	}
}

// Contains reports whether CPU address a lies inside the cartridge.
func Contains(a int) bool { return a >= Base && a < End }

// Byte returns the ROM byte at CPU address a. It panics outside the ROM.
func (r *ROM) Byte(a int) byte {
	if !Contains(a) {
		panic(fmt.Sprintf("rom: address $%04X outside $%04X-$%04X", a, Base, End-1))
	}
	return r.Data[a-Base]
}
