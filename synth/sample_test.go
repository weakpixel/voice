package synth

import (
	"math"
	"strings"
	"testing"
	"time"
)

// sineSample is a 0.5 s sine of hz at rate, tagged with root.
func sineSample(hz float64, rate int, root float32) *Sample {
	d := make([]float32, rate/2)
	for i := range d {
		d[i] = float32(math.Sin(2 * math.Pi * hz * float64(i) / float64(rate)))
	}
	return &Sample{Data: d, Rate: rate, Root: root}
}

func TestSamplePitch(t *testing.T) {
	// A 440 Hz sample at 22050 Hz with root 69, played at 48 kHz.
	smp := sineSample(440, 22050, 69)
	for _, note := range []float32{69, 81, 57, 64} {
		x := playNote(t, Patch{Sample: smp, Env: Envelope{Sustain: 1}}, note, 200*time.Millisecond)
		got, want := pitch(x[frames(5*time.Millisecond):frames(150*time.Millisecond)]), noteHz(float64(note))
		if math.Abs(got-want)/want > 0.005 {
			t.Errorf("note %g: pitch %.2f Hz, want %.2f Hz", note, got, want)
		}
	}
}

func TestSampleOneShotEnds(t *testing.T) {
	// 0.5 s of sample at its root lasts 0.5 s, ends without a click and
	// frees the voice.
	s := newTestStream(t, 2)
	setPatch(t, s, 1, 0, Patch{Sample: sineSample(440, 48000, 69), Env: Envelope{Sustain: 1}})
	s.NoteOn(1, 0, 69, 1, 0)
	x := render(s, 600*time.Millisecond)
	if p := peak(x[frames(510*time.Millisecond):]); p != 0 {
		t.Errorf("sounding after the sample's end: %g", p)
	}
	end := frames(500 * time.Millisecond)
	if st := maxStep(x[end-200 : end+10]); st > 0.07 {
		t.Errorf("end step %g", st)
	}
	if n := s.pool.sounding(); n != 0 {
		t.Errorf("%d voices still sounding", n)
	}
}

func TestSampleLoop(t *testing.T) {
	smp := sineSample(440, 48000, 69)
	smp.Data = smp.Data[:48000/440*20] // 20 whole cycles
	smp.Loop = true
	x := playNote(t, Patch{Sample: smp, Env: Envelope{Sustain: 1}}, 69, time.Second)
	tail := x[frames(900*time.Millisecond):]
	// A centred note is 1/√2 per channel, so a full sine has rms 0.5.
	if r := rms(tail); r < 0.45 {
		t.Errorf("looped sample rms %g after 0.9 s, want a sounding sine", r)
	}
	if p := pitch(tail); math.Abs(p-440) > 6 {
		t.Errorf("looped pitch %.1f Hz", p)
	}
}

func TestPatchValidate(t *testing.T) {
	ok := []Patch{
		{},
		{Wave: Square, PulseWidth: 0.3, Detune: 0.1, Env: Envelope{Attack: time.Millisecond, Sustain: 1}, Filter: Filter{Cutoff: 1000, Resonance: 0.5}},
		{Sample: &Sample{Data: []float32{0, 1}, Rate: 8000, Loop: true}},
	}
	for i, p := range ok {
		if err := p.Validate(); err != nil {
			t.Errorf("ok[%d]: %v", i, err)
		}
	}
	bad := Patch{
		Wave:       Waveform(9),
		PulseWidth: 1.5,
		Env:        Envelope{Attack: -1, Sustain: 2},
		Filter:     Filter{Cutoff: -1, Resonance: 1},
		Gain:       float32(math.NaN()),
		Sample:     &Sample{Rate: 0, Loop: true, LoopStart: 3},
	}
	err := bad.Validate()
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"wave", "pulseWidth", "env.attack", "env.sustain", "filter.cutoff", "filter.resonance", "gain", "sample.data", "sample.rate", "sample.loop"} {
		if !strings.Contains(err.Error(), " "+want+":") {
			t.Errorf("error does not mention %s:\n%v", want, err)
		}
	}
}
