// Command voiceplay renders the voice library's sounds to WAV files, so they
// can be heard without writing code.
//
//	voiceplay -patches -o build/patches.wav
//
// renders every built-in test patch in turn: a short rising phrase per patch,
// then a pause. Rendering a line of dialogue with a voice comes with the
// language and voice files.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/synth"
	"github.com/weakpixel/voice/wav"
)

// Exit codes: 0 success, 1 failure, 2 usage error.
const (
	exitFailure = 1
	exitUsage   = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("voiceplay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	patches := fs.Bool("patches", false, "render every built-in test patch in turn")
	out := fs.String("o", "build/patches.wav", "output WAV file")
	rate := fs.Int("rate", 48000, "sample rate in hertz")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: voiceplay -patches [-o file.wav] [-rate hz]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if !*patches || fs.NArg() > 0 {
		fmt.Fprintln(stderr, "voiceplay: only -patches is built yet; rendering a line needs the language and voice files")
		fs.Usage()
		return exitUsage
	}
	left, right, err := renderPatches(*rate)
	if err != nil {
		fmt.Fprintln(stderr, "voiceplay:", err)
		return exitFailure
	}
	if err := writeWAV(*out, *rate, left, right); err != nil {
		fmt.Fprintln(stderr, "voiceplay:", err)
		return exitFailure
	}
	fmt.Fprintf(stdout, "wrote %s (%d patches, %.1f s)\n", *out, len(testPatches()), float64(len(left))/float64(*rate))
	return 0
}

// phrase is played on every patch: a rising arpeggio of short notes.
var phrase = []float32{60, 64, 67, 72, 67}

const (
	noteLen  = 110 * time.Millisecond
	noteGap  = 40 * time.Millisecond
	patchGap = 400 * time.Millisecond
)

// renderPatches plays the phrase on each test patch, one after another, the
// way a game would: note on, render, note off, render.
func renderPatches(rate int) (left, right []float32, err error) {
	s, err := synth.NewStream(synth.StreamConfig{SampleRate: rate})
	if err != nil {
		return nil, nil, err
	}
	const owner voice.Owner = 1
	step := func(d time.Duration) {
		n := int(d.Seconds() * float64(rate))
		l, r := make([]float32, n), make([]float32, n)
		s.Render(l, r)
		left, right = append(left, l...), append(right, r...)
	}
	for _, np := range testPatches() {
		if err := s.SetPatch(owner, voice.InstVowel, np.patch); err != nil {
			return nil, nil, fmt.Errorf("patch %s: %w", np.name, err)
		}
		for _, note := range phrase {
			s.NoteOn(owner, voice.InstVowel, note, 0.8, 0)
			step(noteLen)
			s.NoteOff(owner, note)
			step(noteGap)
		}
		step(patchGap)
	}
	return left, right, nil
}

func writeWAV(path string, rate int, left, right []float32) (err error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	return wav.WriteStereo(f, rate, left, right)
}
