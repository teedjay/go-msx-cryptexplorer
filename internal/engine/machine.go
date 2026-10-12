package engine

// Machine runs the console frame by frame for the playable frontend, the
// screenshot tool and the input search, pacing ticks the way the
// original does: a tick that does more work than fits in a frame keeps
// the busy flag set for the frames it owes, and those frames' interrupts
// run only the sound work.

// frameCycles is one 60Hz frame of the Z80 at 3.58MHz.
const frameCycles = 59659

// Pacing model: cycles = base + the coefficients below times the
// console's per-tick counters. Fitted with cmd/pacefit to the traced
// ticks (attract and random play: 17,998 ticks, none mispredicted). The
// coefficients are a fit, not Z80 instruction timings: the game logic is
// folded into the base.
var pacing = struct {
	base, setup, write, mirror, ldir, fill, wrtvrm, rle float64
}{4500, 75, 45, 300, 143.5, 16.9, 150, 33.8}

// initInterrupts are the VRAM write counts (since INIT's entry) at which
// the reference machine took its 19 INIT-time interrupts: the first when
// WRTPSG re-enables interrupts, the rest during the 16KB VRAM clear.
var initInterrupts = []NestPoint{{0, 0, false}, {801, 1, false}, {1689, 1, false}, {2604, 1, false}, {3519, 1, false}, {4434, 1, false}, {5349, 1, false}, {6264, 1, false}, {7179, 1, false}, {8094, 1, false}, {9009, 1, false}, {9924, 1, false}, {10840, 1, false}, {11755, 1, false}, {12670, 1, false}, {13585, 1, false}, {14500, 1, false}, {15415, 1, false}, {16330, 1, false}}

// Machine is a console plus the frame pacing.
type Machine struct {
	*Console
	owe int // frames the last tick still owes
}

// NewMachine powers on and runs INIT. The game RAM INIT does not clear
// ($E700-$EFFF) holds the reference machine's power-on pattern: $FF,
// with $F0 at the start of every 256-byte page.
func NewMachine() *Machine {
	c := New()
	for i := 0x700; i < RAMSize; i++ {
		c.RAM[i] = 0xFF
		if i&0xFF == 0 {
			c.RAM[i] = 0xF0
		}
	}
	c.NestAt = append([]NestPoint(nil), initInterrupts...)
	c.Boot()
	c.NestAt = nil
	return &Machine{Console: c}
}

// TickCycles estimates the cycles of the tick that just ran.
func (c *Console) TickCycles() float64 {
	p := pacing
	return p.base + p.setup*float64(c.SetAddrs) + p.write*float64(c.VW) +
		p.mirror*float64(c.Cost[0]) + p.ldir*float64(c.Cost[1]) +
		p.fill*float64(c.Cost[2]) + p.wrtvrm*float64(c.Cost[3]) +
		p.rle*float64(c.Cost[4])
}

// WillTick reports whether the next frame starts a tick (the game reads
// its input then).
func (m *Machine) WillTick() bool { return m.owe == 0 }

// Frame runs one 60Hz frame: either the interrupt of a busy frame (sound
// only) or the frame interrupt and a whole tick. It reports whether a
// tick ran.
func (m *Machine) Frame() bool { return m.FrameWith(nil) }

// FrameWith is Frame with a hook called at the tick start, where the
// reference traces record the state.
func (m *Machine) FrameWith(atTick func()) bool {
	if m.owe > 0 {
		m.owe--
		m.nestedInterrupt()
		return false
	}
	m.Interrupt()
	if atTick != nil {
		atTick()
	}
	m.Tick()
	m.owe = int(m.TickCycles() / frameCycles)
	return true
}

// Clone returns an independent copy of the machine (for searching
// inputs on the port).
func (m *Machine) Clone() *Machine {
	c := *m.Console
	c.NestAt = nil
	return &Machine{Console: &c, owe: m.owe}
}

// PlaySound starts sound id (bit 7 music) for tools that preview sounds;
// SoundFrame then advances only the sound driver by a frame.
func (m *Machine) PlaySound(id byte) { m.soundStart(id) }

// SoundFrame runs only the per-interrupt sound work.
func (m *Machine) SoundFrame() { m.soundTick() }
