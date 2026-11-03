package voice

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"slices"
	"strings"
)

// Voice is how one character speaks: the instruments, pitch, speed and
// variation. Load one with [LoadVoice] or [LoadVoiceFS], or start from a
// [Preset]. Fields may be changed after loading (for example Pitch to tell
// two guards with the same voice apart); [Voice.Validate] checks them.
type Voice struct {
	Name     string
	Language string // name the caller resolves; "" means DefaultLanguage

	BaseNote  float32 // the note a phoneme of pitch 0 plays, 24 to 108
	Pitch     float32 // semitones added to every note, -24 to 24
	Variation float32 // random pitch spread in semitones (plus or minus), 0 to 12
	Velocity  float32 // loudness of the phonemes, 0 to 1
	Length    float32 // note length as a share of its slot, 0.05 to 1
	Speed     float32 // beats (phonemes of length 1) per second, 1 to 60
	Pan       float32 // -1 (left) to 1 (right)
	// MotifVolume is the velocity of the motif notes, 0 to 1; 0 turns the
	// motif off.
	MotifVolume float32
	Seed        uint64 // mixed with a hash of the text: same voice and text, same line

	// Instruments are indexed by Instrument. Loading fills instruments the
	// file leaves out with the vowel's.
	Instruments [InstPercussion]InstrumentSpec

	// Percussion are the sounds the rhythm's beat plays, sorted by name;
	// sound i plays on instrument InstPercussion + i. At most
	// MaxPercussion. Loading adds the built-in sounds the beat names but
	// the file does not define.
	Percussion []Percussion

	// Rhythm puts the voice on a beat grid; nil speaks freely.
	Rhythm *Rhythm
}

// voiceFile is the JSON form of a *.voice.json file.
type voiceFile struct {
	Version     *int                       `json:"version"`
	Name        string                     `json:"name"`
	Language    string                     `json:"language"`
	BaseNote    *float32                   `json:"baseNote"`
	Pitch       float32                    `json:"pitch"`
	Variation   *float32                   `json:"variation"`
	Velocity    *float32                   `json:"velocity"`
	Length      *float32                   `json:"length"`
	Speed       *float32                   `json:"speed"`
	Pan         float32                    `json:"pan"`
	MotifVolume *float32                   `json:"motifVolume"`
	Seed        uint64                     `json:"seed"`
	Instruments map[string]InstrumentSpec  `json:"instruments"`
	Percussion  map[string]json.RawMessage `json:"percussion"`
	Rhythm      *rhythmFile                `json:"rhythm"`
}

// The defaults of the optional voice file fields.
const (
	DefaultBaseNote    = 64
	DefaultVariation   = 1
	DefaultVelocity    = 0.8
	DefaultLength      = 0.8
	DefaultSpeed       = 14
	DefaultMotifVolume = 0.2
)

// LoadVoice reads a *.voice.json file. Malformed JSON and unknown fields are
// reported on their own; otherwise every validation error is returned
// together (errors.Join), each with its JSON path. Sample references are not
// resolved; that is the synthesizer's caller's job.
func LoadVoice(r io.Reader) (*Voice, error) {
	var f voiceFile
	if err := decodeStrict(r, &f); err != nil {
		return nil, fmt.Errorf("voice: voice: %w", err)
	}
	return f.build()
}

// LoadVoiceFS reads the *.voice.json file name from fsys, see [LoadVoice].
func LoadVoiceFS(fsys fs.FS, name string) (*Voice, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("voice: voice: %w", err)
	}
	v, err := LoadVoice(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return v, nil
}

func orDefault(p *float32, def float32) float32 {
	if p == nil {
		return def
	}
	return *p
}

