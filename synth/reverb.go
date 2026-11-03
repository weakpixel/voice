package synth

import (
	"errors"
	"fmt"
	"math"
)

// Reverb sets the stream's reverb, a room every patch can send into with
// its Patch.Reverb level. All fields are 0 to 1.
type Reverb struct {
	Size    float32 // room size: decay from short (0) to very long (1)
	Damping float32 // how fast high frequencies die out
	Width   float32 // stereo width of the reverb's output: 0 mono, 1 full
}

// DefaultReverb is the reverb a new stream has: a medium hall.
var DefaultReverb = Reverb{Size: 0.75, Damping: 0.4, Width: 1}

func (r Reverb) validate() error {
	var errs []error
	for _, f := range []struct {
		name string
		v    float32
	}{{"size", r.Size}, {"damping", r.Damping}, {"width", r.Width}} {
		if !(f.v >= 0 && f.v <= 1) {
			errs = append(errs, fmt.Errorf("synth: reverb: %s: %g is outside 0 to 1", f.name, f.v))
		}
	}
	return errors.Join(errs...)
}

// The Freeverb tunings (Jezar at Dreampoint, public domain), in samples at
// 44.1 kHz. Mutually prime-ish lengths keep the comb echoes from lining up
// into audible pitches; the right channel is 23 samples longer, which
// decorrelates the channels and makes the reverb wide.
var (
	combTuning    = [...]int{1116, 1188, 1277, 1356, 1422, 1491, 1557, 1617}
	allpassTuning = [...]int{556, 441, 341, 225}
)

const (
	stereoSpread = 23
	reverbInGain = 0.015 // Freeverb's fixed input gain: 8 combs in parallel add up
	roomScale    = 0.28  // feedback = size*roomScale + roomOffset, 0.7 to 0.98
	roomOffset   = 0.7
	dampScale    = 0.4
	reverbWet    = 3 // Freeverb's wet scale, so a full send is about as loud as dry
	// silence is the output level below which a reverb without input is
	// treated as finished; stopping there also keeps the filters away from
	// denormal numbers, which are very slow on some CPUs.
	silence = 1e-6
)

// comb is a feedback comb filter with a one-pole low-pass in the feedback
// path: each echo comes back a little quieter and a little duller.
type comb struct {
	buf   []float32
	i     int
	store float32
	fb    float32
	damp1 float32
	damp2 float32
}

func (c *comb) process(x float32) float32 {
	y := c.buf[c.i]
	c.store = y*c.damp2 + c.store*c.damp1
	c.buf[c.i] = x + c.store*c.fb
	if c.i++; c.i == len(c.buf) {
		c.i = 0
	}
	return y
}

// allpass is a Schroeder allpass: it smears the comb echoes in time without
// colouring the spectrum.
type allpass struct {
	buf []float32
	i   int
}

func (a *allpass) process(x float32) float32 {
	b := a.buf[a.i]
	a.buf[a.i] = x + b*0.5
	if a.i++; a.i == len(a.buf) {
		a.i = 0
	}
	return b - x
}

// reverb is a stereo Freeverb fed by a mono bus. Its buffers are allocated
// once by newReverb; process allocates nothing.
type reverb struct {
	combs    [2][len(combTuning)]comb
	allpass  [2][len(allpassTuning)]allpass
	wet1     float32
	wet2     float32
	active   bool
	quiet    int // frames since the last input
	maxDelay int // longest path through the filters: output lags input by up to this much
}

func newReverb(rate int) *reverb {
	r := &reverb{}
	scale := float64(rate) / 44100
	size := func(n, ch int) int { return max(1, int(math.Round(float64(n+ch*stereoSpread)*scale))) }
	for ch := range 2 {
		for i, n := range combTuning {
			r.combs[ch][i].buf = make([]float32, size(n, ch))
		}
		for i, n := range allpassTuning {
			r.allpass[ch][i].buf = make([]float32, size(n, ch))
		}
	}
	r.maxDelay = size(combTuning[len(combTuning)-1], 1)
	for _, n := range allpassTuning {
		r.maxDelay += size(n, 1)
	}
	r.set(DefaultReverb)
	return r
}

func (r *reverb) set(p Reverb) {
	fb := p.Size*roomScale + roomOffset
	damp := p.Damping * dampScale
	for ch := range 2 {
		for i := range r.combs[ch] {
			c := &r.combs[ch][i]
			c.fb, c.damp1, c.damp2 = fb, damp, 1-damp
		}
	}
	r.wet1 = reverbWet * (p.Width/2 + 0.5)
	r.wet2 = reverbWet * (1 - p.Width) / 2
}

// process adds the reverb of the mono bus to left and right. A reverb that
// got no input and has died away is skipped, so streams whose patches send
// nothing pay nothing.
func (r *reverb) process(bus, left, right []float32) {
	if !r.active {
		for _, x := range bus {
			if x != 0 {
				r.active = true
				break
			}
		}
		if !r.active {
			return
		}
	}
	var peak, in float32
	for f, x := range bus {
		x *= reverbInGain
		in = max(in, x, -x)
		var l, rt float32
		for i := range r.combs[0] {
			l += r.combs[0][i].process(x)
			rt += r.combs[1][i].process(x)
		}
		for i := range r.allpass[0] {
			l = r.allpass[0][i].process(l)
			rt = r.allpass[1][i].process(rt)
		}
		outL := l*r.wet1 + rt*r.wet2
		outR := rt*r.wet1 + l*r.wet2
		left[f] += outL
		right[f] += outR
		peak = max(peak, outL, -outL, outR, -outR)
	}
	if in > 0 {
		r.quiet = 0
	} else {
		r.quiet += len(bus)
	}
	// Echoes come out up to maxDelay after their input, so silence only
	// counts once that long has passed without input.
	if r.quiet > r.maxDelay && peak < silence {
		r.clear()
	}
}

// clear empties the delay lines and stops processing until input arrives.
func (r *reverb) clear() {
	for ch := range 2 {
		for i := range r.combs[ch] {
			clear(r.combs[ch][i].buf)
			r.combs[ch][i].store = 0
		}
		for i := range r.allpass[ch] {
			clear(r.allpass[ch][i].buf)
		}
	}
	r.active = false
	r.quiet = 0
}
