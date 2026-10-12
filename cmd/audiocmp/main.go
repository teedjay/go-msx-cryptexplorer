// Command audiocmp compares the port's audio with the original's,
// window by window (1/60 s): both sounding or both silent, and each
// side's strongest frequency present in the other's spectrum (presence,
// not the single peak: in a chord the loudest note depends on each
// output stage's filtering). The two recordings are aligned by their
// loudness envelopes first.
//
//	go run ./cmd/audiocmp port.wav orig.wav
package main

import (
	"fmt"
	"math"
	"math/cmplx"
	"os"
	"sort"

	"go-msx-cryptexplorer/internal/wav"
)

const (
	rate  = 44100
	win   = rate / 60
	fftN  = 2048
	quiet = 0.01 // RMS below this (of the normalised signal) is silence
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: audiocmp port.wav orig.wav")
		os.Exit(2)
	}
	a := load(os.Args[1])
	b := load(os.Args[2])
	ea, eb := envelope(a), envelope(b)
	lag := align(ea, eb, 600)
	// Refine by frequency agreement on a sample of windows around it.
	bestF := -1
	coarse := lag
	for l := coarse - 12; l <= coarse+12; l++ {
		agree := 0
		for i := 0; i < len(ea); i += 7 {
			j := i + l
			if j < 0 || j >= len(eb) || ea[i] <= quiet || eb[j] <= quiet {
				continue
			}
			if present(spectrum(a, i*win), spectrum(b, j*win)) {
				agree++
			}
		}
		if agree > bestF {
			bestF, lag = agree, l
		}
	}
	// Then within a frame, in steps of 1/16 frame: the original changes
	// its registers at its interrupt, not on our frame grid.
	off := lag * win
	bestS := -1
	for d := -win; d <= win; d += win / 16 {
		o := lag*win + d
		agree := 0
		for i := 0; i < len(ea); i += 7 {
			p := i*win + o
			if p < 0 || p+fftN >= len(b) || ea[i] <= quiet {
				continue
			}
			if present(spectrum(a, i*win), spectrum(b, p)) {
				agree++
			}
		}
		if agree > bestS {
			bestS, off = agree, o
		}
	}
	fmt.Printf("aligned: original lags the port by %d samples (%.3f s; envelope said %d windows)\n", off, float64(off)/rate, coarse)
	agreeSound, agreeFreq, both, n := 0, 0, 0, 0
	for i := 0; i < len(ea); i++ {
		j := (i*win + off) / win
		if j < 0 || j >= len(eb) || i*win+off+fftN >= len(b) {
			continue
		}
		n++
		sa, sb := ea[i] > quiet, eb[j] > quiet
		if sa == sb {
			agreeSound++
		} else if os.Getenv("AUDIOCMP_DEBUG") != "" && n%7 == 0 {
			fmt.Printf("  window %5d: port %.4f orig %.4f\n", i, ea[i], eb[j])
		}
		if !(sa && sb) {
			continue
		}
		both++
		pa := spectrum(a, i*win)
		pb := spectrum(b, i*win+off)
		if present(pa, pb) && present(pb, pa) {
			agreeFreq++
		} else if os.Getenv("AUDIOCMP_FAIL") != "" {
			fmt.Printf("F %d %.0f %.0f\n", i, peakHz(pa), peakHz(pb))
		}
	}
	if w := os.Getenv("AUDIOCMP_WIN"); w != "" {
		var i int
		fmt.Sscan(w, &i)
		for _, side := range []struct {
			name string
			m    []float64
		}{{"port", spectrum(a, i*win)}, {"orig", spectrum(b, i*win+off)}} {
			type pk struct{ f, m float64 }
			var ps []pk
			for k := 3; k < len(side.m); k++ {
				ps = append(ps, pk{float64(k) * rate / fftN, side.m[k]})
			}
			sort.Slice(ps, func(x, y int) bool { return ps[x].m > ps[y].m })
			fmt.Printf("%s:", side.name)
			for _, p := range ps[:6] {
				fmt.Printf(" %.0fHz(%.0f)", p.f, p.m)
			}
			fmt.Println()
		}
	}
	fmt.Printf("%d windows: sounding/silent agree %.1f%%; of %d windows where both sound, strongest frequencies present in both %.1f%%\n",
		n, 100*float64(agreeSound)/float64(n), both, 100*float64(agreeFreq)/float64(max(both, 1)))
}

