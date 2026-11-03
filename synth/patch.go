package synth

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Waveform is the shape of an oscillator.
type Waveform uint8

// The oscillator waveforms. Square and Saw are band-limited with PolyBLEP;
// Noise is white noise and ignores the pitch (shape it with the filter).
const (
	Sine Waveform = iota
	Triangle
	Square
	Saw
	Noise
)

var waveformNames = [...]string{"sine", "triangle", "square", "saw", "noise"}

// String returns the waveform's name ("sine", "triangle", "square", "saw",
// "noise").
func (w Waveform) String() string {
	if int(w) < len(waveformNames) {
		return waveformNames[w]
	}
	return fmt.Sprintf("waveform(%d)", w)
}

// MinEnvelopeTime is the shortest attack, release (and, when the sustain is
// below 1, decay) an [Envelope] gets. Shorter values are raised to it, so a
// note never starts or stops with a click.
const MinEnvelopeTime = 2 * time.Millisecond

// Envelope is a linear attack, decay, sustain, release envelope. Attack
// rises from 0 to 1, decay falls to Sustain, the level holds until the note
// is released, and release falls to 0. A Sustain of 0 ends the note after the
// decay without a note off, which suits short blips.
type Envelope struct {
	Attack  time.Duration
	Decay   time.Duration
	Sustain float32 // 0 to 1
	Release time.Duration
}

// Glide slides the pitch into the note: it starts From semitones away from
// the note (negative is below) and reaches it linearly after Time. A zero
// Time turns it off.
type Glide struct {
	From float32
	Time time.Duration
}

// Vibrato modulates the pitch with a sine of Rate hertz and Depth semitones
// (peak), starting Delay after the note on. A zero Rate or Depth turns it
// off.
type Vibrato struct {
	Rate  float32
	Depth float32
	Delay time.Duration
}

// Filter is a resonant state-variable low-pass. Cutoff is in hertz (0 turns
// the filter off; values above 45 % of the sample rate are lowered to it);
// Resonance is 0 (none) to just below 1 (strong peak at the cutoff).
type Filter struct {
	Cutoff    float32
	Resonance float32
}

// Patch is one instrument sound: an oscillator (Wave, PulseWidth, Detune) or,
// when Sample is set, sample playback, followed by the filter, the envelope
// and the gain. Glide and Vibrato modulate the pitch of either.
//
// The zero values of PulseWidth and Gain mean their defaults (0.5 and 1), so
// a zero Patch is a plain sine with the shortest envelope.
type Patch struct {
	Wave Waveform
	// PulseWidth is the high part of a Square's cycle, 0.01 to 0.99; 0 means 0.5.
	PulseWidth float32
	// Detune adds a second oscillator of the same wave this many semitones
	// away, mixed half and half; 0 means a single oscillator.
	Detune float32
	// Sample replaces the oscillator when set. The stream keeps the pointer:
	// do not change the sample after handing it over.
	Sample  *Sample
	Env     Envelope
	Glide   Glide
	Vibrato Vibrato
	Filter  Filter
	// Gain scales the patch's output; 0 means 1.
	Gain float32
	// Reverb is how much of the patch goes into the stream's reverb, 0 (dry)
	// to 1. See Stream.SetReverb.
	Reverb float32
	// Layers are semitone offsets played together for every note, like a
	// chord: [0, -12, 7] plays the note, an octave below and a fifth above.
	// Each layer takes a voice of the pool, and the velocity is scaled by
	// 1/sqrt(len) so a chord is about as loud as one note. Empty means [0];
	// at most MaxLayers, each -24 to 24.
	Layers []float32
	// Unison runs this many copies of the oscillator, detuned evenly across
	// Spread semitones (in total), for the sound of a section instead of a
	// single instrument. 0 and 1 mean one oscillator; at most MaxUnison.
	// Oscillator patches only.
	Unison int
	Spread float32
}

// The limits of Patch.Layers and Patch.Unison.
const (
	MaxLayers = 4
	MaxUnison = 7
)

