// Package dialogue loads and plays a conversation: a list of lines, each said
// by one speaker, each speaker with its own voice, with optional timing per
// line. A conversation is a *.dialogue.json file; [Load] reads one into a
// [Script], [LoadVoices] resolves the voices it names, and a [Player] speaks
// it on a [voice.Sink] one line after the other.
//
// The package uses the standard library and the voice package only.
package dialogue

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/weakpixel/voice"
)

// Advance says how a finished line gives way to the next one.
type Advance string

const (
	// AdvanceInput waits for [Player.Next] (a click or a key).
	AdvanceInput Advance = "input"
	// AdvanceAuto goes on by itself after the line's hold time.
	AdvanceAuto Advance = "auto"
)

// The defaults and limits of the optional fields.
const (
	DefaultHold  = 1.0 // seconds a finished line stays before an auto advance
	MaxDelay     = 10  // seconds, the most a line's delay or hold can be
	MinLineSpeed = 0.25
	MaxLineSpeed = 4
)

// Script is a loaded dialogue. Load fills in every default, so the values
// are what the player uses: a line's Advance and Hold are its own or else
// the script's, Speed is 1 unless the file says otherwise.
type Script struct {
	// Name is the dialogue's name (may be empty).
	Name string
	// Advance and Hold are the dialogue's defaults for its lines.
	Advance Advance
	Hold    float32
	// Speakers are in file order; speaker i plays as owner i + 1.
	Speakers []Speaker
	Lines    []Line
	// Dir is the folder of the file inside the file system it was loaded
	// from ("." for [Load]); [LoadVoices] resolves voice file names there.
	Dir string
}

// Speaker is one character of a dialogue.
type Speaker struct {
	// ID is unique in the script and matches [a-z0-9_-]+.
	ID string
	// Name is what a dialogue box shows; it defaults to the ID.
	Name string
	// Voice is a starter voice name or a *.voice.json file name relative to
	// the dialogue file.
	Voice string
	// Pan overrides the voice's pan for this speaker, -1 to 1; nil keeps
	// the voice's own.
	Pan *float32
}

// Line is one line of a dialogue.
type Line struct {
	// Speaker is the ID of a speaker of the script.
	Speaker string
	Text    string
	// Delay is the time in seconds before the line starts, 0 to 10.
	Delay float32
	// Speed is [voice.SayOptions].Speed, 0.25 to 4.
	Speed   float32
	Instant bool
	// Advance and Hold: see [Script].
	Advance Advance
	Hold    float32
}

type scriptFile struct {
	Version  *int          `json:"version"`
	Name     string        `json:"name"`
	Advance  string        `json:"advance"`
	Hold     *float32      `json:"hold"`
	Speakers []speakerFile `json:"speakers"`
	Lines    []lineFile    `json:"lines"`
}

type speakerFile struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Voice string   `json:"voice"`
	Pan   *float32 `json:"pan"`
}

