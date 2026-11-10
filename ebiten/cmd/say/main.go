// Command say speaks a line of text through the default audio device, with
// no WAV file in between, and types the text out in the terminal as it is
// spoken.
//
//	say [-voice soft|file.voice.json] [-lang file.lang.json] [-volume 1] "Hello there, traveler!"
//
// It takes the same -voice and -lang flags as voiceplay (in the core
// module). It lives in the ebiten module because playing sound needs an
// audio backend, and the core module has no dependencies. It talks to oto,
// the audio library under Ebiten's audio package, directly: Ebiten's audio
// context opens the device only from a running game loop, which a command
// line program does not have. Ctrl+C stops the line.
//
// As in voiceplay, the line may give words to other voices with markup
// ("He said {gruff}stay back{/}."): a tag names a starter voice or a voice
// file, {name~} is an impression, and -plain speaks the text as written.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/synth"
	"github.com/weakpixel/voice/wav"
)

// Exit codes: 0 success, 1 failure, 2 usage error.
const (
	exitFailure = 1
	exitUsage   = 2
)

const (
	owner voice.Owner = 1

	// bufferSize is both the device buffer and the player's read-ahead:
	// notes start within about this much of their Sink call, so the typed
	// text and the sound stay in step.
	bufferSize = 50 * time.Millisecond
	// tick is how often the speaker is updated from the wall clock.
	tick = 5 * time.Millisecond
	// tail lets the last note and the reverb ring out before the program
	// exits.
	tail = 600 * time.Millisecond
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

type options struct {
	voice, lang string
	rate        int
	volume      float64
	plain       bool
	text        string
	spans       []voice.MarkupSpan // other voices of the line, from its markup
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	o, ok := parse(args, stderr)
	if !ok {
		return exitUsage
	}
	if err := say(ctx, o, stdout); err != nil {
		fmt.Fprintln(stderr, "say:", err)
		return exitFailure
	}
	return 0
}

// parse reads the flags and the text. On a usage error it prints the
// problem to stderr and returns false.
func parse(args []string, stderr io.Writer) (options, bool) {
	fs := flag.NewFlagSet("say", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.StringVar(&o.voice, "voice", "", "starter voice ("+strings.Join(voice.PresetNames(), ", ")+") or a *.voice.json file; default soft")
	fs.StringVar(&o.lang, "lang", "", "a *.lang.json file; default the built-in language")
	fs.IntVar(&o.rate, "rate", 48000, "sample rate in hertz")
	fs.Float64Var(&o.volume, "volume", 1, "playback volume, 0 to 1")
	fs.BoolVar(&o.plain, "plain", false, "speak the text as written, without {voice}…{/} markup")
	fs.Usage = func() {
		fmt.Fprintln(stderr, `usage: say [flags] "line of text"`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return o, false
	}
	o.text = strings.Join(fs.Args(), " ")
	if o.text == "" {
		fmt.Fprintln(stderr, "say: give a line of text")
		fs.Usage()
		return o, false
	}
	if !o.plain {
		var err error
		if o.text, o.spans, err = voice.ParseMarkup(o.text); err != nil {
			fmt.Fprintln(stderr, "say:", err)
			return o, false
		}
	}
	if o.volume < 0 || o.volume > 1 {
		fmt.Fprintln(stderr, "say: -volume must be between 0 and 1")
		return o, false
	}
	return o, true
}

// load returns the voice, its sample loader and the language of the
// options. Bad input fails here, before the audio device is opened.
func load(o options) (*voice.Voice, synth.SampleLoader, *voice.Language, error) {
	v, samples, err := loadVoice(cmp.Or(o.voice, "soft"))
	if err != nil {
		return nil, nil, nil, err
	}
	lang := voice.DefaultLanguage()
	if o.lang != "" {
		if lang, err = voice.LoadLanguageFS(os.DirFS(filepath.Dir(o.lang)), filepath.Base(o.lang)); err != nil {
			return nil, nil, nil, err
		}
	}
	return v, samples, lang, nil
}

// guest is a voice that the line's markup names, on its own owner.
type guest struct {
	name    string
	voice   *voice.Voice
	samples synth.SampleLoader
	owner   voice.Owner
}

// loadGuests loads the voices the markup names, on owners 2, 3, … in order
// of first use, and returns the voice spans of the line.
func loadGuests(ms []voice.MarkupSpan) ([]guest, []voice.VoiceSpan, error) {
	var guests []guest
	var spans []voice.VoiceSpan
	for _, m := range ms {
		i := slices.IndexFunc(guests, func(g guest) bool { return g.name == m.Name })
		if i < 0 {
			v, samples, err := loadVoice(m.Name)
			if err != nil {
				return nil, nil, fmt.Errorf("{%s}: %w", m.Name, err)
			}
			guests = append(guests, guest{m.Name, v, samples, owner + 1 + voice.Owner(len(guests))})
			i = len(guests) - 1
		}
		g := guests[i]
		spans = append(spans, voice.VoiceSpan{Start: m.Start, End: m.End, Voice: g.voice, Owner: g.owner, Impression: m.Impression})
	}
	return guests, spans, nil
}

// say plays the line on the default audio device and returns once it has
// been spoken and has rung out, or when ctx is cancelled.
func say(ctx context.Context, o options, stdout io.Writer) error {
	v, samples, lang, err := load(o)
	if err != nil {
		return err
	}
	guests, spans, err := loadGuests(o.spans)
	if err != nil {
		return err
	}
	stream, err := synth.NewStream(synth.StreamConfig{SampleRate: o.rate})
	if err != nil {
		return err
	}
	if err := stream.SetVoice(owner, v, samples); err != nil {
		return err
	}
	for _, g := range guests {
		if err := stream.SetVoice(g.owner, g.voice, g.samples); err != nil {
			return fmt.Errorf("{%s}: %w", g.name, err)
		}
	}
	device, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   o.rate,
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE, // synth.FormatFloat32 is interleaved little-endian stereo
		BufferSize:   bufferSize,
	})
	if err != nil {
		return fmt.Errorf("audio device: %w", err)
	}
	<-ready
	// The player reads the stream from its own goroutine; this goroutine
	// only calls the stream's Sink methods, through the speaker.
	player := device.NewPlayer(stream)
	defer player.Close()
	// The player reads the stream ahead in chunks of its buffer size, 0.5 s
	// by default. The stream applies the Sink calls that arrived since the
	// last Read at the start of the next block, so with the default every
	// blip of half a second would start (and end) at once, smeared into one
	// burst under the motif. Ebiten's Player sets the same buffer size.
	player.SetBufferSize(playerBufferBytes(o.rate))
	player.SetVolume(o.volume)
	player.Play()

	sp := voice.NewSpeaker(stream, owner)
	sp.Say(v, o.text, voice.SayOptions{Language: lang, Spans: spans})
	shown := 0
	t := time.NewTicker(tick)
	defer t.Stop()
	last := time.Now()
	for !sp.Done() {
		select {
		case <-ctx.Done():
			sp.Stop()                  // releases every note of the line
			time.Sleep(2 * bufferSize) // so the release reaches the device
			fmt.Fprintln(stdout)
			return nil
		case now := <-t.C:
			// Advance by the wall clock, so a late tick does not slow the line.
			sp.Update(now.Sub(last))
			last = now
		}
		if n := sp.Revealed(); n > shown {
			fmt.Fprint(stdout, o.text[shown:n])
			shown = n
		}
		if err := errors.Join(device.Err(), player.Err()); err != nil {
			sp.Stop()
			if shown > 0 {
				fmt.Fprintln(stdout)
			}
			return fmt.Errorf("audio device: %w", err)
		}
	}
	fmt.Fprintln(stdout, o.text[shown:])
	select {
	case <-ctx.Done():
	case <-time.After(tail):
	}
	return nil
}

// playerBufferBytes is the player's read-ahead in bytes: bufferSize of
// float32 stereo frames at rate.
func playerBufferBytes(rate int) int {
	const frameBytes = 8 // synth.FormatFloat32
	return int(int64(rate)*int64(bufferSize)/int64(time.Second)) * frameBytes
}

// loadVoice returns a starter voice by name, or a voice file with a loader
// for the samples next to it (the same rule as voiceplay).
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
	samples := func(file string) (*synth.Sample, error) {
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
	return v, samples, nil
}
