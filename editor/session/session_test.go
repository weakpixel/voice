package session

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/voice"
)

const rate = 48000

func preset(t *testing.T, name string) *voice.Voice {
	t.Helper()
	v, err := voice.Preset(name)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLoadVoiceProblems(t *testing.T) {
	cases := []struct {
		name, doc string
		want      []Problem
	}{
		{"fields", `{"version": 0, "speed": 100, "instruments": {"vowel": {"sustain": 2}}}`, []Problem{
			{"instruments.vowel.sustain", "2 is outside 0 to 1"}, {"speed", "100 is outside 1 to 60"}}},
		{"syntax", "{\n  \"version\": 0,\n  \"speed\": ,\n}", []Problem{
			{"", "line 3, column 12: invalid character ',' looking for beginning of value"}}},
		{"unknown field", `{"version": 0, "sped": 3, "instruments": {"vowel": {}}}`, []Problem{
			{"", `json: unknown field "sped"`}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, got := LoadVoice(c.doc)
			if v != nil {
				t.Fatal("loaded")
			}
			slices.SortFunc(got, func(a, b Problem) int { return strings.Compare(a.Path, b.Path) })
			if !slices.Equal(got, c.want) {
				t.Errorf("got %q\nwant %q", got, c.want)
			}
		})
	}
	if v, p := LoadVoice(`{"version": 0, "instruments": {"vowel": {}}}`); v == nil || p != nil {
		t.Errorf("valid voice: %v %v", v, p)
	}
}

func TestLoadLanguage(t *testing.T) {
	if l, p := LoadLanguage("  "); l != voice.DefaultLanguage() || p != nil {
		t.Error("blank document is not the default language")
	}
	_, p := LoadLanguage(`{"version": 0, "phonemes": [{"symbol": "a", "category": "tone"}]}`)
	if len(p) != 1 || p[0].Path != "phonemes[0].category" {
		t.Errorf("problems %q", p)
	}
}

// player reads the session's stream in chunks of 50 ms, as oto does with
// the editor's buffer, and advances the speaker by the same time.
func play(s *Session, d time.Duration, each func()) (peak float32) {
	const chunk = 50 * time.Millisecond
	buf := make([]byte, 8*rate*int(chunk)/int(time.Second))
	for elapsed := time.Duration(0); elapsed < d; elapsed += chunk {
		s.Update(chunk)
		if each != nil {
			each()
		}
		s.Reader().Read(buf)
		peak = max(peak, s.Peak())
	}
	return peak
}

func TestSessionPlaysEveryPreset(t *testing.T) {
	const text = "Hello there, traveler! Are you lost?"
	for _, name := range voice.PresetNames() {
		t.Run(name, func(t *testing.T) {
			s, err := New(rate)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.SetVoice(preset(t, name)); err != nil {
				t.Fatal(err)
			}
			if err := s.Say(text, Options{}); err != nil {
				t.Fatal(err)
			}
			var kinds []voice.EventKind
			var lastTime time.Duration
			peak := play(s, s.Timeline().Duration+200*time.Millisecond, func() {
				st := s.State()
				for _, e := range st.Events {
					kinds = append(kinds, e.Kind)
				}
				if st.Time < lastTime {
					t.Errorf("time went back: %v after %v", st.Time, lastTime)
				}
				lastTime = st.Time
			})
			st := s.State()
			if !st.Done || st.Revealed != len(text) {
				t.Errorf("done %v, revealed %d of %d", st.Done, st.Revealed, len(text))
			}
			if !slices.Equal(kinds, []voice.EventKind{voice.Started, voice.Completed}) {
				t.Errorf("events %v", kinds)
			}
			if st.Time != s.Timeline().Duration {
				t.Errorf("time %v, duration %v", st.Time, s.Timeline().Duration)
			}
			if peak <= 0.01 || peak > 1 {
				t.Errorf("peak %g", peak)
			}
		})
	}
}

func TestSessionStopSilences(t *testing.T) {
	for _, end := range []string{"stop", "skip", "pause"} {
		t.Run(end, func(t *testing.T) {
			s, _ := New(rate)
			s.SetVoice(preset(t, "soft"))
			s.Say("Hello there, traveler! Are you lost? The village is past the old mill.", Options{})
			play(s, 500*time.Millisecond, nil)
			map[string]func(){"stop": s.Stop, "skip": s.Skip, "pause": s.Pause}[end]()
			play(s, 3*time.Second, nil) // release and reverb ring out
			if p := play(s, 100*time.Millisecond, nil); p > 1e-3 {
				t.Errorf("still sounding after %s: peak %g", end, p)
			}
		})
	}
}

func TestSessionVoiceChangeDuringLine(t *testing.T) {
	s, _ := New(rate)
	s.SetVoice(preset(t, "soft"))
	s.Say("Hello there, traveler!", Options{})
	play(s, 300*time.Millisecond, nil)
	if err := s.SetVoice(preset(t, "gruff")); err != nil {
		t.Fatal(err)
	}
	play(s, 3*time.Second, nil)
	if st := s.State(); !st.Done || st.Revealed != len("Hello there, traveler!") {
		t.Errorf("line did not finish after the voice changed: %+v", st)
	}
	bad := preset(t, "soft")
	bad.Instruments[voice.InstVowel].Sample = "missing.wav"
	bad.Instruments[voice.InstVowel].Wave = ""
	if err := s.SetVoice(bad); err == nil {
		t.Error("a sample without a loader was accepted")
	}
	if s.Voice().Name != "gruff" {
		t.Error("a failed SetVoice changed the voice")
	}
}

func TestSessionSpeed(t *testing.T) {
	s, _ := New(rate)
	s.SetVoice(preset(t, "soft"))
	s.Say("Hello there.", Options{Speed: 2})
	s.Update(100 * time.Millisecond)
	if got := s.State().Time; got != 200*time.Millisecond {
		t.Errorf("time %v at speed 2 after 100 ms", got)
	}
}

func TestSessionUpdateAllocs(t *testing.T) {
	s, _ := New(rate)
	s.SetVoice(preset(t, "soft"))
	s.Say(strings.Repeat("Hello there, traveler! ", 50), Options{})
	allocs := testing.AllocsPerRun(200, func() {
		s.Update(time.Second / 60)
		_ = s.State()
	})
	if allocs != 0 {
		t.Errorf("Update and State allocate %g times", allocs)
	}
	buf := make([]byte, 8*1024)
	if a := testing.AllocsPerRun(50, func() { s.Reader().Read(buf); s.Peak() }); a != 0 {
		t.Errorf("Read through the meter allocates %g times", a)
	}
}

func TestRenderLine(t *testing.T) {
	const text = "Hello there, traveler!"
	for _, name := range voice.PresetNames() {
		t.Run(name, func(t *testing.T) {
			v := preset(t, name)
			left, right, tl, err := RenderLine(v, nil, text, Options{}, rate)
			if err != nil {
				t.Fatal(err)
			}
			n, dur := len(left), frames(tl.Duration, rate)
			if len(right) != n || n < dur || n > dur+frames(maxTail, rate)+2*64 {
				t.Errorf("%d frames for a line of %d", n, dur)
			}
			var peak float32
			for i := range left {
				x := max(abs32(left[i]), abs32(right[i]))
				if math.IsNaN(float64(x)) || x > 1 {
					t.Fatalf("frame %d: %g", i, x)
				}
				peak = max(peak, x)
			}
			if peak < 0.01 {
				t.Errorf("silent: peak %g", peak)
			}
			l2, _, _, _ := RenderLine(v, nil, text, Options{}, rate)
			if !slices.Equal(left, l2) {
				t.Error("two renders differ")
			}
			d := Data(v, tl)
			if len(d.Notes) != len(tl.Notes) || d.TextLen != len(text) || d.Duration != tl.Duration.Seconds() {
				t.Errorf("data %+v", d)
			}
			if v.Rhythm != nil && len(v.Percussion) > 0 && !slices.ContainsFunc(d.Notes, func(n NoteData) bool { return n.Instrument == v.Percussion[0].Name }) {
				t.Errorf("no note named %q", v.Percussion[0].Name)
			}
		})
	}
}

func TestPercussionDefaults(t *testing.T) {
	d := PercussionDefaults()
	if len(d) != len(voice.PercussionNames()) {
		t.Fatalf("%d sounds", len(d))
	}
	if d["kick"]["wave"] != "sine" || d["kick"]["note"] != float32(36) || d["hat"]["filter"] != "highpass" {
		t.Errorf("kick %v, hat %v", d["kick"], d["hat"])
	}
}
