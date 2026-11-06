package session

import (
	"slices"
	"testing"
	"time"
)

func TestAuditionPlaysEveryInstrument(t *testing.T) {
	s, _ := New(rate)
	if err := s.SetVoice(preset(t, "flow")); err != nil {
		t.Fatal(err)
	}
	names := []string{"vowel", "sonorant", "fricative", "plosive", "motif"}
	for _, p := range s.Voice().Percussion {
		names = append(names, p.Name)
	}
	if !slices.Contains(names, "kick") {
		t.Fatalf("flow has no kick: %v", names)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if err := s.Audition(name); err != nil {
				t.Fatal(err)
			}
			if !s.State().Audition {
				t.Fatal("not auditioning")
			}
			if p := play(s, 800*time.Millisecond, nil); p <= 0.01 || p > 1 {
				t.Errorf("peak %g", p)
			}
			if s.State().Audition {
				t.Error("still auditioning after the phrase")
			}
			play(s, 3*time.Second, nil) // release and reverb ring out
			if p := play(s, 100*time.Millisecond, nil); p > 1e-3 {
				t.Errorf("still sounding: peak %g", p)
			}
		})
	}
}

func TestAuditionEnds(t *testing.T) {
	for _, end := range []string{"stop", "skip", "say"} {
		t.Run(end, func(t *testing.T) {
			s, _ := New(rate)
			s.SetVoice(preset(t, "divine"))
			s.Audition("motif")
			play(s, 100*time.Millisecond, nil)
			switch end {
			case "stop":
				s.Stop()
			case "skip":
				s.Skip()
			case "say":
				s.Say("Hi.", Options{})
				play(s, s.Timeline().Duration, nil)
			}
			if s.State().Audition {
				t.Error("still auditioning")
			}
			play(s, 4*time.Second, nil)
			if p := play(s, 100*time.Millisecond, nil); p > 1e-3 {
				t.Errorf("still sounding after %s: peak %g", end, p)
			}
		})
	}
}

func TestAuditionUnknown(t *testing.T) {
	s, _ := New(rate)
	if err := s.Audition("vowel"); err == nil {
		t.Error("no error without a voice")
	}
	s.SetVoice(preset(t, "soft"))
	if err := s.Audition("kick"); err == nil {
		t.Error("no error for a percussion sound the voice lacks")
	}
}

func TestAuditionUpdateAllocs(t *testing.T) {
	s, _ := New(rate)
	s.SetVoice(preset(t, "soft"))
	s.Audition("vowel")
	if a := testing.AllocsPerRun(50, func() { s.Update(time.Millisecond) }); a != 0 {
		t.Errorf("Update allocates %v during an audition", a)
	}
}

func TestPeaksAndWAV(t *testing.T) {
	s, _ := New(rate)
	s.SetVoice(preset(t, "soft"))
	lr, err := s.StartRender("Hello there!", Options{})
	if err != nil {
		t.Fatal(err)
	}
	steps := 0
	for !lr.Step(10) {
		steps++
	}
	l, r, tl := lr.Result()
	if steps < 10 {
		t.Errorf("rendered in %d steps of 10 blocks", steps)
	}
	whole, _, _, _ := RenderLine(s.Voice(), nil, "Hello there!", Options{}, rate)
	if !slices.Equal(l, whole) {
		t.Errorf("stepped render differs from RenderLine: %d and %d frames", len(l), len(whole))
	}
	if len(tl.Notes) == 0 || float64(len(l))/rate < tl.Duration.Seconds() {
		t.Errorf("%d notes, render %d frames for %v", len(tl.Notes), len(l), tl.Duration)
	}
	p := Peaks(l, r, 100)
	if len(p) != 200 {
		t.Fatalf("%d values", len(p))
	}
	var loud float32
	for i := 0; i < len(p); i += 2 {
		if p[i] > p[i+1] {
			t.Fatalf("column %d: min %g above max %g", i/2, p[i], p[i+1])
		}
		loud = max(loud, p[i+1])
	}
	if loud < 0.05 {
		t.Errorf("waveform is flat: %g", loud)
	}
	if got := Peaks(nil, nil, 3); len(got) != 6 || slices.ContainsFunc(got, func(v float32) bool { return v != 0 }) {
		t.Errorf("empty render: %v", got)
	}
	data, err := WAV(l, r, rate)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || len(data) < 8*len(l) {
		t.Errorf("not a WAV of the render: %d bytes for %d frames", len(data), len(l))
	}
}
