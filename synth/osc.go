package synth

import "math"

// oscillator produces one waveform. phase runs from 0 to 1 per cycle; inc is
// the frequency divided by the sample rate, set once per render block.
type oscillator struct {
	wave  Waveform
	pw    float64
	phase float64
	inc   float64
	rng   uint32 // xorshift state for Noise, never 0
}

func (o *oscillator) init(wave Waveform, pw float64, seed uint32) {
	*o = oscillator{wave: wave, pw: pw, rng: seed | 1}
}

// setFreq sets the frequency; it is capped just below Nyquist so the phase
// step stays under half a cycle.
func (o *oscillator) setFreq(hz float64, rate int) {
	o.inc = min(hz/float64(rate), 0.49)
}

// polyBLEP is the polynomial band-limited step correction: subtracted around
// each discontinuity of a naive saw or square, it removes most of the
// aliasing that makes naive waveforms sound harsh and "digital", at the cost
// of a few multiplications near the jump. t is the phase, dt the phase step.
func polyBLEP(t, dt float64) float64 {
	switch {
	case t < dt:
		t /= dt
		return t + t - t*t - 1
	case t > 1-dt:
		t = (t - 1) / dt
		return t*t + t + t + 1
	}
	return 0
}

func (o *oscillator) next() float32 {
	t, dt := o.phase, o.inc
	var y float64
	switch o.wave {
	case Sine:
		y = math.Sin(2 * math.Pi * t)
	case Triangle:
		// Starts at 0 and rises, like the sine. Its harmonics fall off with
		// 1/n², so the naive shape aliases little.
		switch {
		case t < 0.25:
			y = 4 * t
		case t < 0.75:
			y = 2 - 4*t
		default:
			y = 4*t - 4
		}
	case Square:
		y = -1
		if t < o.pw {
			y = 1
		}
		y += polyBLEP(t, dt)
		t2 := t - o.pw
		if t2 < 0 {
			t2++
		}
		y -= polyBLEP(t2, dt)
	case Saw:
		y = 2*t - 1 - polyBLEP(t, dt)
	case Noise:
		o.rng ^= o.rng << 13
		o.rng ^= o.rng >> 17
		o.rng ^= o.rng << 5
		y = float64(o.rng)/(1<<31) - 1
	}
	o.phase += dt
	if o.phase >= 1 {
		o.phase--
	}
	return float32(y)
}
