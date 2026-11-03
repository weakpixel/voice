package synth

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/voice"
)

// tailOf plays a 20 ms blip with send and returns the left channel of the
// following span.
func tailOf(t *testing.T, r Reverb, send float32, cutoff float32, span time.Duration) []float32 {
	t.Helper()
	s := newTestStream(t, 4)
	if err := s.SetReverb(r); err != nil {
		t.Fatal(err)
	}
	setPatch(t, s, 1, 0, Patch{Wave: Saw, Reverb: send, Filter: Filter{Cutoff: cutoff}, Env: Envelope{Decay: 20 * time.Millisecond}})
	s.NoteOn(1, 0, 69, 1, 0)
	render(s, 30*time.Millisecond)
	return render(s, span)
}

func TestReverbTail(t *testing.T) {
	dry := tailOf(t, DefaultReverb, 0, 0, 200*time.Millisecond)
	if p := peak(dry); p != 0 {
		t.Errorf("send 0: tail peak %g, want silence", p)
	}
	wet := tailOf(t, DefaultReverb, 1, 0, 2*time.Second)
	early, late := rms(wet[:frames(200*time.Millisecond)]), rms(wet[frames(1500*time.Millisecond):])
	if early < 0.005 || late >= early {
		t.Errorf("tail rms early %g late %g, want a decaying tail", early, late)
	}
	small := tailOf(t, Reverb{Size: 0.1, Damping: 0.4, Width: 1}, 1, 0, 2*time.Second)
	big := tailOf(t, Reverb{Size: 1, Damping: 0.4, Width: 1}, 1, 0, 2*time.Second)
	at := frames(time.Second)
	if rms(big[at:]) <= 4*rms(small[at:]) {
		t.Errorf("size 1 tail %g, size 0.1 tail %g: bigger rooms ring longer", rms(big[at:]), rms(small[at:]))
	}
	for i, x := range big {
		if math.IsNaN(float64(x)) || math.Abs(float64(x)) > 1 {
			t.Fatalf("frame %d: %g", i, x)
		}
	}
}

func TestReverbDamping(t *testing.T) {
	// High-frequency content of the tail (frame-to-frame differences) falls
	// with damping.
	hf := func(d float32) float64 {
		x := tailOf(t, Reverb{Size: 0.8, Damping: d, Width: 1}, 1, 0, time.Second)[frames(300*time.Millisecond):]
		diff := make([]float32, len(x)-1)
		for i := range diff {
			diff[i] = x[i+1] - x[i]
		}
		return rms(diff) / rms(x)
	}
	if bright, dark := hf(0), hf(1); dark >= bright*0.8 {
		t.Errorf("damping 1: hf ratio %g, damping 0: %g", dark, bright)
	}
}

func TestReverbStopsWhenSilent(t *testing.T) {
	s := newTestStream(t, 4)
	setPatch(t, s, 1, 0, Patch{Reverb: 1, Env: Envelope{Decay: 10 * time.Millisecond}})
	s.NoteOn(1, 0, 60, 1, 0)
	render(s, 20*time.Second)
	if s.reverb.active {
		t.Error("reverb still running 20 s after its only note")
	}
	if p := peak(render(s, 100*time.Millisecond)); p != 0 {
		t.Errorf("output %g after the tail", p)
	}
}

func TestSetReverbErrors(t *testing.T) {
	s := newTestStream(t, 1)
	err := s.SetReverb(Reverb{Size: 2, Damping: -1, Width: float32(math.NaN())})
	if err == nil {
		t.Fatal("no error")
	}
	for _, w := range []string{"size: 2", "damping: -1", "width: NaN"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error does not mention %q: %v", w, err)
		}
	}
}

