package voice_test

import (
	"slices"
	"testing"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/voicetest"
)

const line = "Hello there, traveler! Are you lost?"

func soft(t testing.TB) *voice.Voice {
	t.Helper()
	v, err := voice.Preset("soft")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// runAt updates s at hz until done (or limit) and returns the reveal after
// every update with its time.
type sample struct {
	at       time.Duration
	revealed int
}

func runAt(s *voice.Speaker, hz int, limit time.Duration) []sample {
	dt := time.Second / time.Duration(hz)
	var out []sample
	for t := dt; t <= limit && !s.Done(); t += dt {
		s.Update(dt)
		out = append(out, sample{t, s.Revealed()})
	}
	return out
}

func kinds(evs []voice.Event) []voice.EventKind {
	var out []voice.EventKind
	for _, e := range evs {
		out = append(out, e.Kind)
	}
	return out
}

func TestSpeakerCompletes(t *testing.T) {
	sink := voicetest.NewRecordingSink(0)
	s := voice.NewSpeaker(sink, 7)
	if !s.Done() || s.Revealed() != 0 {
		t.Fatal("new speaker not done")
	}
	s.Say(soft(t), line, voice.SayOptions{})
	if got := kinds(s.Events()); !slices.Equal(got, []voice.EventKind{voice.Started}) {
		t.Errorf("after Say: %v", got)
	}
	if s.Revealed() == 0 {
		t.Error("the first token is not revealed at once")
	}
	var all []voice.Event
	prev := 0
	for range 1000 {
		s.Update(time.Second / 60)
		all = append(all, s.Events()...)
		if s.Revealed() < prev {
			t.Fatalf("reveal went back from %d to %d", prev, s.Revealed())
		}
		prev = s.Revealed()
		if s.Done() {
			break
		}
	}
	if got := kinds(all); !slices.Equal(got, []voice.EventKind{voice.Started, voice.Completed}) {
		t.Errorf("events %v", got)
	}
	if s.Revealed() != len(line) {
		t.Errorf("revealed %d of %d", s.Revealed(), len(line))
	}
	if n := sink.Sounding(7); len(n) != 0 {
		t.Errorf("%d notes still sounding: %+v", len(n), n)
	}
	ons := 0
	for _, c := range sink.Calls() {
		if c.Owner != 7 {
			t.Errorf("call for owner %d", c.Owner)
		}
		if c.Kind == voicetest.NoteOn {
			ons++
		}
	}
	if ons != len(s.Timeline().Notes) {
		t.Errorf("%d note ons, timeline has %d notes", ons, len(s.Timeline().Notes))
	}
	s.Update(time.Second)
	if len(s.Events()) != 0 {
		t.Errorf("events after the line: %v", s.Events())
	}
}

func TestSpeakerRevealIndependentOfRate(t *testing.T) {
	// At every moment both rates have seen, the reveal is the same.
	reveal := func(hz int) map[time.Duration]int {
		s := voice.NewSpeaker(voicetest.NewRecordingSink(0), 1)
		s.Say(soft(t), line, voice.SayOptions{})
		m := map[time.Duration]int{}
		for _, smp := range runAt(s, hz, 10*time.Second) {
			m[smp.at] = smp.revealed
		}
		return m
	}
	a, b := reveal(50), reveal(200) // whole milliseconds, so the update times coincide
	common := 0
	for at, r := range a {
		if rb, ok := b[at]; ok {
			common++
			if r != rb {
				t.Errorf("at %v: %d at 50 Hz, %d at 200 Hz", at, r, rb)
			}
		}
	}
	if common < 20 {
		t.Errorf("only %d common times", common)
	}

	// At 30 and 144 Hz the reveal at any update time is the timeline's
	// reveal at that time.
	for _, hz := range []int{30, 144} {
		s := voice.NewSpeaker(voicetest.NewRecordingSink(0), 1)
		s.Say(soft(t), line, voice.SayOptions{})
		checkReveal(t, s.Timeline(), runAt(s, hz, 10*time.Second))
	}
}

func checkReveal(t *testing.T, tl voice.Timeline, samples []sample) {
	t.Helper()
	for _, smp := range samples {
		want := 0
		for _, r := range tl.Reveals {
			if r.At <= smp.at {
				want = r.Offset
			}
		}
		if smp.revealed != want && smp.at < tl.Duration {
			t.Fatalf("at %v: revealed %d, timeline says %d", smp.at, smp.revealed, want)
		}
	}
}

func TestSpeakerSkipStopReplacePause(t *testing.T) {
	tests := []struct {
		name   string
		action func(s *voice.Speaker, v *voice.Voice)
		want   []voice.EventKind
		all    bool // whole text revealed
		done   bool
	}{
		{"skip", func(s *voice.Speaker, _ *voice.Voice) { s.Skip() }, []voice.EventKind{voice.Started, voice.Skipped}, true, true},
		{"stop", func(s *voice.Speaker, _ *voice.Voice) { s.Stop() }, []voice.EventKind{voice.Started, voice.Stopped}, false, true},
		{"close", func(s *voice.Speaker, v *voice.Voice) {
			s.Close()
			s.Say(v, "ignored", voice.SayOptions{})
		}, []voice.EventKind{voice.Started, voice.Stopped}, false, true},
		{"replace", func(s *voice.Speaker, v *voice.Voice) { s.Say(v, "Hi.", voice.SayOptions{}) },
			[]voice.EventKind{voice.Started, voice.Stopped, voice.Started}, false, false},
		{"pause", func(s *voice.Speaker, _ *voice.Voice) { s.Pause() }, []voice.EventKind{voice.Started}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := voicetest.NewRecordingSink(0)
			s := voice.NewSpeaker(sink, 2)
			v := soft(t)
			s.Say(v, line, voice.SayOptions{})
			var evs []voice.Event
			for range 20 { // 333 ms in: notes are sounding
				s.Update(time.Second / 60)
				evs = append(evs, s.Events()...)
			}
			before := s.Revealed()
			tt.action(s, v)
			evs = append(evs, s.Events()...)
			if got := kinds(dedupe(evs)); !slices.Equal(got, tt.want) {
				t.Errorf("events %v, want %v", got, tt.want)
			}
			if n := sink.Sounding(2); len(n) != 0 && tt.name != "replace" {
				t.Errorf("%d notes still sounding", len(n))
			}
			if tt.all && s.Revealed() != len(line) {
				t.Errorf("revealed %d of %d", s.Revealed(), len(line))
			}
			if !tt.all && tt.name != "replace" && s.Revealed() != before {
				t.Errorf("revealed changed from %d to %d", before, s.Revealed())
			}
			if s.Done() != tt.done {
				t.Errorf("Done() = %v", s.Done())
			}
		})
	}
}

