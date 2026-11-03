package synth

import (
	"testing"
	"time"

	"github.com/weakpixel/voice"
)

// benchStream returns a stream with n voices held on a mix of patches.
func benchStream(b *testing.B, n int, filter bool) *Stream {
	s := newTestStream(b, DefaultVoices)
	for inst := range voice.InstrumentCount {
		p := Patch{Wave: Waveform(inst % 5), Detune: 0.1, Env: Envelope{Sustain: 1}}
		if filter {
			p.Filter = Filter{Cutoff: 2000, Resonance: 0.3}
		}
		setPatch(b, s, 1, voice.Instrument(inst), p)
	}
	for i := range n {
		s.NoteOn(1, voice.Instrument(i%voice.InstrumentCount), float32(48+i), 0.5, 0)
	}
	return s
}

// BenchmarkRead measures one Read of 1024 float32 frames (21 ms at 48 kHz).
// The rt% metric is the share of real time the render takes.
func BenchmarkRead(b *testing.B) {
	for _, c := range []struct {
		name   string
		voices int
		filter bool
	}{{"voices=0", 0, false}, {"voices=4", 4, false}, {"voices=8+filter", 8, true}, {"voices=24+filter", 24, true}} {
		b.Run(c.name, func(b *testing.B) {
			s := benchStream(b, c.voices, c.filter)
			buf := make([]byte, 8*1024)
			b.ReportAllocs()
			for b.Loop() {
				s.Read(buf)
			}
			per := b.Elapsed() / time.Duration(b.N)
			b.ReportMetric(100*per.Seconds()/(1024.0/testRate), "rt%")
		})
	}
}

// BenchmarkSink measures one NoteOn and NoteOff pair from the caller's side
// plus applying them in the renderer. Owner 2 has no patches, so no voice
// sounds and the render cost stays out of the number.
func BenchmarkSink(b *testing.B) {
	s := benchStream(b, 0, false)
	buf := make([]byte, 8*BlockFrames)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		s.NoteOn(2, 0, 60, 1, 0)
		s.NoteOff(2, 60)
		if i++; i%64 == 0 {
			s.Read(buf) // drain the command buffer now and then
		}
	}
}
