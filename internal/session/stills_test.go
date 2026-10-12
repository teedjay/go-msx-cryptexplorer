package session

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestStills regenerates the committed screenshots and compares them byte
// for byte. Run it after any change not meant to alter what is drawn;
// when a change is meant to, regenerate with go run ./cmd/screenshot.
func TestStills(t *testing.T) {
	if _, err := os.Stat("../../assets/raw/vdpregs.bin"); err != nil {
		t.Skip("assets not extracted; run go run ./cmd/extract")
	}
	err := RunStills(func(name string, img *image.RGBA) {
		var got bytes.Buffer
		if err := png.Encode(&got, img); err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("../../screenshots", name+".png"))
		if err != nil {
			t.Fatalf("%s: %v (regenerate with go run ./cmd/screenshot)", name, err)
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Errorf("%s differs from screenshots/%s.png", name, name)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}
