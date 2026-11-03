package main

import (
	"bytes"
	"strings"
	"testing"
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
