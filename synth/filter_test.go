package synth

import (
	"math"
	"testing"
	"time"
)

func TestFilterAttenuation(t *testing.T) {
	level := func(note float32, f Filter) float64 {
		// Gain 0.1 keeps the resonant peak below the output clipping.
		x := playNote(t, Patch{Filter: f, Gain: 0.1, Env: Envelope{Sustain: 1}}, note, 300*time.Millisecond)
		return rms(x[frames(50*time.Millisecond):])
	}
	open := level(93, Filter{})                            // 1760 Hz, no filter
	low := level(93, Filter{Cutoff: 220})                  // three octaves above the cutoff
	pass := level(45, Filter{Cutoff: 1760})                // 110 Hz, four octaves below
	reso := level(69, Filter{Cutoff: 440, Resonance: 0.9}) // at the cutoff

	// A 12 dB/octave low-pass: three octaves above is about -36 dB.
	if db := 20 * math.Log10(low/open); db > -30 {
		t.Errorf("3 octaves above the cutoff: %.1f dB, want below -30 dB", db)
	}
	if db := 20 * math.Log10(pass/open); math.Abs(db) > 0.5 {
		t.Errorf("far below the cutoff: %.1f dB, want about 0 dB", db)
	}
	if db := 20 * math.Log10(reso/open); db < 10 {
		t.Errorf("resonance 0.9 at the cutoff: %.1f dB, want a peak above 10 dB", db)
	}
}

func TestFilterStableAtHighCutoff(t *testing.T) {
	x := playNote(t, Patch{Wave: Saw, Filter: Filter{Cutoff: 30000, Resonance: 0.95}, Env: Envelope{Sustain: 1}}, 60, 500*time.Millisecond)
	for i, v := range x {
		if math.IsNaN(float64(v)) || math.Abs(float64(v)) > 1 {
			t.Fatalf("frame %d: %g", i, v)
		}
	}
}
