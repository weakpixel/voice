package main

import (
	"math"
	"time"

	"github.com/weakpixel/voice/synth"
)

type namedPatch struct {
	name  string
	patch synth.Patch
}

const ms = time.Millisecond

// testPatches covers every feature of the synthesizer once: each waveform,
// pulse width, detune, glide, vibrato, the filter with resonance, and a
// one-shot sample. They are listening material, not the starter voices.
func testPatches() []namedPatch {
	return []namedPatch{
		{"sine blip", synth.Patch{Wave: synth.Sine, Env: synth.Envelope{Attack: 3 * ms, Decay: 90 * ms, Release: 30 * ms}}},
		{"triangle soft", synth.Patch{Wave: synth.Triangle, Env: synth.Envelope{Attack: 10 * ms, Decay: 60 * ms, Sustain: 0.6, Release: 60 * ms}}},
		{"square chirp", synth.Patch{Wave: synth.Square, PulseWidth: 0.25, Gain: 0.4, Glide: synth.Glide{From: 5, Time: 25 * ms},
			Env: synth.Envelope{Attack: 2 * ms, Decay: 80 * ms, Sustain: 0.3, Release: 30 * ms}}},
		{"saw gruff", synth.Patch{Wave: synth.Saw, Detune: 0.15, Gain: 0.5, Filter: synth.Filter{Cutoff: 1200, Resonance: 0.4},
			Env: synth.Envelope{Attack: 4 * ms, Decay: 100 * ms, Sustain: 0.4, Release: 40 * ms}}},
		{"square vibrato", synth.Patch{Wave: synth.Square, Gain: 0.4, Vibrato: synth.Vibrato{Rate: 7, Depth: 0.4, Delay: 30 * ms},
			Filter: synth.Filter{Cutoff: 2500}, Env: synth.Envelope{Attack: 5 * ms, Sustain: 1, Release: 50 * ms}}},
		{"noise hiss", synth.Patch{Wave: synth.Noise, Gain: 0.5, Filter: synth.Filter{Cutoff: 4000, Resonance: 0.5},
			Env: synth.Envelope{Attack: 2 * ms, Decay: 50 * ms, Release: 20 * ms}}},
		{"sample pluck", synth.Patch{Sample: pluckSample(), Env: synth.Envelope{Sustain: 1, Release: 40 * ms}}},
	}
}

// pluckSample synthesizes a 0.4 s plucked string at 220 Hz (note 57) with
// the Karplus-Strong algorithm: a noise burst in a delay line, averaged on
// every pass, so the high partials die first.
func pluckSample() *synth.Sample {
	const rate, hz = 44100, 220.0
	period := int(math.Round(rate / hz))
	line := make([]float32, period)
	seed := uint32(12345)
	for i := range line {
		seed = seed*1664525 + 1013904223
		line[i] = float32(seed>>8)/(1<<23) - 1
	}
	data := make([]float32, rate*4/10)
	for i := range data {
		j := i % period
		data[i] = line[j] * 0.6
		line[j] = 0.5 * (line[j] + line[(j+1)%period]) * 0.996
	}
	return &synth.Sample{Data: data, Rate: rate, Root: 57}
}
