package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchesRender(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sub", "patches.wav")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-patches", "-rate", "22050", "-o", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 1000 {
		t.Errorf("WAV of %d bytes", info.Size())
	}
	for _, np := range testPatches() {
		if err := np.patch.Validate(); err != nil {
			t.Errorf("%s: %v", np.name, err)
		}
	}
}

func TestEveryPatchSounds(t *testing.T) {
	left, _, err := renderPatches(22050)
	if err != nil {
		t.Fatal(err)
	}
	per := len(left) / len(testPatches())
	for i, np := range testPatches() {
		var p float32
		for _, v := range left[i*per : (i+1)*per] {
			p = max(p, v, -v)
		}
		if p < 0.05 || p >= 1 {
			t.Errorf("%s: peak %g, want audible and unclipped", np.name, p)
		}
	}
}

func TestUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"hello"}, &stdout, &stderr); code != exitUsage {
		t.Errorf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "only -patches") {
		t.Errorf("stderr %q", stderr.String())
	}
}
