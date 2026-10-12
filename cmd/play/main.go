// Command play runs the game in a window with sound.
//
//	go run ./cmd/play [-scale 3] [-mute] [-record session.keys]
//	go run ./cmd/play -press 300:SPACE,304:-SPACE -shot out.png -shotframe 900
//
// Keys: arrows move (up and down on stairs), SPACE jumps, throws the
// dagger or digs with the pick, F1 pauses, F2 gives up the life, Esc
// quits. The pyramid-select cheat (internal/session/cheat.go): on the
// title screen (or the logo or demo), A starts a game at pyramid 1, B at
// pyramid 2, and so on to O at pyramid 15.
//
// -record writes the session as a key schedule (tick:row:mask:down|up)
// that scripts/refdump.sh can replay into the original, so a session can
// be compared with it by cmd/refcompare. -press scripts keys by frame
// (KEY presses, -KEY releases); with -shot the run is headless: it saves
// frame -shotframe and quits.
package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"image/png"
	"os"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"go-msx-cryptexplorer/internal/session"
	"go-msx-cryptexplorer/internal/wav"
)

const title = "go-msx-cryptexplorer"

var hostKeys = map[ebiten.Key]string{
	ebiten.KeySpace: "SPACE", ebiten.KeyArrowLeft: "LEFT", ebiten.KeyArrowUp: "UP",
	ebiten.KeyArrowDown: "DOWN", ebiten.KeyArrowRight: "RIGHT",
	ebiten.KeyF1: "F1", ebiten.KeyF2: "F2",
}

// pyramidKeys start a game at pyramid 1-15 (the cheat): A-O.
var pyramidKeys = []ebiten.Key{
	ebiten.KeyA, ebiten.KeyB, ebiten.KeyC, ebiten.KeyD, ebiten.KeyE,
	ebiten.KeyF, ebiten.KeyG, ebiten.KeyH, ebiten.KeyI, ebiten.KeyJ,
	ebiten.KeyK, ebiten.KeyL, ebiten.KeyM, ebiten.KeyN, ebiten.KeyO,
}

// game is the Ebitengine side.
type game struct {
	s     *session.Session
	press session.Script
	held  [11]byte
	scale int
	out   *ring
	img   *ebiten.Image
	quit  int
}

func (g *game) Update() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) || g.quit > 0 && g.s.Frame >= g.quit {
		return ebiten.Termination
	}
	g.press.Apply(g.s.Frame, &g.held)
	want := g.held
	for k, name := range hostKeys {
		if ebiten.IsKeyPressed(k) {
			mk := session.Named[name]
			want[mk.Row()] &^= mk.Mask()
		}
	}
	for i, k := range pyramidKeys {
		if inpututil.IsKeyJustPressed(k) {
			g.s.StartAt(i + 1)
		}
	}
	g.s.Step(want)
	if g.out != nil {
		g.out.write(g.s.Samples)
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	g.img.WritePixels(g.s.Draw().Pix)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(g.scale), float64(g.scale))
	screen.DrawImage(g.img, op)
}

func (g *game) Layout(int, int) (int, int) {
	return (256 + 2*session.Border) * g.scale, (192 + 2*session.Border) * g.scale
}

// ring passes rendered samples to the audio player. The player reads
// ahead into its own buffer (50ms), on its own goroutine but holding a
// lock the game loop also needs, so Read must not block. It returns what
// is ready, and only when nothing is a sliver of silence. The first
// version filled every short read with silence: the player's buffer
// filled up with it, and two thirds of the music was dropped as stale
// (measured over 12s of attract).
type ring struct {
	mu     sync.Mutex
	buf    []float32
	tap    []float32 // with -tap: everything Read handed the player
	taping bool
}

// starve is the silence returned when no samples are ready: short, so a
// late frame costs about a millisecond rather than the player's buffer.
const starve = 64

func (r *ring) write(s []float32) {
	r.mu.Lock()
	r.buf = append(r.buf, s...)
	// If the game runs ahead of the audio clock, catch up rather than lag:
	// never more than 200ms behind.
	if len(r.buf) > session.SampleRate/5 {
		r.buf = r.buf[len(r.buf)-session.SampleRate/10:]
	}
	r.mu.Unlock()
}

func (r *ring) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := min(len(p)/4, len(r.buf)) // 16-bit stereo
	if n == 0 {
		n = min(len(p)/4, starve)
		clear(p[:n*4])
		if r.taping {
			r.tap = append(r.tap, make([]float32, n)...)
		}
		return n * 4, nil
	}
	if r.taping {
		r.tap = append(r.tap, r.buf[:n]...)
	}
	for i, v := range r.buf[:n] {
		x := int16(v * 0.8 * 32767)
		binary.LittleEndian.PutUint16(p[i*4:], uint16(x))
		binary.LittleEndian.PutUint16(p[i*4+2:], uint16(x))
	}
	r.buf = r.buf[n:]
	return n * 4, nil
}

func main() {
	scale := flag.Int("scale", 3, "window scale")
	mute := flag.Bool("mute", false, "no sound")
	recPath := flag.String("record", "", "write the session's key schedule here")
	press := flag.String("press", "", "scripted keys: FRAME:KEY or FRAME:-KEY, comma separated")
	shot := flag.String("shot", "", "headless: save frame -shotframe to this PNG and quit")
	shotFrame := flag.Int("shotframe", 600, "frame for -shot")
	tap := flag.String("tap", "", "write what the window hands the audio player to this WAV (checks the hand-off)")
	quit := flag.Int("quitframe", 0, "quit after this many frames (0: never)")
	flag.Parse()
	sc, err := session.ParsePress(*press)
	if err != nil {
		fail(err)
	}
	s := session.New()
	if *recPath != "" {
		f, err := os.Create(*recPath)
		if err != nil {
			fail(err)
		}
		defer f.Close()
		s.Record = bufio.NewWriter(f)
		defer s.Record.Flush()
	}
	g := &game{s: s, press: sc, scale: *scale, held: session.Released(), quit: *quit}

	if *shot != "" {
		for s.Frame <= *shotFrame {
			sc.Apply(s.Frame, &g.held)
			s.Step(g.held)
		}
		f, err := os.Create(*shot)
		if err != nil {
			fail(err)
		}
		if err := png.Encode(f, s.Draw()); err != nil {
			fail(err)
		}
		f.Close()
		return
	}

	if !*mute {
		ctx := audio.NewContext(session.SampleRate)
		g.out = &ring{taping: *tap != ""}
		p, err := ctx.NewPlayer(g.out)
		if err != nil {
			fail(err)
		}
		p.SetBufferSize(50 * time.Millisecond)
		p.Play()
	}
	w, h := 256+2*session.Border, 192+2*session.Border
	g.img = ebiten.NewImage(w, h)
	ebiten.SetWindowTitle(title)
	ebiten.SetWindowSize(w**scale, h**scale)
	ebiten.SetTPS(60)
	if err := ebiten.RunGame(g); err != nil && err != ebiten.Termination {
		fail(err)
	}
	if *tap != "" && g.out != nil {
		g.out.mu.Lock()
		err := wav.Write(*tap, session.SampleRate, g.out.tap)
		g.out.mu.Unlock()
		if err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "play:", err)
	os.Exit(1)
}