func TestLayers(t *testing.T) {
	// One layer an octave up plays at twice the frequency.
	x := playNote(t, Patch{Layers: []float32{12}, Env: Envelope{Sustain: 1}}, 57, 300*time.Millisecond)
	if p := pitch(x[frames(20*time.Millisecond):]); math.Abs(p-440) > 2 {
		t.Errorf("layer +12 on 57: %.1f Hz, want 440", p)
	}

	// A three-note chord takes three voices, is about as loud as one note,
	// and ends with one NoteOff of the played note.
	s := newTestStream(t, 8)
	setPatch(t, s, 1, 0, Patch{Layers: []float32{0, -12, 7}, Env: Envelope{Sustain: 1, Release: 10 * time.Millisecond}})
	setPatch(t, s, 2, 0, Patch{Env: Envelope{Sustain: 1}})
	s.NoteOn(1, 0, 60, 1, 0)
	chord := render(s, 200*time.Millisecond)
	if n := s.pool.sounding(); n != 3 {
		t.Errorf("%d voices for a 3-layer chord", n)
	}
	single := playNote(t, Patch{Env: Envelope{Sustain: 1}}, 60, 200*time.Millisecond)
	if r := rms(chord[frames(20*time.Millisecond):]) / rms(single[frames(20*time.Millisecond):]); r < 0.7 || r > 1.4 {
		t.Errorf("chord/single loudness %.2f, want about 1", r)
	}
	s.NoteOff(1, voice.InstVowel, 60)
	render(s, 50*time.Millisecond)
	if n := s.pool.sounding(); n != 0 {
		t.Errorf("%d layer voices survive the note off", n)
	}
	s.NoteOn(1, 0, 60, 1, 0)
	render(s, 10*time.Millisecond)
	s.Release(1)
	render(s, 50*time.Millisecond)
	if n := s.pool.sounding(); n != 0 {
		t.Errorf("%d layer voices survive Release", n)
	}
}

func TestUnison(t *testing.T) {
	plain := playNote(t, Patch{Wave: Saw, Env: Envelope{Sustain: 1}}, 57, time.Second)
	wide := playNote(t, Patch{Wave: Saw, Unison: 5, Spread: 0.3, Env: Envelope{Sustain: 1}}, 57, time.Second)
	steady := frames(20 * time.Millisecond)
	// The copies beat against each other: the level over 50 ms windows
	// varies far more than for one oscillator.
	vary := func(x []float32) float64 {
		w := frames(50 * time.Millisecond)
		lo, hi := math.Inf(1), 0.0
		for i := steady; i+w <= len(x); i += w {
			r := rms(x[i : i+w])
			lo, hi = math.Min(lo, r), math.Max(hi, r)
		}
		return hi / lo
	}
	if vp, vw := vary(plain), vary(wide); vw < vp*1.2 {
		t.Errorf("level variation: unison %.2f, single %.2f; want the unison to beat", vw, vp)
	}
	if p := peak(wide); p > 1 {
		t.Errorf("unison peak %g", p)
	}
	// A unison of identical copies (no spread) keeps the pitch.
	same := playNote(t, Patch{Wave: Sine, Unison: 3, Env: Envelope{Sustain: 1}}, 69, 300*time.Millisecond)
	if p := pitch(same[steady:]); math.Abs(p-440) > 2 {
		t.Errorf("unison pitch %.1f Hz", p)
	}
}

func TestLayerUnisonValidate(t *testing.T) {
	bad := Patch{Layers: []float32{0, 1, 2, 3, 40}, Unison: 9, Spread: -1, Reverb: 2}
	err := bad.Validate()
	for _, w := range []string{"layers: 5 layers", "layers[4]: 40", "unison: 9", "spread: -1", "reverb: 2"} {
		if err == nil || !strings.Contains(err.Error(), w) {
			t.Errorf("error does not mention %q: %v", w, err)
		}
	}
	smp := Patch{Sample: &Sample{Data: []float32{0, 1}, Rate: 8000}, Unison: 3}
	if err := smp.Validate(); err == nil || !strings.Contains(err.Error(), "needs an oscillator") {
		t.Errorf("unison on a sample: %v", err)
	}
}

func TestSetPatchCopiesLayers(t *testing.T) {
	s := newTestStream(t, 4)
	layers := []float32{12}
	setPatch(t, s, 1, voice.InstVowel, Patch{Layers: layers, Env: Envelope{Sustain: 1}})
	layers[0] = 0 // the caller reuses its slice
	s.NoteOn(1, voice.InstVowel, 57, 1, 0)
	if p := pitch(render(s, 300*time.Millisecond)[frames(20*time.Millisecond):]); math.Abs(p-440) > 2 {
		t.Errorf("pitch %.1f Hz: SetPatch kept the caller's slice", p)
	}
}