type lineFile struct {
	Speaker string   `json:"speaker"`
	Text    string   `json:"text"`
	Delay   float32  `json:"delay"`
	Speed   *float32 `json:"speed"`
	Instant bool     `json:"instant"`
	Advance string   `json:"advance"`
	Hold    *float32 `json:"hold"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// Load reads a *.dialogue.json file. Malformed JSON and unknown fields are
// reported on their own; otherwise every validation error is returned
// together (errors.Join) as a [voice.FieldError] with File "dialogue" and
// its JSON path. Voices are not resolved, see [LoadVoices].
func Load(r io.Reader) (*Script, error) {
	var f scriptFile
	dec := newDecoder(r)
	if err := dec.decode(&f); err != nil {
		return nil, fmt.Errorf("voice: dialogue: %w", err)
	}
	return f.build()
}

// LoadFS reads the *.dialogue.json file name from fsys, see [Load]. The
// script remembers name's folder for [LoadVoices]. Errors are prefixed with
// the file name.
func LoadFS(fsys fs.FS, name string) (*Script, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("voice: dialogue: %w", err)
	}
	s, err := Load(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	s.Dir = path.Dir(name)
	return s, nil
}

type problems struct{ errs []error }

func (p *problems) add(path, format string, args ...any) {
	p.errs = append(p.errs, &voice.FieldError{File: "dialogue", Path: path, Message: fmt.Sprintf(format, args...)})
}

func (p *problems) inRange(path string, v, lo, hi float32) {
	switch {
	case v != v || v > 3.4e38 || v < -3.4e38:
		p.add(path, "not a finite number")
	case v < lo || v > hi:
		p.add(path, "%g is outside %g to %g", v, lo, hi)
	}
}

// advance validates an advance value; "" is allowed and returned as "".
func (p *problems) advance(path, s string) Advance {
	switch Advance(s) {
	case "", AdvanceInput, AdvanceAuto:
		return Advance(s)
	}
	p.add(path, "%q is not \"input\" or \"auto\"", s)
	return ""
}

// voiceRefProblem says why ref is not a starter voice or a voice file name.
func voiceRefProblem(ref string) string {
	switch {
	case ref == "":
		return "missing (a starter voice or a *.voice.json file)"
	case slices.Contains(voice.PresetNames(), ref):
		return ""
	case !strings.HasSuffix(ref, ".voice.json"):
		return fmt.Sprintf("%q is neither a starter voice (%s) nor a *.voice.json file", ref, strings.Join(voice.PresetNames(), ", "))
	case !fs.ValidPath(ref):
		return fmt.Sprintf("%q is not a relative path inside the dialogue's folder", ref)
	}
	return ""
}

func (f *scriptFile) build() (*Script, error) {
	var p problems
	switch {
	case f.Version == nil:
		p.add("version", "missing (must be 0)")
	case *f.Version != 0:
		p.add("version", "%d is not supported (must be 0)", *f.Version)
	}
	s := &Script{Name: f.Name, Hold: DefaultHold, Dir: "."}
	s.Advance = p.advance("advance", f.Advance)
	if s.Advance == "" {
		s.Advance = AdvanceInput
	}
	if f.Hold != nil {
		p.inRange("hold", *f.Hold, 0, MaxDelay)
		s.Hold = *f.Hold
	}

	ids := map[string]bool{}
	if len(f.Speakers) == 0 {
		p.add("speakers", "at least one speaker is needed")
	}
	for i, sf := range f.Speakers {
		at := fmt.Sprintf("speakers[%d]", i)
		switch {
		case sf.ID == "":
			p.add(at+".id", "missing")
		case !idPattern.MatchString(sf.ID):
			p.add(at+".id", "%q is not made of a-z, 0-9, _ and -", sf.ID)
		case ids[sf.ID]:
			p.add(at+".id", "%q is used twice", sf.ID)
		}
		ids[sf.ID] = true
		if msg := voiceRefProblem(sf.Voice); msg != "" {
			p.add(at+".voice", "%s", msg)
		}
		if sf.Pan != nil {
			p.inRange(at+".pan", *sf.Pan, -1, 1)
		}
		sp := Speaker{ID: sf.ID, Name: sf.Name, Voice: sf.Voice, Pan: sf.Pan}
		if sp.Name == "" {
			sp.Name = sp.ID
		}
		s.Speakers = append(s.Speakers, sp)
	}

	if len(f.Lines) == 0 {
		p.add("lines", "at least one line is needed")
	}
	for i, lf := range f.Lines {
		at := fmt.Sprintf("lines[%d]", i)
		switch {
		case lf.Speaker == "":
			p.add(at+".speaker", "missing")
		case !ids[lf.Speaker]:
			p.add(at+".speaker", "unknown speaker %q", lf.Speaker)
		}
		if strings.TrimSpace(lf.Text) == "" {
			p.add(at+".text", "missing or empty")
		}
		l := Line{Speaker: lf.Speaker, Text: lf.Text, Delay: lf.Delay, Speed: 1, Instant: lf.Instant, Hold: s.Hold}
		p.inRange(at+".delay", lf.Delay, 0, MaxDelay)
		if lf.Speed != nil {
			p.inRange(at+".speed", *lf.Speed, MinLineSpeed, MaxLineSpeed)
			l.Speed = *lf.Speed
		}
		if lf.Hold != nil {
			p.inRange(at+".hold", *lf.Hold, 0, MaxDelay)
			l.Hold = *lf.Hold
		}
		l.Advance = p.advance(at+".advance", lf.Advance)
		if l.Advance == "" {
			l.Advance = s.Advance
		}
		s.Lines = append(s.Lines, l)
	}
	if err := errors.Join(p.errs...); err != nil {
		return nil, err
	}
	return s, nil
}

// SpeakerIndex returns the index of the speaker with this ID, or -1.
func (s *Script) SpeakerIndex(id string) int {
	return slices.IndexFunc(s.Speakers, func(sp Speaker) bool { return sp.ID == id })
}

func isStarter(ref string) bool { return slices.Contains(voice.PresetNames(), ref) }

func itoa(i int) string { return fmt.Sprint(i) }

func joinProblems(errs []error) error { return errors.Join(errs...) }
