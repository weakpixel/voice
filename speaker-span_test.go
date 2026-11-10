package voice_test

import (
	"testing"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/voicetest"
)

// sayWithSpan says the markup line "Hello {g}there, traveler{/}! Are you
// lost?" on owner 2, the span with gruff on owner 3.
func sayWithSpan(t testing.TB, s *voice.Speaker) string {
	t.Helper()
	text, ms, err := voice.ParseMarkup("Hello {g}there, traveler{/}! Are you lost? " + line)
	if err != nil {
		t.Fatal(err)
	}
	g, err := voice.Preset("gruff")
	if err != nil {
		t.Fatal(err)
	}
	s.Say(soft(t), text, voice.SayOptions{Spans: []voice.VoiceSpan{{Start: ms[0].Start, End: ms[0].End, Voice: g, Owner: 3}}})
	return text
}

func TestSpeakerSpanOwners(t *testing.T) {
	sink := voicetest.NewRecordingSink(0)
	s := voice.NewSpeaker(sink, 2)
	sayWithSpan(t, s)
	runAt(s, 60, time.Minute)
	ons := map[voice.Owner]int{}
	for _, c := range sink.Calls() {
		if c.Kind == voicetest.NoteOn {
			ons[c.Owner]++
		}
	}
	if ons[2] == 0 || ons[3] == 0 || len(ons) != 2 {
		t.Errorf("notes per owner %v, want owners 2 and 3", ons)
	}
	for _, o := range []voice.Owner{2, 3} {
		if n := sink.Sounding(o); len(n) != 0 {
			t.Errorf("owner %d: %d notes still sounding after the line", o, len(n))
		}
	}
}

func TestSpeakerSpanReleases(t *testing.T) {
	actions := map[string]func(s *voice.Speaker){
		"skip":    (*voice.Speaker).Skip,
		"stop":    (*voice.Speaker).Stop,
		"pause":   (*voice.Speaker).Pause,
		"close":   (*voice.Speaker).Close,
		"replace": func(s *voice.Speaker) { s.Say(soft(t), "Hi.", voice.SayOptions{}) },
	}
	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			sink := voicetest.NewRecordingSink(0)
			s := voice.NewSpeaker(sink, 2)
			sayWithSpan(t, s)
			guest := false
			for range 120 { // until the span sounds
				s.Update(time.Second / 60)
				if len(sink.Sounding(3)) > 0 {
					guest = true
					break
				}
			}
			if !guest {
				t.Fatal("the span never sounded")
			}
			action(s)
			if n := sink.Sounding(3); len(n) != 0 {
				t.Errorf("%d guest notes still sounding", len(n))
			}
			if n := sink.Sounding(2); len(n) != 0 && name != "replace" {
				t.Errorf("%d notes still sounding", len(n))
			}
		})
	}
}

func TestSpeakerSpanSameOwnerReleasedOnce(t *testing.T) {
	sink := voicetest.NewRecordingSink(0)
	s := voice.NewSpeaker(sink, 2)
	text, ms, _ := voice.ParseMarkup("{a}Hello{/} there {a}traveler{/}")
	v := soft(t)
	s.Say(v, text, voice.SayOptions{Spans: []voice.VoiceSpan{
		{Start: ms[0].Start, End: ms[0].End, Voice: v, Owner: 2},
		{Start: ms[1].Start, End: ms[1].End, Voice: v, Owner: 2},
	}})
	sink.Reset()
	s.Stop()
	if n := len(sink.Calls()); n != 1 {
		t.Errorf("%d calls on Stop, want one Release", n)
	}
}

func TestSpeakerSpanUpdateAllocs(t *testing.T) {
	sink := voicetest.NewRecordingSink(10000)
	s := voice.NewSpeaker(sink, 2)
	sayWithSpan(t, s)
	if n := testing.AllocsPerRun(200, func() { s.Update(time.Second / 144) }); n != 0 {
		t.Errorf("Update: %g allocs per run", n)
	}
	if s.Done() {
		t.Error("line finished during the alloc test; make it longer")
	}
}
