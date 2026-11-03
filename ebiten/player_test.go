package ebiten

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/synth"
	"github.com/weakpixel/voice/voicetest"
)

const testRate = 48000

// Ebiten opens the audio device only from its game loop, so a player in a
// test stays pending and nothing is heard: this covers construction, the
// checks and Close, not the device.
func context(t *testing.T) *audio.Context {
	ctx := audio.CurrentContext()
	if ctx == nil {
		ctx = audio.NewContext(testRate)
	}
	if ctx.SampleRate() != testRate {
		t.Skipf("audio context already runs at %d Hz", ctx.SampleRate())
	}
	return ctx
}

func TestNewPlayer(t *testing.T) {
	ctx := context(t)
	s, err := synth.NewStream(synth.StreamConfig{SampleRate: testRate})
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewPlayer(ctx, s, 0)
	if err != nil {
		t.Fatal(err)
	}
	p.SetVolume(0.5)
	if p.Volume() != 0.5 {
		t.Errorf("volume %g", p.Volume())
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewPlayerErrors(t *testing.T) {
	ctx := context(t)
	s, _ := synth.NewStream(synth.StreamConfig{SampleRate: 44100, Format: synth.FormatInt16})
	_, err := NewPlayer(ctx, s, 0)
	if err == nil || !strings.Contains(err.Error(), "FormatFloat32") || !strings.Contains(err.Error(), "44100 Hz") {
		t.Errorf("error %v", err)
	}
	if _, err := NewPlayer(nil, s, 0); err == nil {
		t.Error("nil context accepted")
	}
}

// TestSpeakerThroughStream renders a line the way the player would read it,
// without a device: a Speaker on the stream, Read in player-sized chunks.
func TestSpeakerThroughStream(t *testing.T) {
	s, _ := synth.NewStream(synth.StreamConfig{SampleRate: testRate})
	v, _ := voice.Preset("chirpy")
	if err := s.SetVoice(1, v, nil); err != nil {
		t.Fatal(err)
	}
	rec := voicetest.NewRecordingSink(0)
	sp := voice.NewSpeaker(teeSink{s, rec}, 1)
	sp.Say(v, "Hi there!", voice.SayOptions{})
	buf := make([]byte, 8*testRate/60) // one 60 Hz frame of float32 stereo
	loud := false
	for range 600 {
		sp.Update(time.Second / 60)
		n, _ := s.Read(buf)
		for i := 0; i+4 <= n; i += 4 {
			if math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[i:])))) > 0.05 {
				loud = true
			}
		}
		if sp.Done() {
			break
		}
	}
	if !sp.Done() || !loud {
		t.Errorf("done %v, audible %v", sp.Done(), loud)
	}
	if n := rec.Sounding(1); len(n) != 0 {
		t.Errorf("%d notes left sounding", len(n))
	}
}

type teeSink struct{ a, b voice.Sink }

func (t teeSink) NoteOn(o voice.Owner, i voice.Instrument, n, v, p float32) {
	t.a.NoteOn(o, i, n, v, p)
	t.b.NoteOn(o, i, n, v, p)
}
func (t teeSink) NoteOff(o voice.Owner, n float32) { t.a.NoteOff(o, n); t.b.NoteOff(o, n) }
func (t teeSink) Release(o voice.Owner)            { t.a.Release(o); t.b.Release(o) }