func load(path string) []float32 {
	r, s, err := wav.Read(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, path+":", err)
		os.Exit(1)
	}
	if r != rate { // linear resample
		out := make([]float32, int(float64(len(s))*rate/float64(r)))
		for i := range out {
			x := float64(i) * float64(r) / rate
			k := int(x)
			if k+1 >= len(s) {
				break
			}
			f := float32(x - float64(k))
			out[i] = s[k]*(1-f) + s[k+1]*f
		}
		s = out
	}
	// Remove DC and normalise to the loudest window.
	var mean float64
	for _, v := range s {
		mean += float64(v)
	}
	mean /= float64(max(len(s), 1))
	var peak float32
	for i := range s {
		s[i] -= float32(mean)
		if v := float32(math.Abs(float64(s[i]))); v > peak {
			peak = v
		}
	}
	if peak > 0 {
		for i := range s {
			s[i] /= peak
		}
	}
	return s
}

func envelope(s []float32) []float64 {
	var e []float64
	for i := 0; i+win <= len(s); i += win {
		var sum float64
		for _, v := range s[i : i+win] {
			sum += float64(v) * float64(v)
		}
		e = append(e, math.Sqrt(sum/win))
	}
	return e
}

// align finds the lag (in windows) maximising the correlation of the
// "sounding" envelopes.
func align(a, b []float64, maxLag int) int {
	best, bestLag := -1.0, 0
	for lag := -maxLag; lag <= maxLag; lag++ {
		var c float64
		for i := range a {
			j := i + lag
			if j < 0 || j >= len(b) {
				continue
			}
			if (a[i] > quiet) == (b[j] > quiet) {
				c++
			}
		}
		if c > best {
			best, bestLag = c, lag
		}
	}
	return bestLag
}

// spectrum is the magnitude spectrum of fftN samples from p (Hann window).
func spectrum(s []float32, p int) []float64 {
	x := make([]complex128, fftN)
	for i := range x {
		if p+i < len(s) {
			w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/fftN)
			x[i] = complex(float64(s[p+i])*w, 0)
		}
	}
	fft(x)
	m := make([]float64, fftN/2)
	for i := range m {
		m[i] = cmplx.Abs(x[i])
	}
	return m
}

// present reports whether a's strongest bin (above 60 Hz) is in b, within
// one bin, at a quarter of b's peak.
func present(a, b []float64) bool {
	lo := 60 * fftN / rate
	pk, at := 0.0, 0
	bp := 0.0
	for i := lo; i < len(a); i++ {
		if a[i] > pk {
			pk, at = a[i], i
		}
		bp = math.Max(bp, b[i])
	}
	for d := -1; d <= 1; d++ {
		if k := at + d; k >= lo && k < len(b) && b[k] >= bp/4 {
			return true
		}
	}
	return false
}

func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for l := 2; l <= n; l <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(l)))
		for i := 0; i < n; i += l {
			wk := complex(1, 0)
			for k := 0; k < l/2; k++ {
				u, v := x[i+k], x[i+k+l/2]*wk
				x[i+k], x[i+k+l/2] = u+v, u-v
				wk *= w
			}
		}
	}
}

func peakHz(m []float64) float64 {
	pk, at := 0.0, 0
	for i := 60 * fftN / rate; i < len(m); i++ {
		if m[i] > pk {
			pk, at = m[i], i
		}
	}
	return float64(at) * rate / fftN
}