// dedupe drops events seen twice because Events keeps the actions' events
// until the next Update.
func dedupe(evs []voice.Event) []voice.Event {
	var out []voice.Event
	for _, e := range evs {
		if len(out) == 0 || out[len(out)-1] != e {
			out = append(out, e)
		}
	}
	return out
}

func TestSpeakerPauseResume(t *testing.T) {
	sink := voicetest.NewRecordingSink(0)
	s := voice.NewSpeaker(sink, 1)
	s.Say(soft(t), line, voice.SayOptions{})
	s.Update(200 * time.Millisecond)
	s.Pause()
	if !s.Paused() || len(sink.Sounding(1)) != 0 {
		t.Fatal("pause did not release")
	}
	r := s.Revealed()
	calls := len(sink.Calls())
	s.Update(time.Second)
	if s.Revealed() != r || len(sink.Calls()) != calls {
		t.Error("paused speaker advanced")
	}
	s.Resume()
	runAt(s, 60, 20*time.Second)
	if !s.Done() || s.Revealed() != len(line) || len(sink.Sounding(1)) != 0 {
		t.Errorf("after resume: done %v revealed %d sounding %d", s.Done(), s.Revealed(), len(sink.Sounding(1)))
	}
}

func TestSpeakerInstantAndSpeed(t *testing.T) {
	sink := voicetest.NewRecordingSink(0)
	s := voice.NewSpeaker(sink, 1)
	s.Say(soft(t), line, voice.SayOptions{Instant: true})
	if s.Revealed() != len(line) {
		t.Errorf("instant: revealed %d", s.Revealed())
	}
	runAt(s, 60, 20*time.Second)
	for _, c := range sink.Calls() {
		if c.Kind == voicetest.NoteOn && c.Instrument != voice.InstMotif {
			t.Fatalf("instant line played %v", c.Instrument)
		}
	}
	if len(sink.Calls()) == 0 {
		t.Error("instant line played no motif")
	}

	duration := func(speed float32) time.Duration {
		s := voice.NewSpeaker(voicetest.NewRecordingSink(0), 1)
		s.Say(soft(t), line, voice.SayOptions{Speed: speed})
		smp := runAt(s, 100, 20*time.Second)
		return smp[len(smp)-1].at
	}
	normal, fast := duration(0), duration(2)
	if d := fast - normal/2; d < -20*time.Millisecond || d > 20*time.Millisecond {
		t.Errorf("speed 2 took %v, normal %v", fast, normal)
	}
}

