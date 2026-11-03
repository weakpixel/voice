package synth

import (
	"math"
	"strings"
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

func TestFilterTypes(t *testing.T) {
	level := func(note float32, f Filter) float64 {
		x := playNote(t, Patch{Filter: f, Gain: 0.1, Env: Envelope{Sustain: 1}}, note, 300*time.Millisecond)
		return rms(x[frames(50*time.Millisecond):])
	}
	db := func(a, b float64) float64 { return 20 * math.Log10(a/b) }
	high := Filter{Type: HighPass, Cutoff: 1760}
	if d := db(level(45, high), level(45, Filter{})); d > -40 {
		t.Errorf("high-pass, 4 octaves below the cutoff: %.1f dB, want below -40 dB", d)
	}
	if d := db(level(117, high), level(117, Filter{})); math.Abs(d) > 1 {
		t.Errorf("high-pass, 2 octaves above the cutoff: %.1f dB, want about 0 dB", d)
	}
	band := Filter{Type: BandPass, Cutoff: 880}
	center := level(81, band)
	for _, note := range []float32{45, 117} {
		if d := db(level(note, band), center); d > -12 {
			t.Errorf("band-pass, 3 octaves from the center (note %g): %.1f dB below the center, want below -12 dB", note, d)
		}
	}
	bad := Patch{Filter: Filter{Type: 3}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "filter.type") {
		t.Errorf("unknown filter type: %v", err)
	}
}
