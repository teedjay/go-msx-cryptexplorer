// Command screenshot plays the scripted run in internal/session headless
// and writes its stills as PNGs (default: screenshots/, the committed
// regression baseline checked by go test ./internal/session).
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"go-msx-cryptexplorer/internal/session"
)

func main() {
	out := flag.String("out", "screenshots", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	err := session.RunStills(func(name string, img *image.RGBA) {
		p := filepath.Join(*out, name+".png")
		f, err := os.Create(p)
		if err != nil {
			fail(err)
		}
		if err := png.Encode(f, img); err != nil {
			fail(err)
		}
		f.Close()
		fmt.Println("wrote", p)
	})
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "screenshot:", err)
	os.Exit(1)
}
