package session

import (
	"bytes"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/weakpixel/voice/wav"
)

// sineWAV is a 16-bit PCM mono WAV of a 440 Hz sine, one second long. The
// wav package writes float32 only, so the file is built by hand.
func sineWAV(t *testing.T) []byte {
	t.Helper()
	const rate, n = 22050, 22050
	var b bytes.Buffer
	le := func(v any) {
		switch x := v.(type) {
		case uint32:
			b.Write([]byte{byte(x), byte(x >> 8), byte(x >> 16), byte(x >> 24)})
		case uint16:
			b.Write([]byte{byte(x), byte(x >> 8)})
		}
	}
	b.WriteString("RIFF")
	le(uint32(36 + 2*n))
	b.WriteString("WAVEfmt ")
	le(uint32(16))
	le(uint16(1))
	le(uint16(1))
	le(uint32(rate))
	le(uint32(rate * 2))
	le(uint16(2))
	le(uint16(16))
	b.WriteString("data")
	le(uint32(2 * n))
	for i := range n {
		le(uint16(int16(0.5 * 32767 * math.Sin(2*math.Pi*440*float64(i)/rate))))
	}
	if _, _, err := wav.Read(bytes.NewReader(b.Bytes())); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

const sampleVoice = `{"version": 0, "name": "hum", "instruments": {
  "vowel": {"wave": "triangle", "decayMs": 80},
  "motif": {"sample": "hum.wav", "root": 69, "loop": true, "sustain": 1, "releaseMs": 100}
}, "motifVolume": 1}`

func TestSamples(t *testing.T) {
	s, _ := New(rate)
	if p := s.MissingSamples(sampleVoice); len(p) != 1 || p[0].Path != "instruments.motif.sample" {
		t.Fatalf("missing samples %v", p)
	}
	v, problems := LoadVoice(sampleVoice)
	if v == nil {
		t.Fatal(problems)
	}
	if err := s.SetVoice(v); err == nil {
		t.Fatal("a voice with a missing sample was set")
	}
	if err := s.AddSample("hum.wav", []byte("not a wav")); err == nil {
		t.Error("a broken file was added")
	}
	if err := s.AddSample("hum.wav", sineWAV(t)); err != nil {
		t.Fatal(err)
	}
	if p := s.MissingSamples(sampleVoice); len(p) != 0 {
		t.Errorf("still missing: %v", p)
	}
	if !slices.Equal(s.Samples(), []string{"hum.wav"}) {
		t.Errorf("samples %v", s.Samples())
	}
	if err := s.SetVoice(v); err != nil {
		t.Fatal(err)
	}
	if err := s.Audition("motif"); err != nil {
		t.Fatal(err)
	}
	if p := play(s, 600*time.Millisecond, nil); p < 0.05 {
		t.Errorf("the sample does not sound: peak %g", p)
	}
	r, err := s.StartRender("Hello there.", Options{})
	if err != nil {
		t.Fatal(err)
	}
	for !r.Step(1 << 20) {
	}
	s.RemoveSample("hum.wav")
	if len(s.MissingSamples(sampleVoice)) != 1 {
		t.Error("a removed sample still counts")
	}
}
