package voice

import (
	"strings"
	"testing"
)

// composeSpan composes markup src with host, the span voices by name.
func composeSpan(t *testing.T, host *Voice, src string, voices map[string]*Voice) Timeline {
	t.Helper()
	text, ms, err := ParseMarkup(src)
	if err != nil {
		t.Fatal(err)
	}
	var spans []VoiceSpan
	for _, m := range ms {
		spans = append(spans, VoiceSpan{Start: m.Start, End: m.End, Voice: voices[m.Name], Impression: m.Impression})
	}
	return ComposeWith(host, text, Parse(DefaultLanguage(), text), ComposeOptions{Spans: spans})
}

const spanLine = "Hello {g}there, traveler{/}! Are you lost?"

// A span spoken by the line's own voice changes nothing but Note.Span.
func TestComposeSpanOfOwnVoice(t *testing.T) {
	for _, name := range PresetNames() {
		t.Run(name, func(t *testing.T) {
			host := preset(t, name)
			same := *host
			got := composeSpan(t, host, spanLine, map[string]*Voice{"g": &same})
			want := ComposeText(host, DefaultLanguage(), strings.NewReplacer("{g}", "", "{/}", "").Replace(spanLine))
			if len(got.Notes) != len(want.Notes) || got.Duration != want.Duration || len(got.Reveals) != len(want.Reveals) {
				t.Fatalf("got %d notes %v, want %d notes %v", len(got.Notes), got.Duration, len(want.Notes), want.Duration)
			}
			spanned := 0
			for i, n := range got.Notes {
				if n.Span != 0 {
					spanned++
				}
				n.Span = 0
				if n != want.Notes[i] {
					t.Errorf("note %d: %+v, want %+v", i, n, want.Notes[i])
				}
			}
			if spanned == 0 {
				t.Error("no note has Span 1")
			}
			for i, r := range got.Reveals {
				if r != want.Reveals[i] {
					t.Errorf("reveal %d: %+v, want %+v", i, r, want.Reveals[i])
				}
			}
		})
	}
}

func TestComposeSpanVoice(t *testing.T) {
	host := preset(t, "soft")
	host.Variation = 0
	guest := *host
	guest.BaseNote -= 12
	guest.Pitch = 0
	guest.Speed = host.Speed / 2
	guest.Pan = 0.8
	host.Pan = -0.5
	plain := ComposeText(host, DefaultLanguage(), "Hello there, traveler! Are you lost?")
	full := composeSpan(t, host, spanLine, map[string]*Voice{"g": &guest})
	imp := composeSpan(t, host, strings.Replace(spanLine, "{g}", "{g~}", 1), map[string]*Voice{"g": &guest})
	shift := register(&guest) - register(host)

	firstGuest := -1
	for i, n := range full.Notes {
		if n.Instrument == InstMotif {
			if n.Span != 0 {
				t.Errorf("motif note %d has Span %d", i, n.Span)
			}
			continue
		}
		if n.Pan != -0.5 {
			t.Errorf("note %d pan %g, want the host's -0.5", i, n.Pan)
		}
		if n.Span == 1 && firstGuest < 0 {
			firstGuest = i
		}
	}
	if firstGuest < 0 {
		t.Fatal("no guest note")
	}
	// Notes before the span are the same as without it.
	for i := range firstGuest {
		if n := full.Notes[i]; n.Instrument != InstMotif && n != plain.Notes[i] {
			t.Errorf("note %d before the span changed: %+v, want %+v", i, n, plain.Notes[i])
		}
	}
	g, p, im := full.Notes[firstGuest], plain.Notes[firstGuest], imp.Notes[firstGuest]
	if d := g.Note - p.Note; abs32(d-shift) > 1e-3 {
		t.Errorf("guest note shifted %g semitones, want %g", d, shift)
	}
	if d := im.Note - p.Note; abs32(d-shift/2) > 1e-3 {
		t.Errorf("impression note shifted %g semitones, want %g", d, shift/2)
	}
	if got, want := g.Off-g.On, 2*(p.Off-p.On); absDur(int64(got-want)) > 2 {
		t.Errorf("guest note lasts %v, want %v (half the speed)", got, want)
	}
	if g.On != p.On {
		t.Errorf("the switch added time: guest starts %v, plain %v", g.On, p.On)
	}
	if full.Duration <= plain.Duration {
		t.Errorf("a slower span should make the line longer: %v <= %v", full.Duration, plain.Duration)
	}
}

func TestComposeSpanSnapsToTokens(t *testing.T) {
	host := preset(t, "soft")
	text, ms, _ := ParseMarkup("sooo{g}ooo{/}!")
	tokens := Parse(DefaultLanguage(), text)
	spans := []VoiceSpan{{Start: ms[0].Start, End: ms[0].End, Voice: host}}
	ts := tokenSpans(tokens, spans)
	for i, tok := range tokens {
		want := 0
		if tok.Start >= 4 && tok.Start < 7 {
			want = 1
		}
		if ts[i] != want {
			t.Errorf("token %d (%d to %d) in span %d, want %d", i, tok.Start, tok.End, ts[i], want)
		}
	}
}

func TestComposeSpanRhythmicHost(t *testing.T) {
	host := preset(t, "flow")
	guest := preset(t, "gruff")
	tl := composeSpan(t, host, "Yo, listen {g}up, I came{/} from the hills", map[string]*Voice{"g": guest})
	plain := ComposeText(host, DefaultLanguage(), "Yo, listen up, I came from the hills")
	perc := func(tl Timeline) (n int) {
		for _, x := range tl.Notes {
			if x.Instrument >= InstPercussion {
				n++
				if x.Span != 0 {
					t.Errorf("percussion note with Span %d", x.Span)
				}
			}
		}
		return n
	}
	if perc(tl) < perc(plain) {
		t.Error("the host's percussion is missing")
	}
	guestNotes := 0
	for _, x := range tl.Notes {
		if x.Span == 1 {
			guestNotes++
		}
	}
	if guestNotes == 0 {
		t.Error("no guest notes on a rhythmic host")
	}
}

func TestComposeSpanPanics(t *testing.T) {
	v := preset(t, "soft")
	tests := []struct {
		name  string
		spans []VoiceSpan
		want  string
	}{
		{"no voice", []VoiceSpan{{Start: 0, End: 2}}, "has no voice"},
		{"outside", []VoiceSpan{{Start: 0, End: 99, Voice: v}}, "outside the text"},
		{"overlap", []VoiceSpan{{Start: 0, End: 3, Voice: v}, {Start: 2, End: 4, Voice: v}}, "overlaps"},
		{"reversed", []VoiceSpan{{Start: 3, End: 2, Voice: v}}, "not sorted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if s, _ := r.(string); !strings.Contains(s, tt.want) || !strings.HasPrefix(s, "voice: ") {
					t.Errorf("panic %v, want %q", r, tt.want)
				}
			}()
			ComposeWith(v, "hello", Parse(DefaultLanguage(), "hello"), ComposeOptions{Spans: tt.spans})
		})
	}
}

func abs32(x float32) float32 { return max(x, -x) }
func absDur(x int64) int64    { return max(x, -x) }
