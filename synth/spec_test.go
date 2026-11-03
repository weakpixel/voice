package synth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/voice"
)

func TestPatchFromSpec(t *testing.T) {
	p, err := PatchFromSpec(voice.InstrumentSpec{
		Wave: "square", PulseWidth: 0.3, AttackMs: 2.5, DecayMs: 40, Sustain: 0.5, ReleaseMs: 20,
		GlideFrom: 3, GlideMs: 15, VibratoRate: 6, VibratoDepth: 0.2, VibratoDelayMs: 50, Cutoff: 2000, Resonance: 0.4, Gain: 0.5,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Patch{
		Wave: Square, PulseWidth: 0.3,
		Env:     Envelope{Attack: 2500 * time.Microsecond, Decay: 40 * time.Millisecond, Sustain: 0.5, Release: 20 * time.Millisecond},
		Glide:   Glide{From: 3, Time: 15 * time.Millisecond},
		Vibrato: Vibrato{Rate: 6, Depth: 0.2, Delay: 50 * time.Millisecond},
		Filter:  Filter{Cutoff: 2000, Resonance: 0.4}, Gain: 0.5,
	}
	if p != want {
		t.Errorf("got %+v\nwant %+v", p, want)
	}
}

func TestPatchFromSpecSample(t *testing.T) {
	smp := &Sample{Data: make([]float32, 100), Rate: 22050}
	calls := 0
	load := func(name string) (*Sample, error) {
		calls++
		if name != "blip.wav" {
			return nil, errors.New("not found")
		}
		return smp, nil
	}
	p, err := PatchFromSpec(voice.InstrumentSpec{Sample: "blip.wav", Root: 72, Loop: true, LoopStart: 10}, load)
	if err != nil {
		t.Fatal(err)
	}
	if p.Sample.Root != 72 || !p.Sample.Loop || p.Sample.LoopStart != 10 || smp.Root != 0 {
		t.Errorf("sample %+v (loaded sample changed: %v)", p.Sample, smp.Root != 0)
	}
	if p, _ := PatchFromSpec(voice.InstrumentSpec{Sample: "blip.wav"}, load); p.Sample.Root != 60 {
		t.Errorf("default root %g", p.Sample.Root)
	}
	if _, err := PatchFromSpec(voice.InstrumentSpec{Sample: "gone.wav"}, load); err == nil || !strings.Contains(err.Error(), `sample "gone.wav": not found`) {
		t.Errorf("missing sample: %v", err)
	}
	if _, err := PatchFromSpec(voice.InstrumentSpec{Sample: "blip.wav"}, nil); err == nil {
		t.Error("sample without a loader accepted")
	}
	if _, err := PatchFromSpec(voice.InstrumentSpec{Sample: "blip.wav", Loop: true, LoopStart: 500}, load); err == nil {
		t.Error("loop outside the sample accepted")
	}
}

func TestSetVoicePresets(t *testing.T) {
	for _, name := range voice.PresetNames() {
		v, err := voice.Preset(name)
		if err != nil {
			t.Fatal(err)
		}
		s := newTestStream(t, 8)
		if err := s.SetVoice(3, v, nil); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for inst := range voice.Instrument(voice.InstrumentCount) {
			s.NoteOn(3, inst, v.BaseNote, 0.8, 0)
			x := render(s, 50*time.Millisecond)
			s.Release(3)
			render(s, time.Second)
			if p := peak(x); p < 0.05 || p >= 1 {
				t.Errorf("%s %s: peak %g", name, inst, p)
			}
		}
	}
}

func TestSetVoiceErrors(t *testing.T) {
	v, _ := voice.Preset("soft")
	v.Instruments[voice.InstPlosive].Wave = "buzz"
	v.Instruments[voice.InstMotif] = voice.InstrumentSpec{Sample: "x.wav"}
	s := newTestStream(t, 2)
	err := s.SetVoice(1, v, nil)
	if err == nil || !strings.Contains(err.Error(), "plosive: synth: unknown wave") || !strings.Contains(err.Error(), "motif: synth: sample") {
		t.Errorf("error %v", err)
	}
	s.NoteOn(1, voice.InstVowel, 60, 1, 0)
	if p := peak(render(s, 10*time.Millisecond)); p != 0 {
		t.Error("a failed SetVoice set patches")
	}
}
