package synth

import (
	"encoding/binary"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/weakpixel/voice"
)

var heldPatch = Patch{Env: Envelope{Sustain: 1, Release: 10 * time.Millisecond}}

func TestNewStreamErrors(t *testing.T) {
	_, err := NewStream(StreamConfig{SampleRate: 8000, Format: Format(7), Voices: -1})
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"sample rate 8000", "unknown format 7", "negative voice count -1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%v", want, err)
		}
	}
	for _, rate := range []int{MinSampleRate, 44100, MaxSampleRate} {
		if _, err := NewStream(StreamConfig{SampleRate: rate}); err != nil {
			t.Errorf("rate %d: %v", rate, err)
		}
	}
}

func TestSetPatchErrors(t *testing.T) {
	s := newTestStream(t, 1)
	if err := s.SetPatch(1, voice.Instrument(voice.InstrumentCount), Patch{}); err == nil {
		t.Error("out-of-range instrument accepted")
	}
	if err := s.SetPatch(1, 0, Patch{Gain: -1}); err == nil {
		t.Error("invalid patch accepted")
	}
}

func TestNoteWithoutPatchIsSilent(t *testing.T) {
	s := newTestStream(t, 2)
	setPatch(t, s, 1, voice.InstVowel, heldPatch)
	s.NoteOn(1, voice.InstMotif, 60, 1, 0) // no patch for the motif
	s.NoteOn(2, voice.InstVowel, 60, 1, 0) // no patches for owner 2
	if p := peak(render(s, 20*time.Millisecond)); p != 0 {
		t.Errorf("peak %g, want silence", p)
	}
}

func TestOwnersAreSeparate(t *testing.T) {
	s := newTestStream(t, 8)
	for owner := voice.Owner(1); owner <= 2; owner++ {
		setPatch(t, s, owner, voice.InstVowel, heldPatch)
		s.NoteOn(owner, voice.InstVowel, 60, 1, 0)
		s.NoteOn(owner, voice.InstVowel, 64, 1, 0)
	}
	render(s, 10*time.Millisecond)
	if n := s.pool.sounding(); n != 4 {
		t.Fatalf("%d voices sounding, want 4", n)
	}
	s.NoteOff(1, voice.InstVowel, 60)
	s.Release(2)
	render(s, 50*time.Millisecond)
	if n := s.pool.sounding(); n != 1 {
		t.Fatalf("%d voices sounding, want 1 (owner 1, note 64)", n)
	}
	for _, v := range s.pool.voices {
		if v.active && (v.on.owner != 1 || v.on.note != 64) {
			t.Errorf("voice of owner %d note %g survived", v.on.owner, v.on.note)
		}
	}
}

func TestNoteOnOffInOneBlock(t *testing.T) {
	// On and off before the next block: the note still sounds briefly and
	// ends; nothing is dropped and nothing hangs.
	s := newTestStream(t, 2)
	setPatch(t, s, 1, 0, heldPatch)
	s.NoteOn(1, 0, 69, 1, 0)
	s.NoteOff(1, voice.InstVowel, 69)
	x := render(s, 50*time.Millisecond)
	if peak(x) == 0 {
		t.Error("note never sounded")
	}
	if n := s.pool.sounding(); n != 0 {
		t.Errorf("%d voices still sounding", n)
	}
}