// Validate reports every problem of the patch together (errors.Join), or nil.
func (p *Patch) Validate() error {
	var errs []error
	bad := func(field, format string, args ...any) {
		errs = append(errs, fmt.Errorf("synth: patch: %s: %s", field, fmt.Sprintf(format, args...)))
	}
	finite := func(field string, v float32) bool {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			bad(field, "not a finite number")
			return false
		}
		return true
	}
	if int(p.Wave) >= len(waveformNames) {
		bad("wave", "unknown waveform %d", p.Wave)
	}
	if finite("pulseWidth", p.PulseWidth) && p.PulseWidth != 0 && (p.PulseWidth < 0.01 || p.PulseWidth > 0.99) {
		bad("pulseWidth", "%g is outside 0.01 to 0.99", p.PulseWidth)
	}
	finite("detune", p.Detune)
	for _, d := range []struct {
		field string
		v     time.Duration
	}{
		{"env.attack", p.Env.Attack}, {"env.decay", p.Env.Decay}, {"env.release", p.Env.Release},
		{"glide.time", p.Glide.Time}, {"vibrato.delay", p.Vibrato.Delay},
	} {
		if d.v < 0 {
			bad(d.field, "negative duration %v", d.v)
		}
	}
	if finite("env.sustain", p.Env.Sustain) && (p.Env.Sustain < 0 || p.Env.Sustain > 1) {
		bad("env.sustain", "%g is outside 0 to 1", p.Env.Sustain)
	}
	finite("glide.from", p.Glide.From)
	if finite("vibrato.rate", p.Vibrato.Rate) && p.Vibrato.Rate < 0 {
		bad("vibrato.rate", "negative rate %g", p.Vibrato.Rate)
	}
	finite("vibrato.depth", p.Vibrato.Depth)
	if finite("filter.cutoff", p.Filter.Cutoff) && p.Filter.Cutoff < 0 {
		bad("filter.cutoff", "negative cutoff %g", p.Filter.Cutoff)
	}
	if finite("filter.resonance", p.Filter.Resonance) && (p.Filter.Resonance < 0 || p.Filter.Resonance >= 1) {
		bad("filter.resonance", "%g is outside 0 to below 1", p.Filter.Resonance)
	}
	if finite("gain", p.Gain) && p.Gain < 0 {
		bad("gain", "negative gain %g", p.Gain)
	}
	if finite("reverb", p.Reverb) && (p.Reverb < 0 || p.Reverb > 1) {
		bad("reverb", "%g is outside 0 to 1", p.Reverb)
	}
	if len(p.Layers) > MaxLayers {
		bad("layers", "%d layers, at most %d", len(p.Layers), MaxLayers)
	}
	for i, l := range p.Layers {
		if finite(fmt.Sprintf("layers[%d]", i), l) && (l < -24 || l > 24) {
			bad(fmt.Sprintf("layers[%d]", i), "%g is outside -24 to 24", l)
		}
	}
	if p.Unison < 0 || p.Unison > MaxUnison {
		bad("unison", "%d is outside 0 to %d", p.Unison, MaxUnison)
	}
	if finite("spread", p.Spread) && (p.Spread < 0 || p.Spread > 12) {
		bad("spread", "%g is outside 0 to 12", p.Spread)
	}
	if p.Sample != nil {
		errs = append(errs, p.Sample.validate()...)
		if p.Unison > 1 {
			bad("unison", "needs an oscillator, not a sample")
		}
	}
	return errors.Join(errs...)
}

func (p *Patch) pulseWidth() float64 {
	if p.PulseWidth == 0 {
		return 0.5
	}
	return float64(p.PulseWidth)
}

func (p *Patch) unison() int { return max(p.Unison, 1) }

func (p *Patch) gain() float32 {
	if p.Gain == 0 {
		return 1
	}
	return p.Gain
}

// framesOf converts d to frames at rate, rounded, at least minFrames.
func framesOf(d time.Duration, rate int, minFrames int) int {
	n := int(math.Round(d.Seconds() * float64(rate)))
	return max(n, minFrames)
}