func (f *voiceFile) build() (*Voice, error) {
	errs := fieldErrors{prefix: "voice: voice"}
	switch {
	case f.Version == nil:
		errs.add("version", "missing (must be 0)")
	case *f.Version != 0:
		errs.add("version", "%d is not supported (must be 0)", *f.Version)
	}
	v := &Voice{
		Name:        f.Name,
		Language:    f.Language,
		BaseNote:    orDefault(f.BaseNote, DefaultBaseNote),
		Pitch:       f.Pitch,
		Variation:   orDefault(f.Variation, DefaultVariation),
		Velocity:    orDefault(f.Velocity, DefaultVelocity),
		Length:      orDefault(f.Length, DefaultLength),
		Speed:       orDefault(f.Speed, DefaultSpeed),
		Pan:         f.Pan,
		MotifVolume: orDefault(f.MotifVolume, DefaultMotifVolume),
		Seed:        f.Seed,
	}
	vowel, ok := f.Instruments[InstVowel.String()]
	if !ok {
		errs.add("instruments.vowel", "missing (the other instruments default to it)")
	}
	for i := range v.Instruments {
		v.Instruments[i] = vowel
	}
	names := make([]string, 0, len(f.Instruments))
	for name := range f.Instruments {
		names = append(names, name)
	}
	slices.Sort(names) // report errors in a stable order
	for _, name := range names {
		i := slices.Index(instrumentNames[:], name)
		if i < 0 {
			errs.add("instruments."+name, "unknown instrument (one of vowel, sonorant, fricative, plosive, motif)")
			continue
		}
		v.Instruments[i] = f.Instruments[name]
	}
	v.buildPercussion(&errs, f.Percussion)
	if f.Rhythm != nil {
		v.Rhythm = f.Rhythm.build(&errs)
		v.addBuiltinBeatSounds()
	}
	v.validate(&errs, names)
	if len(errs.errs) > 0 {
		return nil, errors.Join(errs.errs...)
	}
	return v, nil
}

// Validate reports every out-of-range field of v together, or nil.
func (v *Voice) Validate() error {
	errs := fieldErrors{prefix: "voice: voice"}
	v.validate(&errs, instrumentNames[:])
	return errors.Join(errs.errs...)
}

// validate checks the scalar fields and the instruments named in check.
func (v *Voice) validate(errs *fieldErrors, check []string) {
	errs.inRange("baseNote", v.BaseNote, 24, 108)
	errs.inRange("pitch", v.Pitch, -24, 24)
	errs.inRange("variation", v.Variation, 0, 12)
	errs.inRange("velocity", v.Velocity, 0, 1)
	errs.inRange("length", v.Length, 0.05, 1)
	errs.inRange("speed", v.Speed, 1, 60)
	errs.inRange("pan", v.Pan, -1, 1)
	errs.inRange("motifVolume", v.MotifVolume, 0, 1)
	for _, name := range check {
		if i := slices.Index(instrumentNames[:], name); i >= 0 {
			v.Instruments[i].validate(errs, "instruments."+name)
		}
	}
	v.validatePercussion(errs)
	if v.Rhythm != nil {
		v.Rhythm.validate(errs, v)
	}
}

// buildPercussion loads the voice file's percussion entries, sorted by
// name.
func (v *Voice) buildPercussion(errs *fieldErrors, entries map[string]json.RawMessage) {
	names := slices.Sorted(maps.Keys(entries))
	for _, name := range names {
		if p, ok := loadPercussion(errs, name, entries[name]); ok {
			v.Percussion = append(v.Percussion, p)
		}
	}
}

// addBuiltinBeatSounds adds the built-in sounds the beat names but the
// percussion list lacks, keeping the list sorted by name.
func (v *Voice) addBuiltinBeatSounds() {
	for _, t := range v.Rhythm.Beat {
		if v.percussionIndex(t.Sound) >= 0 {
			continue
		}
		if p, ok := BuiltinPercussion(t.Sound); ok {
			v.Percussion = append(v.Percussion, p)
		}
	}
	slices.SortFunc(v.Percussion, func(a, b Percussion) int { return strings.Compare(a.Name, b.Name) })
}