func TestStealing(t *testing.T) {
	s := newTestStream(t, 2)
	setPatch(t, s, 1, 0, Patch{Env: Envelope{Sustain: 1, Release: time.Second}})
	s.NoteOn(1, 0, 60, 1, 0)
	s.NoteOn(1, 0, 62, 1, 0)
	render(s, 10*time.Millisecond)
	s.NoteOff(1, voice.InstVowel, 62) // 62 releases (slowly): it is stolen before the older 60
	render(s, 10*time.Millisecond)
	s.NoteOn(1, 0, 64, 1, 0)
	x := render(s, 10*time.Millisecond)
	notes := map[float32]bool{}
	for _, v := range s.pool.voices {
		notes[v.on.note] = v.active
	}
	if !notes[60] || !notes[64] || len(notes) != 2 {
		t.Errorf("sounding notes %v, want 60 and 64 (62 stolen)", notes)
	}
	// The steal fades: no step beyond what two sines produce.
	if st := maxStep(x); st > 0.1 {
		t.Errorf("steal step %g", st)
	}

	// Nothing releasing: the oldest sounding note (60) is stolen.
	s.NoteOn(1, 0, 67, 1, 0)
	render(s, 10*time.Millisecond)
	for _, v := range s.pool.voices {
		if v.on.note == 60 {
			t.Error("oldest note 60 not stolen")
		}
	}
}

func TestReleaseCoversNotesWaitingForSteal(t *testing.T) {
	s := newTestStream(t, 1)
	setPatch(t, s, 1, 0, heldPatch)
	s.NoteOn(1, 0, 60, 1, 0)
	render(s, 5*time.Millisecond)
	s.NoteOn(1, 0, 62, 1, 0) // steals; waits for the fade
	s.Release(1)
	render(s, 100*time.Millisecond)
	if n := s.pool.sounding(); n != 0 {
		t.Errorf("%d voices still sounding", n)
	}
}

func TestReadFormats(t *testing.T) {
	for _, f := range []struct {
		format Format
		bytes  int
	}{{FormatFloat32, 8}, {FormatInt16, 4}} {
		s, err := NewStream(StreamConfig{SampleRate: testRate, Format: f.format})
		if err != nil {
			t.Fatal(err)
		}
		setPatch(t, s, 1, 0, heldPatch)
		s.SetGain(1)
		s.NoteOn(1, 0, 69, 1, 1) // hard right
		buf := make([]byte, 100*f.bytes+3)
		n, err := s.Read(buf)
		if err != nil || n != 100*f.bytes {
			t.Fatalf("format %d: Read = %d, %v; want %d bytes", f.format, n, err, 100*f.bytes)
		}
		var left, right float64
		for i := 0; i < n; i += f.bytes {
			if f.format == FormatFloat32 {
				left += math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[i:]))))
				right += math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[i+4:]))))
			} else {
				left += math.Abs(float64(int16(binary.LittleEndian.Uint16(buf[i:]))))
				right += math.Abs(float64(int16(binary.LittleEndian.Uint16(buf[i+2:]))))
			}
		}
		if left > 1e-3 || right == 0 {
			t.Errorf("format %d: left %g right %g, want sound on the right only", f.format, left, right)
		}
		if n, _ := s.Read(buf[:f.bytes-1]); n != 0 {
			t.Errorf("format %d: short Read = %d, want 0", f.format, n)
		}
	}
}

func TestReadAndRenderContinue(t *testing.T) {
	// Reading in odd chunk sizes gives the same frames as one Render.
	a, b := newTestStream(t, 4), newTestStream(t, 4)
	for _, s := range []*Stream{a, b} {
		setPatch(t, s, 1, 0, Patch{Wave: Saw, Env: Envelope{Sustain: 1}})
		s.NoteOn(1, 0, 60, 0.5, -0.3)
	}
	want := make([]float32, 1000)
	a.Render(want, make([]float32, 1000))
	buf := make([]byte, 8*1000)
	for off, chunk := 0, 1; off < len(buf); chunk = chunk*3 + 1 {
		end := min(off+chunk*8, len(buf))
		n, _ := b.Read(buf[off:end])
		off += n
	}
	for i := range want {
		if got := math.Float32frombits(binary.LittleEndian.Uint32(buf[i*8:])); got != want[i] {
			t.Fatalf("frame %d: %g, want %g", i, got, want[i])
		}
	}
}

