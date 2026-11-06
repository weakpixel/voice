package dialogue_test

import (
	"os"
	"testing"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/dialogue"
)

// voiceJSON returns a minimal valid voice file.
func voiceJSON() ([]byte, error) {
	return []byte(`{"version":0,"name":"bram","instruments":{"vowel":{"wave":"sine"}}}`), nil
}

// scene loads testdata/scene.dialogue.json with its starter voices.
func scene(t testing.TB) (*dialogue.Script, map[string]*voice.Voice) {
	t.Helper()
	fsys := os.DirFS("testdata")
	s, err := dialogue.LoadFS(fsys, "scene.dialogue.json")
	if err != nil {
		t.Fatal(err)
	}
	voices, err := dialogue.LoadVoices(fsys, s)
	if err != nil {
		t.Fatal(err)
	}
	return s, voices
}
