package dialogue_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/dialogue"
)

const valid = `{"version":0,"speakers":[{"id":"a","voice":"soft"}],"lines":[{"speaker":"a","text":"Hi."}]}`

func TestLoadDefaults(t *testing.T) {
	s, err := dialogue.Load(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	sp, l := s.Speakers[0], s.Lines[0]
	if sp.Name != "a" || sp.Pan != nil {
		t.Errorf("speaker %+v", sp)
	}
	if s.Advance != dialogue.AdvanceInput || s.Hold != 1 || l.Advance != dialogue.AdvanceInput || l.Hold != 1 || l.Speed != 1 || l.Delay != 0 || l.Instant {
		t.Errorf("defaults: script %+v line %+v", s, l)
	}
}

func TestLoadFile(t *testing.T) {
	fsys := fstest.MapFS{"scenes/x.dialogue.json": {Data: []byte(valid)}}
	s, err := dialogue.LoadFS(fsys, "scenes/x.dialogue.json")
	if err != nil || s.Dir != "scenes" {
		t.Fatalf("%v, dir %q", err, s.Dir)
	}
	if _, err := dialogue.LoadFS(fsys, "nope.dialogue.json"); err == nil {
		t.Error("missing file loaded")
	}
	_, err = dialogue.LoadFS(fstest.MapFS{"x.dialogue.json": {Data: []byte(`{"version":1}`)}}, "x.dialogue.json")
	if err == nil || !strings.HasPrefix(err.Error(), "x.dialogue.json: ") {
		t.Errorf("error %v lacks the file name", err)
	}
}

func TestLoadOverrides(t *testing.T) {
	s, err := dialogue.Load(strings.NewReader(`{"version":0,"advance":"auto","hold":2,"speakers":[{"id":"a","name":"Ann","voice":"soft","pan":0.5}],
		"lines":[{"speaker":"a","text":"x"},{"speaker":"a","text":"y","advance":"input","hold":3,"speed":2,"delay":1.5,"instant":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	a, b := s.Lines[0], s.Lines[1]
	if a.Advance != dialogue.AdvanceAuto || a.Hold != 2 {
		t.Errorf("inherited: %+v", a)
	}
	if b.Advance != dialogue.AdvanceInput || b.Hold != 3 || b.Speed != 2 || b.Delay != 1.5 || !b.Instant {
		t.Errorf("own: %+v", b)
	}
	if s.Speakers[0].Name != "Ann" || *s.Speakers[0].Pan != 0.5 {
		t.Errorf("speaker %+v", s.Speakers[0])
	}
}

func TestLoadProblems(t *testing.T) {
	tests := []struct{ name, json, path, msg string }{
		{"no version", `{"speakers":[{"id":"a","voice":"soft"}],"lines":[{"speaker":"a","text":"x"}]}`, "version", "missing"},
		{"version 1", `{"version":1}`, "version", "not supported"},
		{"advance", `{"version":0,"advance":"now"}`, "advance", "not \"input\" or \"auto\""},
		{"hold", `{"version":0,"hold":11}`, "hold", "outside 0 to 10"},
		{"no speakers", `{"version":0}`, "speakers", "at least one"},
		{"no lines", `{"version":0}`, "lines", "at least one"},
		{"id missing", `{"version":0,"speakers":[{"voice":"soft"}]}`, "speakers[0].id", "missing"},
		{"id chars", `{"version":0,"speakers":[{"id":"Wren","voice":"soft"}]}`, "speakers[0].id", "a-z"},
		{"id twice", `{"version":0,"speakers":[{"id":"a","voice":"soft"},{"id":"a","voice":"soft"}]}`, "speakers[1].id", "twice"},
		{"voice missing", `{"version":0,"speakers":[{"id":"a"}]}`, "speakers[0].voice", "missing"},
		{"voice typo", `{"version":0,"speakers":[{"id":"a","voice":"sofft"}]}`, "speakers[0].voice", "neither a starter"},
		{"voice path", `{"version":0,"speakers":[{"id":"a","voice":"../x.voice.json"}]}`, "speakers[0].voice", "relative path"},
		{"pan", `{"version":0,"speakers":[{"id":"a","voice":"soft","pan":2}]}`, "speakers[0].pan", "outside -1 to 1"},
		{"line speaker missing", `{"version":0,"lines":[{"text":"x"}]}`, "lines[0].speaker", "missing"},
		{"line speaker unknown", `{"version":0,"lines":[{"speaker":"z","text":"x"}]}`, "lines[0].speaker", "unknown speaker"},
		{"text empty", `{"version":0,"lines":[{"speaker":"z","text":"  "}]}`, "lines[0].text", "empty"},
		{"delay", `{"version":0,"lines":[{"delay":-1}]}`, "lines[0].delay", "outside 0 to 10"},
		{"speed", `{"version":0,"lines":[{"speed":9}]}`, "lines[0].speed", "outside 0.25 to 4"},
		{"line advance", `{"version":0,"lines":[{"advance":"x"}]}`, "lines[0].advance", "not \"input\""},
		{"line hold", `{"version":0,"lines":[{"hold":-2}]}`, "lines[0].hold", "outside 0 to 10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dialogue.Load(strings.NewReader(tt.json))
			if !hasProblem(err, tt.path, tt.msg) {
				t.Errorf("want %s: ...%s..., got %v", tt.path, tt.msg, err)
			}
		})
	}
}

func hasProblem(err error, path, msg string) bool {
	for _, e := range flatten(err) {
		var fe *voice.FieldError
		if errors.As(e, &fe) && fe.File == "dialogue" && fe.Path == path && strings.Contains(fe.Message, msg) {
			return true
		}
	}
	return false
}

func flatten(err error) []error {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		var out []error
		for _, e := range j.Unwrap() {
			out = append(out, flatten(e)...)
		}
		return out
	}
	if err == nil {
		return nil
	}
	if u := errors.Unwrap(err); u != nil {
		return flatten(u)
	}
	return []error{err}
}

func TestLoadCollectsEveryProblem(t *testing.T) {
	_, err := dialogue.Load(strings.NewReader(`{"version":0,"hold":99,"speakers":[{"id":"A","voice":"zzz"}],"lines":[{"speaker":"q","text":""}]}`))
	if got := len(flatten(err)); got != 5 {
		t.Errorf("%d problems, want 5: %v", got, err)
	}
}

func TestLoadMalformed(t *testing.T) {
	for _, j := range []string{`{`, `{"version":0,"bogus":1}`, valid + `{}`} {
		if _, err := dialogue.Load(strings.NewReader(j)); err == nil {
			t.Errorf("%q loaded", j)
		}
	}
}

func TestLoadVoices(t *testing.T) {
	voiceFile, err := voiceJSON()
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{
		"scenes/bram.voice.json": {Data: voiceFile},
		"scenes/s.dialogue.json": {Data: []byte(`{"version":0,"speakers":[
			{"id":"a","voice":"soft"},{"id":"b","voice":"bram.voice.json"},{"id":"c","voice":"soft"}],
			"lines":[{"speaker":"a","text":"x"}]}`)},
	}
	s, err := dialogue.LoadFS(fsys, "scenes/s.dialogue.json")
	if err != nil {
		t.Fatal(err)
	}
	voices, err := dialogue.LoadVoices(fsys, s)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sortedKeys(voices), []string{"bram.voice.json", "soft"}) {
		t.Errorf("keys %v", sortedKeys(voices))
	}
	if voices["soft"].Name != "soft" {
		t.Errorf("starter %q", voices["soft"].Name)
	}

	// A missing file and a broken file are problems at their speakers.
	bad := fstest.MapFS{
		"s.dialogue.json": fsys["scenes/s.dialogue.json"],
		"bram.voice.json": {Data: []byte(`{"version":0}`)},
	}
	s, _ = dialogue.LoadFS(bad, "s.dialogue.json")
	_, err = dialogue.LoadVoices(bad, s)
	if !hasProblem(err, "speakers[1].voice", "bram.voice.json") {
		t.Errorf("broken voice: %v", err)
	}
	_, err = dialogue.LoadVoices(fstest.MapFS{}, s)
	if !hasProblem(err, "speakers[1].voice", "file does not exist") {
		t.Errorf("missing voice: %v", err)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
