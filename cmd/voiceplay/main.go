// Command voiceplay renders the voice library's sounds to WAV files, so they
// can be heard without writing code.
//
//	voiceplay [-voice soft|file.voice.json] [-lang file.lang.json] [-o build/line.wav] "Hello there, traveler!"
//	voiceplay -patches [-voice ...] [-o build/patches.wav]
//
// The first form speaks a line with a voice (a starter voice by name, or a
// voice file whose samples are resolved next to it) in the default language
// or a language file. -patches plays a short rising phrase on every
// instrument of the voice (and four hits on each percussion sound), or,
// without -voice, on every built-in test patch of the synthesizer.
//
// The line may give words to other voices with markup: "He said {gruff}stay
// back{/} and left." A tag names a starter voice or a voice file, like
// -voice; {name~} is an impression. -plain speaks the text as written.
package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	patches, plain   bool
	rate             int
	text             string
	spans            []voice.MarkupSpan // other voices of the line, from its markup
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
	fs.BoolVar(&o.plain, "plain", false, "speak the text as written, without {voice}…{/} markup")
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
	if !o.plain && o.text != "" {
		var err error
		if o.text, o.spans, err = voice.ParseMarkup(o.text); err != nil {
			fmt.Fprintln(stderr, "voiceplay:", err)
			return exitUsage
		}
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
		for inst := range voice.InstPercussion {
			r.phrase(inst)
		}
		for i, p := range v.Percussion {
			r.hits(voice.InstPercussion+voice.Instrument(i), p.Note)
		}
		n := int(voice.InstPercussion) + len(v.Percussion)
		return r.left, r.right, fmt.Sprintf("voice %s, %d instruments", v.Name, n), nil
	}
	lang := voice.DefaultLanguage()
	if o.lang != "" {
		if lang, err = voice.LoadLanguageFS(os.DirFS(filepath.Dir(o.lang)), filepath.Base(o.lang)); err != nil {
			return nil, nil, "", err
		}
	}
	spans, err := guestSpans(s, o.spans)
	if err != nil {
		return nil, nil, "", err
	}
	n := r.speak(v, lang, o.text, spans)
	return r.left, r.right, fmt.Sprintf("voice %s, %d notes", v.Name, n), nil
}

// guestSpans loads the voices the markup names and sets their instruments
// on owners 2, 3, … in order of first use.
func guestSpans(s *synth.Stream, ms []voice.MarkupSpan) ([]voice.VoiceSpan, error) {
	owners := map[string]voice.Owner{}
	voices := map[string]*voice.Voice{}
	var spans []voice.VoiceSpan
	for _, m := range ms {
		if _, ok := owners[m.Name]; !ok {
			v, load, err := loadVoice(m.Name)
			if err != nil {
				return nil, fmt.Errorf("{%s}: %w", m.Name, err)
			}
			o := owner + 1 + voice.Owner(len(owners))
			if err := s.SetVoice(o, v, load); err != nil {
				return nil, fmt.Errorf("{%s}: %w", m.Name, err)
			}
			owners[m.Name], voices[m.Name] = o, v
		}
		spans = append(spans, voice.VoiceSpan{Start: m.Start, End: m.End, Voice: voices[m.Name], Owner: owners[m.Name], Impression: m.Impression})
	}
	return spans, nil
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
	elapsed     time.Duration
	left, right []float32
}

// advance renders d more. Frames are counted from the total time, so many
// short steps do not drift at rates that are not a multiple of 1000.
func (r *renderer) advance(d time.Duration) {
	r.elapsed += d
	n := int(r.elapsed.Seconds()*float64(r.rate)) - len(r.left)
	l, rt := make([]float32, n), make([]float32, n)
	r.s.Render(l, rt)
	r.left, r.right = append(r.left, l...), append(r.right, rt...)
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
		r.s.NoteOff(owner, inst, note)
		r.advance(noteGap)
	}
	r.advance(patchGap)
}

// hits plays a percussion sound four times at its note.
func (r *renderer) hits(inst voice.Instrument, note float32) {
	for range 4 {
		r.s.NoteOn(owner, inst, note, 0.8, 0)
		r.advance(noteLen)
		r.s.NoteOff(owner, inst, note)
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

// speak plays a line through a voice.Speaker, updating it every
// millisecond the way a game loop would, and returns the number of notes.
func (r *renderer) speak(v *voice.Voice, lang *voice.Language, text string, spans []voice.VoiceSpan) int {
	sp := voice.NewSpeaker(r.s, owner)
	sp.Say(v, text, voice.SayOptions{Language: lang, Spans: spans})
	for !sp.Done() {
		r.advance(time.Millisecond)
		sp.Update(time.Millisecond)
	}
	r.advance(tail)
	return len(sp.Timeline().Notes)
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
