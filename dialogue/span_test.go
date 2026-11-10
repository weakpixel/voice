package dialogue_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/weakpixel/voice/dialogue"
	"github.com/weakpixel/voice/voicetest"
)

// spanScript has wren quoting bram, and a ghost without lines of its own.
const spanScript = `{"version":0,
 "speakers":[{"id":"wren","voice":"chirpy","pan":-0.3},{"id":"bram","voice":"gruff","pan":0.3},{"id":"ghost","voice":"divine"}],
 "lines":[
  {"speaker":"wren","text":"Grandpa said {bram}keep your boots dry{/}, and {ghost~}something else{/}."},
  {"speaker":"bram","text":"Did I?","delay":0.5}]}`

func TestLoadSpans(t *testing.T) {
	s, err := dialogue.Load(strings.NewReader(spanScript))
	if err != nil {
		t.Fatal(err)
	}
	l := s.Lines[0]
	if l.Text != "Grandpa said keep your boots dry, and something else." || !strings.Contains(l.Source, "{bram}") {
		t.Errorf("text %q source %q", l.Text, l.Source)
	}
	want := []dialogue.Span{{Start: 13, End: 32, Speaker: "bram"}, {Start: 38, End: 52, Speaker: "ghost", Impression: true}}
	if !reflect.DeepEqual(l.Spans, want) {
		t.Errorf("spans %+v, want %+v", l.Spans, want)
	}
	if s.Lines[1].Spans != nil || s.Lines[1].Source != "Did I?" {
		t.Errorf("plain line %+v", s.Lines[1])
	}
}

func TestLoadSpanProblems(t *testing.T) {
	tests := []struct{ name, text, msg string }{
		{"unknown speaker", "a {nobody}b{/}", "{nobody} is not a speaker"},
		{"starter name is not a speaker", "a {soft}b{/}", "{soft} is not a speaker"},
		{"unclosed", "a {a}b", "byte 2: span {a} is not closed"},
		{"nested", "{a}x{a}y{/}", "byte 4:"},
		{"only markup", "{a}{/}", "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := `{"version":0,"speakers":[{"id":"a","voice":"soft"}],"lines":[{"speaker":"a","text":"` + tt.text + `"}]}`
			_, err := dialogue.Load(strings.NewReader(src))
			if !hasProblem(err, "lines[0].text", tt.msg) {
				t.Errorf("want lines[0].text: ...%s..., got %v", tt.msg, err)
			}
		})
	}
}

func TestPlayerSpans(t *testing.T) {
	s, err := dialogue.Load(strings.NewReader(spanScript))
	if err != nil {
		t.Fatal(err)
	}
	voices, err := dialogue.LoadVoices(nil, s)
	if err != nil {
		t.Fatal(err)
	}
	cuts := map[string]func(*dialogue.Player){
		"end":   nil,
		"next":  (*dialogue.Player).Next,
		"start": func(p *dialogue.Player) { p.Start(1) },
		"stop":  (*dialogue.Player).Stop,
		"close": (*dialogue.Player).Close,
		"pause": (*dialogue.Player).Pause,
	}
	for name, cut := range cuts {
		t.Run(name, func(t *testing.T) {
			sink := voicetest.NewRecordingSink(0)
			p, err := dialogue.NewPlayer(sink, s, voices, dialogue.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if cut == nil {
				runUntil(t, p, func() bool { return p.Phase() == dialogue.Holding })
			} else {
				runUntil(t, p, func() bool { return len(sink.Sounding(2)) > 0 }) // bram's voice speaks the quote
				cut(p)
			}
			assertSilent(t, sink, p)
			ons := map[int]int{}
			for _, c := range sink.Calls() {
				if c.Kind == voicetest.NoteOn {
					ons[int(c.Owner)]++
					if c.Pan != float32(-0.3) {
						t.Fatalf("a note on owner %d has pan %g, want wren's -0.3", c.Owner, c.Pan)
					}
				}
			}
			if ons[1] == 0 || ons[2] == 0 {
				t.Errorf("notes per owner %v: want wren's and bram's", ons)
			}
		})
	}
}
