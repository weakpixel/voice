package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/weakpixel/voice"
)

func TestBuild(t *testing.T) {
	out := t.TempDir()
	exec := filepath.Join(runtime.GOROOT(), "lib", "wasm", "wasm_exec.js")
	if err := build(out, "../../web", "../../../presets", "v123", exec); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), "__VERSION__") || !strings.Contains(string(html), "app.js?v=v123") {
		t.Error("version not stamped into index.html")
	}
	for _, f := range []string{"app.js", "text.js", "style.css", "wasm_exec.js", "icons/LICENSE", "presets/default.lang.json"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "test")); err == nil {
		t.Error("the page's tests were copied into the site")
	}
	var names []string
	data, _ := os.ReadFile(filepath.Join(out, "presets", "index.json"))
	if err := json.Unmarshal(data, &names); err != nil || !slices.Equal(names, voice.PresetNames()) {
		t.Errorf("index.json %s: %v", data, err)
	}
	for _, n := range names {
		if _, err := voice.LoadVoiceFS(os.DirFS(filepath.Join(out, "presets")), n+".voice.json"); err != nil {
			t.Error(err)
		}
	}
}
