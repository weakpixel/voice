package load_test

import (
	"os"
	"slices"
	"testing"

	"github.com/weakpixel/voice/editor/game/load"
)

func village(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../../ebiten/example/village.dialogue.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func has(problems []load.Problem, file, path string) bool {
	return slices.ContainsFunc(problems, func(p load.Problem) bool { return p.File == file && p.Path == path })
}

func TestDecodeVillage(t *testing.T) {
	l, problems := load.Decode(load.Request{Dialogue: village(t), Start: 2, AutoAdvance: true})
	if len(problems) > 0 || l == nil {
		t.Fatalf("problems: %+v", problems)
	}
	if len(l.Script.Lines) != 14 || len(l.Voices) != 4 || l.Language != nil {
		t.Errorf("script %d lines, %d voices, language %v", len(l.Script.Lines), len(l.Voices), l.Language)
	}
}

func TestDecodeProblems(t *testing.T) {
	dlg := `{"version":0,"speakers":[{"id":"a","voice":"bram.voice.json"}],"lines":[{"speaker":"a","text":"x"}]}`
	tests := []struct {
		name string
		req  load.Request
		file string
		path string
	}{
		{"malformed dialogue", load.Request{Dialogue: `{`}, "dialogue", ""},
		{"bad field", load.Request{Dialogue: `{"version":0,"speakers":[],"lines":[]}`}, "dialogue", "speakers"},
		{"missing voice file", load.Request{Dialogue: dlg}, "dialogue", "speakers[0].voice"},
		{"broken voice file", load.Request{Dialogue: dlg, Voices: map[string]string{"bram.voice.json": `{"version":0}`}}, "dialogue", "speakers[0].voice"},
		{"bad language", load.Request{Dialogue: village(t), Language: `{"version":0,"phonemes":[]}`}, "language", ""},
		{"start", load.Request{Dialogue: village(t), Start: 99}, "dialogue", "lines"},
		{"bad sample", load.Request{Dialogue: village(t), Samples: map[string][]byte{"x.wav": []byte("no")}}, "sample", "x.wav"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, problems := load.Decode(tt.req)
			if l != nil || len(problems) == 0 {
				t.Fatalf("loaded: %v %v", l, problems)
			}
			if tt.path == "" {
				if !slices.ContainsFunc(problems, func(p load.Problem) bool { return p.File == tt.file }) {
					t.Errorf("no %s problem in %+v", tt.file, problems)
				}
			} else if !has(problems, tt.file, tt.path) {
				t.Errorf("no %s problem at %s in %+v", tt.file, tt.path, problems)
			}
		})
	}
}

func TestDecodeVoiceFile(t *testing.T) {
	dlg := `{"version":0,"speakers":[{"id":"a","voice":"bram.voice.json"}],"lines":[{"speaker":"a","text":"x"}]}`
	voiceText := `{"version":0,"name":"bram","instruments":{"vowel":{"wave":"sine"}}}`
	l, problems := load.Decode(load.Request{Dialogue: dlg, Voices: map[string]string{"bram.voice.json": voiceText}})
	if len(problems) > 0 || l.Voices["bram.voice.json"].Name != "bram" {
		t.Fatalf("%+v %+v", problems, l)
	}
}
