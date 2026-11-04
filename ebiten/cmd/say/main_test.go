package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/synth"
)

// The audio device cannot be opened in tests; these cover everything up to
// it: flags, usage errors and loading the voice and language.
func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		ok      bool
		text    string
		wantErr string
	}{
		{"text", []string{"Hello", "there!"}, true, "Hello there!", ""},
		{"voice", []string{"-voice", "gruff", "Hi."}, true, "Hi.", ""},
		{"no text", []string{"-voice", "gruff"}, false, "", "give a line of text"},
		{"bad flag", []string{"-nope", "Hi."}, false, "", "flag provided but not defined"},
		{"bad volume", []string{"-volume", "2", "Hi."}, false, "Hi.", "-volume must be between 0 and 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			o, ok := parse(tt.args, &stderr)
			if ok != tt.ok || o.text != tt.text || !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("parse(%q) = %q, %v; stderr %q", tt.args, o.text, ok, stderr.String())
			}
		})
	}
}

func TestLoad(t *testing.T) {
	for _, name := range []string{"", "chirpy", "flow"} {
		if _, _, lang, err := load(options{voice: name}); err != nil || lang == nil {
			t.Errorf("load(%q): %v", name, err)
		}
	}
	if _, _, _, err := load(options{voice: "nobody"}); err == nil {
		t.Error("load(nobody): no error")
	}
	if _, _, _, err := load(options{lang: "missing.lang.json"}); err == nil {
		t.Error("load(missing language): no error")
	}
}

// TestPlayerBuffer reads the stream the way the audio player does, in
// chunks of its buffer whenever the read-ahead runs out, while the speaker
// follows the clock. The blips must keep their rhythm: with oto's default
// read-ahead of 0.5 s they bunched into one burst every half second, with
// silence in between.
func TestPlayerBuffer(t *testing.T) {
	const rate = 48000
	for _, name := range voice.PresetNames() {
		t.Run(name, func(t *testing.T) {
			want := longestGap(t, name, rate, rate*8/1000) // 1 ms ahead
			got := longestGap(t, name, rate, playerBufferBytes(rate))
			if got > want+100*time.Millisecond {
				t.Errorf("longest silence in the line %v with the player buffer, %v reading 1 ms ahead", got, want)
			}
		})
	}
}

// longestGap speaks a line with the motif off, reading the stream
// chunkBytes at a time, and returns the longest silence between its first
// and last sound, in 5 ms windows.
func longestGap(t *testing.T, name string, rate, chunkBytes int) time.Duration {
	t.Helper()
	v, err := voice.Preset(name)
	if err != nil {
		t.Fatal(err)
	}
	v.MotifVolume = 0
	s, err := synth.NewStream(synth.StreamConfig{SampleRate: rate})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetVoice(owner, v, nil); err != nil {
		t.Fatal(err)
	}
	sp := voice.NewSpeaker(s, owner)
	sp.Say(v, "The Village is past you.", voice.SayOptions{})
	buf := make([]byte, chunkBytes)
	var out []float32
	var ahead time.Duration
	for range 4000 {
		if ahead <= 0 {
			s.Read(buf)
			for i := 0; i+8 <= len(buf); i += 8 {
				out = append(out, math.Float32frombits(binary.LittleEndian.Uint32(buf[i:])))
			}
			ahead += time.Duration(len(buf)/8) * time.Second / time.Duration(rate)
		}
		sp.Update(time.Millisecond)
		ahead -= time.Millisecond
	}
	window := rate * 5 / 1000
	gap, longest, started := 0, 0, false
	for i := 0; i+window <= len(out); i += window {
		var e float64
		for _, x := range out[i : i+window] {
			e += float64(x) * float64(x)
		}
		if math.Sqrt(e/float64(window)) > 0.002 {
			if started {
				longest = max(longest, gap)
			}
			started, gap = true, 0
		} else {
			gap++
		}
	}
	return time.Duration(longest) * 5 * time.Millisecond
}
