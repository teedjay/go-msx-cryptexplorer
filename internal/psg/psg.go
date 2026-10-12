// Package psg models the AY-3-8910 sound chip of the MSX: three square
// tone channels, a noise generator, the mixer, a logarithmic volume table
// and the envelope generator. The game only writes registers; this turns
// a frame's registers into samples. Render each frame's samples from that
// frame's registers, on the game's thread: applying registers once per
// audio-callback chunk garbles short effects.
package psg

// Clock is the PSG clock on the MSX (3.579545 MHz / 2).
const Clock = 1789772.5

// volume is the AY-3-8910 output level per volume step (about 3dB a
// step), normalised to 1.
var volume = [16]float32{
	0, 0.0078, 0.0110, 0.0156, 0.0221, 0.0312, 0.0441, 0.0624,
	0.0883, 0.1249, 0.1766, 0.2497, 0.3532, 0.4995, 0.7064, 1.0,
}

// Chip is the PSG state that persists between frames.
type Chip struct {
	Regs [16]byte

	toneCount [3]int
	toneOut   [3]bool
	noiseCnt  int
	noiseLFSR uint32
	envCnt    int
	envStep   int
	envHold   bool
	envUp     bool
	lastShape byte
	frac      float64
	// DC blocker state: the MSX's output is AC-coupled, so a constant
	// level (a channel with tone and noise both off still outputs its
	// volume) is silent there.
	dcIn, dcOut float32
}

// New returns a chip in its reset state.
func New() *Chip { return &Chip{noiseLFSR: 1} }

// SetRegs takes the registers of a frame. A write to R13 restarts the
// envelope; the caller passes envRestart when the game wrote R13.
func (c *Chip) SetRegs(r [16]byte, envRestart bool) {
	c.Regs = r
	if envRestart {
		c.envStep = 0
		c.envHold = false
		c.envUp = r[13]&0x04 != 0
		c.envCnt = 0
	}
}

// step advances the chip by one tick of Clock/8 and returns the mixed
// output.
func (c *Chip) step() float32 {
	r := &c.Regs
	// A tone of period p is Clock/(16p) Hz: it toggles every p steps.
	for ch := 0; ch < 3; ch++ {
		p := int(r[ch*2]) | int(r[ch*2+1]&0x0F)<<8
		if p == 0 {
			p = 1
		}
		c.toneCount[ch]++
		if c.toneCount[ch] >= p {
			c.toneCount[ch] = 0
			c.toneOut[ch] = !c.toneOut[ch]
		}
	}
	np := int(r[6] & 0x1F)
	if np == 0 {
		np = 1
	}
	c.noiseCnt++
	if c.noiseCnt >= np*2 {
		c.noiseCnt = 0
		bit := (c.noiseLFSR ^ c.noiseLFSR>>3) & 1
		c.noiseLFSR = c.noiseLFSR>>1 | bit<<16
	}
	noise := c.noiseLFSR&1 != 0
	// Envelope at Clock/256: a step every 32 of ours, times the period.
	ep := int(r[11]) | int(r[12])<<8
	if ep == 0 {
		ep = 1
	}
	c.envCnt++
	if c.envCnt >= ep*32 {
		c.envCnt = 0
		c.envTick()
	}
	env := c.envStep
	if !c.envUp {
		env = 15 - c.envStep
	}
	var out float32
	for ch := 0; ch < 3; ch++ {
		toneOn := r[7]&(1<<ch) == 0
		noiseOn := r[7]&(8<<ch) == 0
		on := (!toneOn || c.toneOut[ch]) && (!noiseOn || noise)
		if !on {
			continue
		}
		v := r[8+ch]
		if v&0x10 != 0 {
			out += volume[env]
		} else {
			out += volume[v&0x0F]
		}
	}
	return out / 3
}

func (c *Chip) envTick() {
	if c.envHold {
		return
	}
	c.envStep++
	if c.envStep < 16 {
		return
	}
	shape := c.Regs[13] & 0x0F
	cont, alt, hold := shape&8 != 0, shape&2 != 0, shape&1 != 0
	switch {
	case !cont: // one ramp, then silence
		c.envUp, c.envStep, c.envHold = false, 15, true
	case hold: // one ramp, then hold at the end (or the start, alternating)
		if alt {
			c.envUp = !c.envUp
		}
		c.envStep, c.envHold = 15, true
	default: // repeat, alternating direction if asked
		if alt {
			c.envUp = !c.envUp
		}
		c.envStep = 0
	}
}

// Render appends n samples at the given rate, box-filtering the chip's
// Clock/8 output.
func (c *Chip) Render(out []float32, n int, rate float64) []float32 {
	per := Clock / 8 / rate
	for i := 0; i < n; i++ {
		c.frac += per
		steps := int(c.frac)
		c.frac -= float64(steps)
		var sum float32
		for s := 0; s < steps; s++ {
			sum += c.step()
		}
		if steps > 0 {
			sum /= float32(steps)
		}
		y := sum - c.dcIn + 0.995*c.dcOut
		c.dcIn, c.dcOut = sum, y
		out = append(out, y)
	}
	return out
}
