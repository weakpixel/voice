package voice

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the golden timelines in testdata/")

func preset(t *testing.T, name string) *Voice {
	t.Helper()
	v, err := Preset(name)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

const goldenLine = "Hello there, traveler! Are you lost?"

// TestComposeGolden compares the timelines of the starter voices with
// testdata/compose/<voice>.json. Run with -update after an intended change.
func TestComposeGolden(t *testing.T) {
	for _, name := range PresetNames() {
		t.Run(name, func(t *testing.T) {
			tl := ComposeText(preset(t, name), DefaultLanguage(), goldenLine)
			got, err := json.MarshalIndent(tl, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "compose", name+".json")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got)+"\n" != string(want) {
				t.Errorf("timeline differs from %s (run go test -run ComposeGolden -update and review the diff)", path)
			}
		})
	}
}

func TestComposeDeterministic(t *testing.T) {
	v := preset(t, "chirpy")
	a := ComposeText(v, DefaultLanguage(), goldenLine)
	b := ComposeText(v, DefaultLanguage(), goldenLine)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Error("same voice and text gave different timelines")
	}
	c := ComposeText(v, DefaultLanguage(), goldenLine+" ")
	if c.Notes[0].Note == a.Notes[0].Note && c.Notes[1].Note == a.Notes[1].Note && c.Notes[2].Note == a.Notes[2].Note {
		t.Error("a different text gave the same random pitches")
	}
}

func TestComposeTiming(t *testing.T) {
	v := preset(t, "soft")
	v.Speed = 10 // a beat is 100 ms
	v.Variation = 0
	v.MotifVolume = 0
	v.Length = 0.5
	l := testLanguage(t, `{"policy": "skip"}`)
	text := `ta... "a".`
	tl := Compose(v, text, Parse(l, text))

	ms := time.Millisecond
	wantNotes := []Note{
		{On: 0, Off: 50 * ms, Instrument: InstPlosive},
		{On: 100 * ms, Off: 150 * ms, Instrument: InstVowel},
		// "... " is 6 + 0.5 beats of silence; the quote is skipped. The
		// line's last syllable is 1.5 times as long (150 ms) and its note
		// holds 0.5 + 0.15 of it.
		{On: 850 * ms, Off: 947500 * time.Microsecond, Instrument: InstVowel},
	}
	if len(tl.Notes) != len(wantNotes) {
		t.Fatalf("%d notes: %+v", len(tl.Notes), tl.Notes)
	}
	for i, w := range wantNotes {
		n := tl.Notes[i]
		if n.On != w.On || n.Off != w.Off || n.Instrument != w.Instrument {
			t.Errorf("note %d: %v-%v %v, want %v-%v %v", i, n.On, n.Off, n.Instrument, w.On, w.Off, w.Instrument)
		}
	}
	// The last "a" ends the clause with a fall: 4 semitones below the first.
	if d := tl.Notes[2].Note - tl.Notes[1].Note; d != -4 {
		t.Errorf("fall on the last phoneme: %g semitones, want -4", d)
	}
	wantReveals := []Reveal{{0, 1}, {100 * ms, 2}, {200 * ms, 5}, {800 * ms, 6}, {850 * ms, 8}, {1000 * ms, 10}}
	if len(tl.Reveals) != len(wantReveals) {
		t.Fatalf("reveals %+v", tl.Reveals)
	}
	for i, w := range wantReveals {
		if tl.Reveals[i] != w {
			t.Errorf("reveal %d: %+v, want %+v", i, tl.Reveals[i], w)
		}
	}
	// The line ends after the voice's last sound plus the breath.
	if d := tl.Duration - (1000+400)*ms; d < -time.Microsecond || d > time.Microsecond || tl.TextLen != len(text) {
		t.Errorf("duration %v textLen %d", tl.Duration, tl.TextLen)
	}
}

func TestComposeContours(t *testing.T) {
	v := preset(t, "soft")
	v.Variation = 0
	v.MotifVolume = 0
	l := testLanguage(t, `{"policy": "skip"}`)
	last := func(text string) (pitch, vel float32) {
		tl := Compose(v, text, Parse(l, text))
		n := tl.Notes[len(tl.Notes)-1]
		return n.Note, n.Velocity
	}
	flat, _ := last("tatata")
	if p, _ := last("tatata."); p != flat-4 {
		t.Errorf("fall: %g, want %g", p, flat-4)
	}
	if p, _ := last("tatata?"); p != flat+5 {
		t.Errorf("rise: %g, want %g", p, flat+5)
	}
	if l2, err := LoadLanguage(stringsReader(`{"version": 0, "phonemes": [{"symbol": "a", "category": "vowel", "velocity": 0.5}]}`)); err == nil {
		tl := ComposeText(v, l2, "aa!")
		if n := tl.Notes[1]; n.Velocity != v.Velocity*0.5*accentVelocity*finalFade[2] {
			t.Errorf("accent velocity %g", n.Velocity)
		}
	} else {
		t.Fatal(err)
	}
}

func TestComposeMotif(t *testing.T) {
	v := preset(t, "soft")
	motif := func(text string) []Note {
		var out []Note
		for _, n := range ComposeText(v, DefaultLanguage(), text).Notes {
			if n.Instrument == InstMotif {
				out = append(out, n)
			}
		}
		return out
	}
	tests := []struct {
		text string
		want int
	}{
		{"hello", 2},
		{"hello. there", 2},
		{"one, two, three", 3},
		{"one, two, three, four, five, six!", 4},
		{"...", 0},
	}
	for _, tt := range tests {
		m := motif(tt.text)
		if len(m) != tt.want {
			t.Errorf("%q: %d motif notes, want %d", tt.text, len(m), tt.want)
		}
		for i, n := range m {
			if n.Velocity <= 0 || n.Off <= n.On || i > 0 && n.On < m[i-1].Off {
				t.Errorf("%q: motif note %d %+v", tt.text, i, n)
			}
		}
	}
	// A question ends higher than it starts, a statement lower.
	if q := motif("hello there?"); q[1].Note <= q[0].Note {
		t.Errorf("question motif %g -> %g", q[0].Note, q[1].Note)
	}
	if s := motif("hello there."); s[1].Note >= s[0].Note {
		t.Errorf("statement motif %g -> %g", s[0].Note, s[1].Note)
	}
	v.MotifVolume = 0
	if m := motif("hello"); len(m) != 0 {
		t.Error("motif volume 0 still plays a motif")
	}
}

func TestComposeRevealsCoverText(t *testing.T) {
	for _, text := range []string{"", "...", "\"\"", "Hi!", goldenLine} {
		tl := ComposeText(preset(t, "gruff"), DefaultLanguage(), text)
		last := tl.Reveals[len(tl.Reveals)-1]
		if last.Offset != len(text) || last.At > tl.Duration {
			t.Errorf("%q: last reveal %+v, duration %v", text, last, tl.Duration)
		}
		for i := 1; i < len(tl.Reveals); i++ {
			if tl.Reveals[i].At <= tl.Reveals[i-1].At || tl.Reveals[i].Offset < tl.Reveals[i-1].Offset {
				t.Errorf("%q: reveals out of order: %+v", text, tl.Reveals)
			}
		}
		for _, n := range tl.Notes {
			if n.Off > tl.Duration {
				t.Errorf("%q: note %+v ends after the duration %v", text, n, tl.Duration)
			}
		}
	}
}

func stringsReader(s string) *strings.Reader { return strings.NewReader(s) }
