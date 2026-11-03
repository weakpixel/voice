package voice

import (
	"strings"
	"testing"
)

func testLanguage(t *testing.T, policy string) *Language {
	t.Helper()
	l, err := LoadLanguage(strings.NewReader(`{"version": 0, "phonemes": [
		{"symbol": "a", "category": "vowel"},
		{"symbol": "ä", "category": "vowel", "pitch": 1},
		{"symbol": "s", "category": "fricative"},
		{"symbol": "sch", "category": "fricative", "length": 1.5},
		{"symbol": "t", "category": "plosive"}
	], "punctuation": [
		{"symbol": ".", "pause": 4, "end": "fall"},
		{"symbol": "...", "pause": 6},
		{"symbol": "?", "pause": 4, "end": "rise"}
	], "unknown": ` + policy + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// describe renders tokens as "symbol/kind" separated by spaces, with the
// text of each span: a=[a].
func describe(text string, toks []Phoneme) string {
	var b strings.Builder
	for _, tk := range toks {
		kind := map[TokenKind]string{TokenPhoneme: "p", TokenPause: "_", TokenSkip: "x"}[tk.Kind]
		b.WriteString(kind + "[" + text[tk.Start:tk.End] + "] ")
	}
	return strings.TrimSpace(b.String())
}

func TestParse(t *testing.T) {
	tests := []struct {
		name, policy, text, want string
	}{
		{"longest first", `{"policy": "skip"}`, "schast", "p[sch] p[a] p[s] p[t]"},
		{"case folding", `{"policy": "skip"}`, "SCHÄ Sa", "p[SCH] p[Ä] _[ ] p[S] p[a]"},
		{"whitespace run", `{"policy": "skip"}`, "a \t\n a", "p[a] _[ \t\n ] p[a]"},
		{"punctuation longest", `{"policy": "skip"}`, "ta... ta.", "p[t] p[a] _[...] _[ ] p[t] p[a] _[.]"},
		{"unknown skip", `{"policy": "skip"}`, "bat9", "x[b] p[a] p[t] x[9]"},
		{"unknown pause", `{"policy": "pause"}`, "bat", "_[b] p[a] p[t]"},
		{"unknown fallback", `{"policy": "fallback", "phoneme": "t"}`, "bä", "p[b] p[ä]"},
		{"symbols always skip", `{"policy": "fallback", "phoneme": "t"}`, `"a"—😀`, `x["] p[a] x["] x[—] x[😀]`},
		{"invalid utf-8", `{"policy": "fallback", "phoneme": "t"}`, "a\xffa", "p[a] x[\xff] p[a]"},
		{"empty", `{"policy": "skip"}`, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := Parse(testLanguage(t, tt.policy), tt.text)
			if got := describe(tt.text, toks); got != tt.want {
				t.Errorf("Parse(%q)\n got %s\nwant %s", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseFields(t *testing.T) {
	l := testLanguage(t, `{"policy": "fallback", "phoneme": "s"}`)
	toks := Parse(l, "schäb? .")
	want := []Phoneme{
		{Kind: TokenPhoneme, Symbol: "sch", Category: Fricative, Length: 1.5, Velocity: 1, Start: 0, End: 3},
		{Kind: TokenPhoneme, Symbol: "ä", Category: Vowel, Pitch: 1, Length: 1, Velocity: 1, Start: 3, End: 5},
		{Kind: TokenPhoneme, Symbol: "s", Category: Fricative, Length: 1, Velocity: 1, Start: 5, End: 6},
		{Kind: TokenPause, Symbol: "?", Length: 4, Contour: ContourRise, Clause: true, Start: 6, End: 7},
		{Kind: TokenPause, Symbol: " ", Length: 0.5, Start: 7, End: 8},
		{Kind: TokenPause, Symbol: ".", Length: 4, Contour: ContourFall, Clause: true, Start: 8, End: 9},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens: %+v", len(toks), toks)
	}
	for i := range want {
		if toks[i] != want[i] {
			t.Errorf("token %d\n got %+v\nwant %+v", i, toks[i], want[i])
		}
	}
}

func TestParseSpansTile(t *testing.T) {
	l := DefaultLanguage()
	for _, text := range []string{
		"Hello there, traveler!",
		"Ünïcödé — ΣΊΣΥΦΟΣ? 日本語… ok...",
		"  leading and trailing  ",
		"\xff\xfe broken",
	} {
		toks := Parse(l, text)
		at := 0
		for _, tk := range toks {
			if tk.Start != at || tk.End <= tk.Start {
				t.Fatalf("%q: token %+v does not continue at %d", text, tk, at)
			}
			at = tk.End
		}
		if at != len(text) {
			t.Errorf("%q: tokens end at %d of %d", text, at, len(text))
		}
	}
}

func TestAppendParseReuses(t *testing.T) {
	l := DefaultLanguage()
	buf := make([]Phoneme, 0, 64)
	if n := testing.AllocsPerRun(100, func() { buf = AppendParse(buf[:0], l, "Hello there, traveler!") }); n != 0 {
		t.Errorf("AppendParse into a big enough slice: %g allocs", n)
	}
}

func FuzzParse(f *testing.F) {
	f.Add("Hello there, traveler!")
	f.Add("ÄÖÜ sch... ?!")
	f.Fuzz(func(t *testing.T, text string) {
		at := 0
		for _, tk := range Parse(DefaultLanguage(), text) {
			if tk.Start != at || tk.End <= tk.Start || tk.End > len(text) {
				t.Fatalf("bad span %+v at %d", tk, at)
			}
			at = tk.End
		}
		if at != len(text) {
			t.Fatalf("ends at %d of %d", at, len(text))
		}
	})
}
