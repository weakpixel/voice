package synth

import (
	"math"
	"testing"
	"time"
)

func TestOscPitch(t *testing.T) {
	for _, wave := range []Waveform{Sine, Triangle, Square, Saw} {
		for _, note := range []float32{48, 60, 69, 81.5} {
			t.Run(wave.String(), func(t *testing.T) {
				x := playNote(t, Patch{Wave: wave, Env: Envelope{Sustain: 1}}, note, 500*time.Millisecond)
				got, want := pitch(x[frames(20*time.Millisecond):]), noteHz(float64(note))
				if math.Abs(got-want)/want > 0.005 {
					t.Errorf("note %g: pitch %.2f Hz, want %.2f Hz", note, got, want)
				}
			})
		}
	}
}

func TestOscDetune(t *testing.T) {
	// A second oscillator an octave up still crosses zero at the base rate.
	x := playNote(t, Patch{Wave: Sine, Detune: 12, Env: Envelope{Sustain: 1}}, 57, 300*time.Millisecond)
	if p := peak(x); p < 0.5 || p > 1 {
		t.Errorf("peak %g, want 0.5 to 1", p)
	}
}

func TestOscLevelsAndNoise(t *testing.T) {
	for _, wave := range []Waveform{Sine, Triangle, Square, Saw, Noise} {
		t.Run(wave.String(), func(t *testing.T) {
			x := playNote(t, Patch{Wave: wave, Env: Envelope{Sustain: 1}}, 60, 200*time.Millisecond)
			x = x[frames(10*time.Millisecond):]
			// PolyBLEP may overshoot slightly; nothing may go far beyond 1.
			if p := peak(x); p < 0.5 || p > 1.1 {
				t.Errorf("peak %g, want 0.5 to 1.1", p)
			}
			if r := rms(x); r < 0.2 {
				t.Errorf("rms %g, want at least 0.2", r)
			}
		})
	}
}

func TestPolyBLEPReducesAliasing(t *testing.T) {
	// A high saw: the band-limited version has a smaller jump at the reset
	// than a naive one (which jumps by 2).
	x := playNote(t, Patch{Wave: Saw, Env: Envelope{Sustain: 1}}, 100, 100*time.Millisecond)
	if s := maxStep(x[frames(10*time.Millisecond):]); s >= 1.9 {
		t.Errorf("max step %g, want below the naive 2", s)
	}
}

func TestPulseWidth(t *testing.T) {
	// The share of positive frames follows the pulse width.
	for _, pw := range []float32{0.25, 0.5, 0.75} {
		x := playNote(t, Patch{Wave: Square, PulseWidth: pw, Env: Envelope{Sustain: 1}}, 57, 300*time.Millisecond)
		x = x[frames(10*time.Millisecond):]
		pos := 0
		for _, v := range x {
			if v > 0 {
				pos++
			}
		}
		if got := float64(pos) / float64(len(x)); math.Abs(got-float64(pw)) > 0.02 {
			t.Errorf("pulse width %g: positive share %.3f", pw, got)
		}
	}
}

func TestGlideAndVibrato(t *testing.T) {
	// Glide from an octave below: the first 10 ms are lower than the note,
	// the time after the glide is at the note.
	x := playNote(t, Patch{Glide: Glide{From: -12, Time: 50 * time.Millisecond}, Env: Envelope{Sustain: 1}}, 69, 400*time.Millisecond)
	if p := pitch(x[:frames(20*time.Millisecond)]); p > 380 {
		t.Errorf("glide start pitch %.1f Hz, want well below 440", p)
	}
	if p := pitch(x[frames(100*time.Millisecond):]); math.Abs(p-440) > 2 {
		t.Errorf("after glide pitch %.1f Hz, want 440", p)
	}

	// Vibrato of 1 semitone at 5 Hz: the pitch over a quarter period near
	// the crest is above the note; the mean over whole periods is the note.
	v := playNote(t, Patch{Vibrato: Vibrato{Rate: 5, Depth: 1}, Env: Envelope{Sustain: 1}}, 69, time.Second)
	crest := pitch(v[frames(25*time.Millisecond):frames(75*time.Millisecond)])
	if crest < 455 {
		t.Errorf("vibrato crest %.1f Hz, want above 455", crest)
	}
	if p := pitch(v); math.Abs(p-440) > 3 {
		t.Errorf("vibrato mean pitch %.1f Hz, want 440", p)
	}
}
