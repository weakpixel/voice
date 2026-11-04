package voice

import (
	"math"
	"testing"
	"time"
)

// rhythmVoice is soft on a 120 bpm grid of sixteenths (125 ms steps, 2 s
// bars) with a kick on every beat and a hat on every eighth.
func rhythmVoice(t *testing.T) *Voice {
	t.Helper()
	v := preset(t, "soft")
	v.Variation = 0
	v.MotifVolume = 0
	kick, _ := BuiltinPercussion("kick")
	hat, _ := BuiltinPercussion("hat")
	v.Percussion = []Percussion{hat, kick}
	v.Rhythm = &Rhythm{BPM: 120, Division: 4, PauseRest: RestBeat, EndRest: RestBar, BeatVolume: 0.8,
		Beat: []BeatTrack{{"hat", "x.x.x.x.x.x.x.x."}, {"kick", "x...x...x...x..."}}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	return v
}

const rhythmStep = 125 * time.Millisecond

// steps returns d in grid steps and whether it is a whole number of them.
func steps(d time.Duration) (int, bool) {
	f := float64(d) / float64(rhythmStep)
	return int(math.Round(f)), math.Abs(f-math.Round(f)) < 1e-4
}

func notesOf(tl Timeline, inst Instrument) []Note {
	var out []Note
	for _, n := range tl.Notes {
		if n.Instrument == inst {
			out = append(out, n)
		}
	}
	return out
}

func TestRhythmVowelsOnGrid(t *testing.T) {
	v := rhythmVoice(t)
	tl := ComposeText(v, DefaultLanguage(), "Hello there traveler, are you lost?")
	vowels := notesOf(tl, InstVowel)
	if len(vowels) < 8 {
		t.Fatalf("%d vowel notes", len(vowels))
	}
	origin := vowels[0].On
	for _, n := range vowels {
		if _, ok := steps(n.On - origin); !ok {
			t.Errorf("vowel at %v is not on the grid (origin %v)", n.On, origin)
		}
	}
	// The comma rests to the next beat: "are" starts on a beat.
	text := "ta ta ta, ta"
	tl = ComposeText(v, DefaultLanguage(), text)
	vowels = notesOf(tl, InstVowel)
	if k, _ := steps(vowels[3].On - vowels[0].On); k%4 != 0 || k <= 3 {
		t.Errorf("after the comma the vowel is on step %d, want the next beat", k)
	}
	// The period rests to the next bar.
	tl = ComposeText(v, DefaultLanguage(), "ta ta. ta")
	vowels = notesOf(tl, InstVowel)
	if k, _ := steps(vowels[2].On - vowels[0].On); k != 16 {
		t.Errorf("after the period the vowel is on step %d, want 16", k)
	}
}

func TestRhythmSwing(t *testing.T) {
	v := rhythmVoice(t)
	v.Rhythm.Swing = 0.3
	v.Rhythm.Beat = v.Rhythm.Beat[:1] // the hat on every eighth
	v.Rhythm.Beat[0].Pattern = "xxxxxxxxxxxxxxxx"
	tl := ComposeText(v, DefaultLanguage(), "ta")
	hats := notesOf(tl, InstPercussion)
	for i := 1; i < len(hats); i++ {
		gap := hats[i].On - hats[i-1].On
		want := time.Duration(1.3 * float64(rhythmStep)) // an even step to the odd one
		if i%2 == 0 {
			want = time.Duration(0.7 * float64(rhythmStep))
		}
		if d := gap - want; d < -time.Microsecond || d > time.Microsecond {
			t.Fatalf("hit %d: gap %v, want %v", i, gap, want)
		}
	}
}

func TestRhythmAccents(t *testing.T) {
	v := rhythmVoice(t)
	v.Speed = 8 // a "ta" is about one step
	v.Velocity = 0.5
	tl := ComposeText(v, DefaultLanguage(), "ta ta ta ta ta ta ta ta ta ta ta ta ta ta ta ta ta")
	vowels := notesOf(tl, InstVowel)
	origin := vowels[0].On
	var strong, weak float32
	for _, n := range vowels[:len(vowels)-3] { // the last ones fade out
		k, _ := steps(n.On - origin)
		switch {
		case k%16 == 0:
			strong = n.Velocity
		case k%4 != 0: // no accent between the beats
			weak = n.Velocity
		}
	}
	if strong == 0 || weak == 0 || strong <= weak*1.2 {
		t.Errorf("velocity on beat 1 %g, off-beat %g, want beat 1 clearly louder", strong, weak)
	}
}

func TestRhythmPercussion(t *testing.T) {
	v := rhythmVoice(t)
	v.Rhythm.CountIn = 1
	tl := ComposeText(v, DefaultLanguage(), "Hello there traveler")
	kicks, hats := notesOf(tl, InstPercussion+1), notesOf(tl, InstPercussion)
	bars := int(tl.Duration / (16 * rhythmStep))
	if tl.Duration != time.Duration(bars)*16*rhythmStep || bars < 2 {
		t.Fatalf("duration %v is not a whole number of bars (at least count-in plus one)", tl.Duration)
	}
	if len(kicks) != 4*bars || len(hats) != 8*bars {
		t.Errorf("%d kicks and %d hats over %d bars, want %d and %d", len(kicks), len(hats), bars, 4*bars, 8*bars)
	}
	if kicks[0].On != 0 || kicks[0].Note != 36 || kicks[0].Velocity != 0.8 {
		t.Errorf("first kick %+v, want at 0 with note 36 and velocity 0.8", kicks[0])
	}
	for _, k := range kicks {
		if n, ok := steps(k.On); !ok || n%4 != 0 {
			t.Errorf("kick at %v is not on a beat", k.On)
		}
	}
	// Nothing sounds or shows during the count-in but the beat.
	bar := 16 * rhythmStep
	for _, n := range tl.Notes {
		if n.Instrument < InstPercussion && n.On < bar-rhythmStep/2 {
			t.Errorf("voice note %+v during the count-in", n)
		}
	}
	for _, r := range tl.Reveals {
		if r.Offset > 0 && r.At < bar-rhythmStep/2 {
			t.Errorf("reveal %+v during the count-in", r)
		}
	}
	if vowels := notesOf(tl, InstVowel); vowels[0].On != bar {
		t.Errorf("first vowel at %v, want the bar after the count-in (%v)", vowels[0].On, bar)
	}
	muted := ComposeWith(v, "Hello there traveler", Parse(DefaultLanguage(), "Hello there traveler"), ComposeOptions{MuteBeat: true})
	for _, n := range muted.Notes {
		if n.Instrument >= InstPercussion {
			t.Fatalf("MuteBeat: percussion note %+v", n)
		}
	}
}

func TestRhythmClock(t *testing.T) {
	v := rhythmVoice(t)
	v.Speed = 5 // a long "t": its pickup is the most a step allows (75 ms)
	text := "Tell me more"
	tokens := Parse(DefaultLanguage(), text)
	// 100 bpm: 150 ms steps, 2.4 s bars.
	for _, tt := range []struct {
		phase, want time.Duration
	}{
		{300 * time.Millisecond, 300 * time.Millisecond},
		{-200 * time.Millisecond, 2200 * time.Millisecond}, // the music started 200 ms ago
		{2450 * time.Millisecond, 2450 * time.Millisecond}, // 50 ms after the bar line leaves no room for the "t": a bar later
	} {
		tl := ComposeWith(v, text, tokens, ComposeOptions{Clock: &Clock{BPM: 100, Phase: tt.phase}})
		got := notesOf(tl, InstVowel)[0].On
		if d := got - tt.want; d < -time.Microsecond || d > time.Microsecond {
			t.Errorf("phase %v: first vowel at %v, want %v", tt.phase, got, tt.want)
		}
		kicks := notesOf(tl, InstPercussion+1)
		if gap := kicks[1].On - kicks[0].On; gap < 600*time.Millisecond-time.Microsecond || gap > 600*time.Millisecond+time.Microsecond {
			t.Errorf("phase %v: kicks %v apart, want 600 ms (100 bpm)", tt.phase, gap)
		}
	}
}

func TestRhythmReveals(t *testing.T) {
	v := rhythmVoice(t)
	v.Rhythm.CountIn = 2
	for _, text := range []string{"", "...", "\"\"", "Hi!", goldenLine, " Yo, listen up. I came from the hills, with nothing but skills."} {
		tl := ComposeText(v, DefaultLanguage(), text)
		last := tl.Reveals[len(tl.Reveals)-1]
		if last.Offset != len(text) || last.At > tl.Duration {
			t.Errorf("%q: last reveal %+v, duration %v", text, last, tl.Duration)
		}
		for i := 1; i < len(tl.Reveals); i++ {
			if tl.Reveals[i].At <= tl.Reveals[i-1].At || tl.Reveals[i].Offset < tl.Reveals[i-1].Offset {
				t.Errorf("%q: reveals out of order: %+v", text, tl.Reveals)
			}
		}
		for i, n := range tl.Notes {
			if n.Off > tl.Duration || n.Off <= n.On {
				t.Errorf("%q: note %+v, duration %v", text, n, tl.Duration)
			}
			if i > 0 && n.On < tl.Notes[i-1].On {
				t.Errorf("%q: notes out of order", text)
			}
		}
	}
}

func TestRhythmSyllables(t *testing.T) {
	c := composer{v: rhythmVoice(t), beat: 0.1}
	tokens := Parse(DefaultLanguage(), "banana strong")
	syl := c.syllables(tokens, c.v.Rhythm)
	var got []string
	for _, s := range syl {
		got = append(got, "banana strong"[tokens[s.first].Start:tokens[s.last].End]+"/"+tokens[s.nucleus].Symbol)
	}
	want := []string{"ba/a", "na/a", "na/a", "strong/o"}
	if len(got) != len(want) {
		t.Fatalf("syllables %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("syllables %q, want %q", got, want)
			break
		}
	}
}