func TestSpeakerEmptyLine(t *testing.T) {
	s := voice.NewSpeaker(voicetest.NewRecordingSink(0), 1)
	s.Say(soft(t), "", voice.SayOptions{})
	s.Update(time.Millisecond)
	if !s.Done() || kinds(s.Events())[len(s.Events())-1] != voice.Completed {
		t.Errorf("empty line: done %v events %v", s.Done(), s.Events())
	}
}

func TestSpeakerUpdateAllocs(t *testing.T) {
	for _, name := range []string{"soft", "flow"} { // flow: rhythm and percussion
		sink := voicetest.NewRecordingSink(10000)
		s := voice.NewSpeaker(sink, 1)
		v, err := voice.Preset(name)
		if err != nil {
			t.Fatal(err)
		}
		s.Say(v, line+" "+line+" "+line, voice.SayOptions{})
		if n := testing.AllocsPerRun(200, func() { s.Update(time.Second / 144) }); n != 0 {
			t.Errorf("%s: Update: %g allocs per run", name, n)
		}
		if s.Done() {
			t.Errorf("%s: line finished during the alloc test; make it longer", name)
		}
	}
}

func BenchmarkSpeakerUpdate(b *testing.B) {
	v := soft(b)
	s := voice.NewSpeaker(nopSink{}, 1)
	b.ReportAllocs()
	for b.Loop() {
		if s.Done() {
			s.Say(v, line, voice.SayOptions{})
		}
		s.Update(time.Second / 60)
	}
}

func BenchmarkSay(b *testing.B) {
	v := soft(b)
	s := voice.NewSpeaker(nopSink{}, 1)
	b.ReportAllocs()
	for b.Loop() {
		s.Say(v, line, voice.SayOptions{})
	}
}

type nopSink struct{}

func (nopSink) NoteOn(voice.Owner, voice.Instrument, float32, float32, float32) {}
func (nopSink) NoteOff(voice.Owner, voice.Instrument, float32)                  {}
func (nopSink) Release(voice.Owner)                                             {}

func TestSpeakerRhythm(t *testing.T) {
	v, err := voice.Preset("flow")
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"skip", "stop"} {
		sink := voicetest.NewRecordingSink(0)
		s := voice.NewSpeaker(sink, 1)
		s.Say(v, line, voice.SayOptions{})
		runFor(s, 3*time.Second) // past the count-in: voice and beat sound
		if action == "skip" {
			s.Skip()
		} else {
			s.Stop()
		}
		if n := sink.Sounding(1); len(n) != 0 {
			t.Errorf("%s: %d notes still sounding", action, len(n))
		}
		percussion := false
		for _, c := range sink.Calls() {
			percussion = percussion || c.Kind == voicetest.NoteOn && c.Instrument >= voice.InstPercussion
		}
		if !percussion {
			t.Errorf("%s: no percussion played", action)
		}
	}
	// A clock keeps the music's tempo: Speed is ignored.
	duration := func(opts voice.SayOptions) time.Duration {
		s := voice.NewSpeaker(voicetest.NewRecordingSink(0), 1)
		s.Say(v, line, opts)
		smp := runAt(s, 100, 60*time.Second)
		return smp[len(smp)-1].at
	}
	clock := &voice.Clock{BPM: 92}
	if a, b := duration(voice.SayOptions{Clock: clock}), duration(voice.SayOptions{Clock: clock, Speed: 2}); a != b {
		t.Errorf("with a clock, speed 2 took %v, normal %v", b, a)
	}
}

func runFor(s *voice.Speaker, d time.Duration) {
	for t := time.Duration(0); t < d; t += time.Second / 60 {
		s.Update(time.Second / 60)
	}
}

func BenchmarkSayRhythm(b *testing.B) {
	v, err := voice.Preset("flow")
	if err != nil {
		b.Fatal(err)
	}
	s := voice.NewSpeaker(nopSink{}, 1)
	b.ReportAllocs()
	for b.Loop() {
		s.Say(v, line, voice.SayOptions{})
	}
}
