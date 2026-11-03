package synth

import (
	"math"
	"testing"
	"time"

	"github.com/weakpixel/voice"
)

const testRate = 48000

func newTestStream(t testing.TB, voices int) *Stream {
	t.Helper()
	s, err := NewStream(StreamConfig{SampleRate: testRate, Voices: voices})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func setPatch(t testing.TB, s *Stream, owner voice.Owner, inst voice.Instrument, p Patch) {
	t.Helper()
	if err := s.SetPatch(owner, inst, p); err != nil {
		t.Fatal(err)
	}
}

func frames(d time.Duration) int { return int(d.Seconds() * testRate) }

// render returns the next d of the left channel.
func render(s *Stream, d time.Duration) []float32 {
	n := frames(d)
	l, r := make([]float32, n), make([]float32, n)
	s.Render(l, r)
	return l
}

// playNote renders note on patch p for d with the note held.
func playNote(t testing.TB, p Patch, note float32, d time.Duration) []float32 {
	t.Helper()
	s := newTestStream(t, 4)
	setPatch(t, s, 1, voice.InstVowel, p)
	s.NoteOn(1, voice.InstVowel, note, 1, 0)
	return render(s, d)
}

// pitch estimates the frequency of x from its upward zero crossings,
// interpolated between frames.
func pitch(x []float32) float64 {
	first, last, n := -1.0, -1.0, 0
	for i := 1; i < len(x); i++ {
		if x[i-1] < 0 && x[i] >= 0 {
			pos := float64(i-1) + float64(-x[i-1]/(x[i]-x[i-1]))
			if first < 0 {
				first = pos
			}
			last = pos
			n++
		}
	}
	if n < 2 {
		return 0
	}
	return float64(n-1) * testRate / (last - first)
}

func peak(x []float32) float32 {
	var p float32
	for _, v := range x {
		p = max(p, float32(math.Abs(float64(v))))
	}
	return p
}

func rms(x []float32) float64 {
	var sum float64
	for _, v := range x {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum / float64(len(x)))
}

// maxStep is the largest difference between neighbouring frames: a click
// shows up as a step far above what the waveform itself produces.
func maxStep(x []float32) float32 {
	var m float32
	for i := 1; i < len(x); i++ {
		m = max(m, float32(math.Abs(float64(x[i]-x[i-1]))))
	}
	return m
}

func noteHz(note float64) float64 { return 440 * math.Exp2((note-69)/12) }
