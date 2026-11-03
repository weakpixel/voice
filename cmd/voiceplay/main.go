// Command voiceplay renders the voice library's sounds to WAV files, so they
// can be heard without writing code.
//
//	voiceplay [-voice soft|file.voice.json] [-lang file.lang.json] [-o build/line.wav] "Hello there, traveler!"
//	voiceplay -patches [-voice ...] [-o build/patches.wav]
//
// The first form speaks a line with a voice (a starter voice by name, or a
// voice file whose samples are resolved next to it) in the default language
// or a language file. -patches plays a short rising phrase on every
// instrument of the voice, or, without -voice, on every built-in test patch
// of the synthesizer.
package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

const owner voice.Owner = 1

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type options struct {
	voice, lang, out string
	patches          bool
	rate             int
	text             string
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("voiceplay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.StringVar(&o.voice, "voice", "", "starter voice ("+strings.Join(voice.PresetNames(), ", ")+") or a *.voice.json file; default soft")
	fs.StringVar(&o.lang, "lang", "", "a *.lang.json file; default the built-in language")
	fs.BoolVar(&o.patches, "patches", false, "play a phrase on every instrument of the voice (or every built-in test patch without -voice)")
	fs.StringVar(&o.out, "o", "", "output WAV file; default build/line.wav or build/patches.wav")
	fs.IntVar(&o.rate, "rate", 48000, "sample rate in hertz")
	fs.Usage = func() {
		fmt.Fprintln(stderr, `usage: voiceplay [flags] "line of text"`)
		fmt.Fprintln(stderr, "       voiceplay -patches [flags]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	o.text = strings.Join(fs.Args(), " ")
	if o.patches == (o.text != "") {
		fmt.Fprintln(stderr, "voiceplay: give either a line of text or -patches")
		fs.Usage()
		return exitUsage
	}
	if o.out == "" {
		o.out = map[bool]string{false: "build/line.wav", true: "build/patches.wav"}[o.patches]
	}
	left, right, what, err := render(o)
	if err == nil {
		err = writeWAV(o.out, o.rate, left, right)
	}
	if err != nil {
		fmt.Fprintln(stderr, "voiceplay:", err)
		return exitFailure
	}
	fmt.Fprintf(stdout, "wrote %s (%s, %.1f s)\n", o.out, what, float64(len(left))/float64(o.rate))
	return 0
}

// render returns the rendered audio and a short description of it.
func render(o options) (left, right []float32, what string, err error) {
	s, err := synth.NewStream(synth.StreamConfig{SampleRate: o.rate})
	if err != nil {
		return nil, nil, "", err
	}
	r := &renderer{s: s, rate: o.rate}
	if o.patches && o.voice == "" {
		err := r.testPatches()
		return r.left, r.right, fmt.Sprintf("%d test patches", len(testPatches())), err
	}
	v, load, err := loadVoice(cmp.Or(o.voice, "soft"))
	if err != nil {
		return nil, nil, "", err
	}
	if err := s.SetVoice(owner, v, load); err != nil {
		return nil, nil, "", err
	}
	if o.patches {
		for inst := range voice.Instrument(voice.InstrumentCount) {
			r.phrase(inst)
		}
		return r.left, r.right, fmt.Sprintf("voice %s, %d instruments", v.Name, voice.InstrumentCount), nil
	}
	lang := voice.DefaultLanguage()
	if o.lang != "" {
		if lang, err = voice.LoadLanguageFS(os.DirFS(filepath.Dir(o.lang)), filepath.Base(o.lang)); err != nil {
			return nil, nil, "", err
		}
	}
	tl := voice.ComposeText(v, lang, o.text)
	r.timeline(tl)
	return r.left, r.right, fmt.Sprintf("voice %s, %d notes", v.Name, len(tl.Notes)), nil
}

// loadVoice returns a starter voice by name, or a voice file with a loader
// for samples next to it.
func loadVoice(name string) (*voice.Voice, synth.SampleLoader, error) {
	if !strings.HasSuffix(name, ".json") {
		v, err := voice.Preset(name)
		return v, nil, err
	}
	dir := os.DirFS(filepath.Dir(name))
	v, err := voice.LoadVoiceFS(dir, filepath.Base(name))
	if err != nil {
		return nil, nil, err
	}
	cache := map[string]*synth.Sample{}
	load := func(file string) (*synth.Sample, error) {
		if s, ok := cache[file]; ok {
			return s, nil
		}
		f, err := dir.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data, rate, err := wav.Read(f)
		if err != nil {
			return nil, err
		}
		s := &synth.Sample{Data: data, Rate: rate}
		cache[file] = s
		return s, nil
	}
	return v, load, nil
}

// renderer drives a stream the way a game would: sink calls, then render up
// to the next event.
type renderer struct {
	s           *synth.Stream
	rate        int
	left, right []float32
}

func (r *renderer) advance(d time.Duration) {
	n := int(d.Seconds() * float64(r.rate))
	l, rt := make([]float32, n), make([]float32, n)
	r.s.Render(l, rt)
	r.left, r.right = append(r.left, l...), append(r.right, rt...)
}

// advanceTo renders until t from the start of the output.
func (r *renderer) advanceTo(t time.Duration) {
	now := time.Duration(float64(len(r.left)) / float64(r.rate) * float64(time.Second))
	if t > now {
		r.advance(t - now)
	}
}

// phrase is played on every patch: a rising arpeggio of short notes.
var phrase = []float32{0, 4, 7, 12, 7}

const (
	noteLen  = 110 * time.Millisecond
	noteGap  = 40 * time.Millisecond
	patchGap = 400 * time.Millisecond
	tail     = 500 * time.Millisecond
)

func (r *renderer) phrase(inst voice.Instrument) {
	for _, step := range phrase {
		note := 60 + step
		r.s.NoteOn(owner, inst, note, 0.8, 0)
		r.advance(noteLen)
		r.s.NoteOff(owner, note)
		r.advance(noteGap)
	}
	r.advance(patchGap)
}

func (r *renderer) testPatches() error {
	for _, np := range testPatches() {
		if err := r.s.SetPatch(owner, voice.InstVowel, np.patch); err != nil {
			return fmt.Errorf("patch %s: %w", np.name, err)
		}
		r.phrase(voice.InstVowel)
	}
	return nil
}

// timeline plays tl: note offs before note ons at the same time, so a
// repeated pitch is not cut by its predecessor's off.
func (r *renderer) timeline(tl voice.Timeline) {
	type event struct {
		at   time.Duration
		on   bool
		note voice.Note
	}
	var events []event
	for _, n := range tl.Notes {
		events = append(events, event{n.On, true, n}, event{n.Off, false, n})
	}
	slices.SortStableFunc(events, func(a, b event) int {
		if c := cmp.Compare(a.at, b.at); c != 0 {
			return c
		}
		if a.on == b.on {
			return 0
		}
		if a.on {
			return 1
		}
		return -1
	})
	for _, e := range events {
		r.advanceTo(e.at)
		if e.on {
			r.s.NoteOn(owner, e.note.Instrument, e.note.Note, e.note.Velocity, e.note.Pan)
		} else {
			r.s.NoteOff(owner, e.note.Note)
		}
	}
	r.advanceTo(tl.Duration + tail)
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
