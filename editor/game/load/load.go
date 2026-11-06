// Package load turns the page's request to play a dialogue into what the
// game plays: the script, the speakers' voices, the language and the sample
// files, with every problem listed by file and JSON path. It has no
// syscall/js, so it is tested natively; the game (editor/game) is the thin
// bridge around it.
package load

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing/fstest"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/dialogue"
	"github.com/weakpixel/voice/synth"
	"github.com/weakpixel/voice/wav"
)

// Request is the page's "load" message.
type Request struct {
	// Dialogue is the text of the *.dialogue.json file.
	Dialogue string `json:"dialogue"`
	// Voices maps a voice file name, as a speaker names it, to the text of
	// that *.voice.json file. Starter voices need no entry.
	Voices map[string]string `json:"voices"`
	// Language is the text of a *.lang.json file used for every voice; empty
	// means the built-in language.
	Language string `json:"language"`
	// Samples maps a sample file name to the bytes of a 16-bit PCM WAV file.
	Samples map[string][]byte `json:"-"`
	// Start is the index of the line to start with.
	Start int `json:"start"`
	// AutoAdvance plays hands-free.
	AutoAdvance bool `json:"autoAdvance"`
}

// Problem is one thing wrong with a file of the request.
type Problem struct {
	// File is "dialogue", "voice", "language" or "sample". Path is the JSON
	// path in that file (the sample's name for "sample").
	File    string `json:"file"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Loaded is what the game plays.
type Loaded struct {
	Script *dialogue.Script
	// Voices is keyed by the speakers' voice references, see
	// dialogue.LoadVoices.
	Voices map[string]*voice.Voice
	// Language is the request's language, nil for the built-in one.
	Language *voice.Language
	// Samples resolves the request's sample files.
	Samples synth.SampleLoader
}

// Decode loads the request. It reports every problem it finds: when the
// returned problems are not empty, Loaded is nil.
func Decode(req Request) (*Loaded, []Problem) {
	var problems []Problem
	l := &Loaded{}

	if req.Language != "" {
		lang, err := voice.LoadLanguage(strings.NewReader(req.Language))
		problems = append(problems, collect("language", err)...)
		l.Language = lang
	}

	script, err := dialogue.Load(strings.NewReader(req.Dialogue))
	if err != nil {
		return nil, append(problems, collect("dialogue", err)...)
	}
	l.Script = script
	if req.Start < 0 || req.Start >= len(script.Lines) {
		problems = append(problems, Problem{File: "dialogue", Path: "lines", Message: fmt.Sprintf("start line %d is outside 0 to %d", req.Start, len(script.Lines)-1)})
	}

	// The voice files the speakers name, read as a file system next to the
	// dialogue; a missing one is a problem at the speaker.
	fsys := fstest.MapFS{}
	for name, text := range req.Voices {
		fsys[name] = &fstest.MapFile{Data: []byte(text)}
	}
	if l.Voices, err = dialogue.LoadVoices(fsys, script); err != nil {
		problems = append(problems, collect("dialogue", err)...)
	}

	samples := map[string]*synth.Sample{}
	for name, data := range req.Samples {
		pcm, rate, err := wav.Read(bytes.NewReader(data))
		if err != nil {
			problems = append(problems, Problem{File: "sample", Path: name, Message: err.Error()})
			continue
		}
		samples[name] = &synth.Sample{Data: pcm, Rate: rate}
	}
	l.Samples = func(name string) (*synth.Sample, error) {
		if s, ok := samples[name]; ok {
			return s, nil
		}
		return nil, fmt.Errorf("no sample file %q added", name)
	}

	if len(problems) > 0 {
		return nil, problems
	}
	return l, nil
}

// collect lists the problems of err: each voice.FieldError with its file
// and path, anything else (malformed JSON, say) as one problem of file.
func collect(file string, err error) []Problem {
	if err == nil {
		return nil
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		var out []Problem
		for _, e := range multi.Unwrap() {
			out = append(out, collect(file, e)...)
		}
		return out
	}
	var fe *voice.FieldError
	if errors.As(err, &fe) {
		return []Problem{{File: fe.File, Path: fe.Path, Message: fe.Message}}
	}
	return []Problem{{File: file, Message: err.Error()}}
}
