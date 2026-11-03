package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runOK(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("voiceplay %v: exit %d: %s", args, code, stderr.String())
	}
	return stdout.String()
}

func TestPatchesRender(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sub", "patches.wav")
	runOK(t, "-patches", "-rate", "22050", "-o", out)
	if info, err := os.Stat(out); err != nil || info.Size() < 1000 {
		t.Fatalf("WAV: %v %v", info, err)
	}
	for _, np := range testPatches() {
		if err := np.patch.Validate(); err != nil {
			t.Errorf("%s: %v", np.name, err)
		}
	}
}

func TestEveryTestPatchSounds(t *testing.T) {
	left, _, _, err := render(options{patches: true, rate: 22050})
	if err != nil {
		t.Fatal(err)
	}
	per := len(left) / len(testPatches())
	for i, np := range testPatches() {
		if p := peak(left[i*per : (i+1)*per]); p < 0.05 || p >= 1 {
			t.Errorf("%s: peak %g, want audible and unclipped", np.name, p)
		}
	}
}

func peak(x []float32) float32 {
	var p float32
	for _, v := range x {
		p = max(p, v, -v)
	}
	return p
}

func TestLineRender(t *testing.T) {
	for _, name := range []string{"soft", "chirpy", "gruff"} {
		left, _, _, err := render(options{voice: name, rate: 22050, text: "Hello there, traveler!"})
		if err != nil {
			t.Fatal(err)
		}
		if p := peak(left); p < 0.1 || p >= 1 {
			t.Errorf("%s: peak %g", name, p)
		}
		if s := float64(len(left)) / 22050; s < 1 || s > 4 {
			t.Errorf("%s: %.1f s", name, s)
		}
	}
}

func TestVoiceFileWithSample(t *testing.T) {
	dir := t.TempDir()
	// A 16-bit mono WAV of a short ramp.
	var data []byte
	for i := range 2000 {
		data = binary.LittleEndian.AppendUint16(data, uint16(int16((i%100-50)*400)))
	}
	var w []byte
	w = append(w, "RIFF"...)
	w = binary.LittleEndian.AppendUint32(w, uint32(36+len(data)))
	w = append(w, "WAVEfmt "...)
	w = binary.LittleEndian.AppendUint32(w, 16)
	w = binary.LittleEndian.AppendUint16(w, 1)
	w = binary.LittleEndian.AppendUint16(w, 1)
	w = binary.LittleEndian.AppendUint32(w, 22050)
	w = binary.LittleEndian.AppendUint32(w, 44100)
	w = binary.LittleEndian.AppendUint16(w, 2)
	w = binary.LittleEndian.AppendUint16(w, 16)
	w = append(w, "data"...)
	w = binary.LittleEndian.AppendUint32(w, uint32(len(data)))
	w = append(w, data...)
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("blip.wav", string(w))
	write("me.voice.json", `{"version": 0, "name": "me", "instruments": {"vowel": {"sample": "blip.wav", "root": 60}}}`)
	write("tiny.lang.json", `{"version": 0, "phonemes": [{"symbol": "a", "category": "vowel"}]}`)
	out := runOK(t, "-voice", filepath.Join(dir, "me.voice.json"), "-lang", filepath.Join(dir, "tiny.lang.json"), "-o", filepath.Join(dir, "x.wav"), "aaa", "aa.")
	if !strings.Contains(out, "voice me, 7 notes") { // 5 phonemes and 2 motif notes
		t.Errorf("stdout %q", out)
	}
	runOK(t, "-patches", "-voice", filepath.Join(dir, "me.voice.json"), "-o", filepath.Join(dir, "p.wav"))
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"-patches", "hello"}, {"-bogus"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, exitUsage)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-voice", "loud", "hi"}, &stdout, &stderr); code != exitFailure || !strings.Contains(stderr.String(), "unknown preset") {
		t.Errorf("unknown voice: exit %d, %s", code, stderr.String())
	}
}