func TestGainAndClipping(t *testing.T) {
	s := newTestStream(t, 8)
	setPatch(t, s, 1, 0, Patch{Wave: Square, Env: Envelope{Sustain: 1}})
	for i := range 6 {
		s.NoteOn(1, 0, 60+float32(i)*0.01, 1, 0)
	}
	if p := peak(render(s, 50*time.Millisecond)); p != 1 {
		t.Errorf("peak %g, want clipped to 1", p)
	}
	s.SetGain(0.01)
	// Frames of the block already rendered keep the old gain.
	if p := peak(render(s, 50*time.Millisecond)[BlockFrames:]); p > 0.05 {
		t.Errorf("peak %g with gain 0.01", p)
	}
}

func TestDeterministic(t *testing.T) {
	run := func() []float32 {
		s := newTestStream(t, 4)
		setPatch(t, s, 1, 0, Patch{Wave: Noise, Filter: Filter{Cutoff: 2000}, Env: Envelope{Decay: 50 * time.Millisecond}})
		setPatch(t, s, 1, 1, Patch{Wave: Square, Detune: 0.2, Vibrato: Vibrato{Rate: 6, Depth: 0.3}, Env: Envelope{Sustain: 1}})
		s.NoteOn(1, 0, 60, 1, 0)
		s.NoteOn(1, 1, 64, 0.7, 0.5)
		return render(s, 200*time.Millisecond)
	}
	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("frame %d differs: %g vs %g", i, a[i], b[i])
		}
	}
}

func TestConcurrentSink(t *testing.T) {
	// Sink calls from one goroutine while another reads: run with -race.
	s := newTestStream(t, 8)
	setPatch(t, s, 1, 0, Patch{Env: Envelope{Decay: 5 * time.Millisecond}})
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 2000 {
			s.NoteOn(1, 0, float32(50+i%20), 1, 0)
			if i%7 == 0 {
				s.Release(1)
			}
		}
	})
	buf := make([]byte, 8*256)
	for range 400 {
		s.Read(buf)
	}
	wg.Wait()
	s.Release(1)
	for range 100 {
		s.Read(buf)
	}
	if n := s.pool.sounding(); n != 0 {
		t.Errorf("%d voices sounding after Release", n)
	}
}

func TestAllocs(t *testing.T) {
	s := newTestStream(t, DefaultVoices)
	for inst := range voice.InstrumentCount {
		// Reverb sends, chord layers and unison are active too.
		setPatch(t, s, 1, voice.Instrument(inst), Patch{Wave: Waveform(inst % 4), Filter: Filter{Cutoff: 3000}, Env: Envelope{Decay: 40 * time.Millisecond},
			Reverb: 0.3, Layers: []float32{0, 7}, Unison: 3, Spread: 0.2})
	}
	buf := make([]byte, 8*512)
	left, right := make([]float32, 512), make([]float32, 512)
	i := 0
	step := func() {
		for range 16 {
			s.NoteOn(1, voice.Instrument(i%voice.InstrumentCount), float32(48+i%24), 0.8, 0)
			i++
		}
		s.NoteOff(1, voice.Instrument(2), 50)
		s.Release(2)
		s.Read(buf)
		s.Render(left, right)
	}
	for range 50 { // warm up: buffers grow to the busiest block
		step()
	}
	if n := testing.AllocsPerRun(200, step); n != 0 {
		t.Errorf("Sink calls, Read and Render: %g allocs per run, want 0", n)
	}
}

func TestNoteOffMatchesInstrument(t *testing.T) {
	s := newTestStream(t, 4)
	held := Patch{Env: Envelope{Sustain: 1, Release: 10 * time.Millisecond}, Gain: 0.5}
	setPatch(t, s, 1, voice.InstMotif, held)
	setPatch(t, s, 1, voice.InstPercussion, held)
	s.NoteOn(1, voice.InstMotif, 42, 1, 0)
	s.NoteOn(1, voice.InstPercussion, 42, 1, 0)
	s.NoteOff(1, voice.InstPercussion, 42) // a hit at the motif's pitch
	render(s, 100*time.Millisecond)
	if n := s.pool.sounding(); n != 1 {
		t.Errorf("%d voices sounding, want the motif note only", n)
	}
}
