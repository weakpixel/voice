package voice

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"unicode"
)

// Language is a fictional language: the phonemes text is split into, the
// punctuation rules and what happens to characters it does not know. Load
// one with [LoadLanguage] or [LoadLanguageFS], or use [DefaultLanguage].
//
// A Language is immutable after loading and safe to share between
// goroutines and speakers.
type Language struct {
	name            string
	phonemes        []PhonemeDef
	punctuation     []PunctuationDef
	whitespacePause float32
	unknown         UnknownPolicy
	fallback        int // index into phonemes for UnknownFallback
	trie            *trieNode
}

// PhonemeDef is one phoneme of a language.
type PhonemeDef struct {
	Symbol   string   // one or more runes, matched case-insensitively
	Category Category // which instrument plays it
	Pitch    float32  // semitones added to the voice's note
	Length   float32  // relative length of its slot, 1 is one beat
	Velocity float32  // relative loudness, 0 to 1
}

// PunctuationDef is one punctuation rule of a language.
type PunctuationDef struct {
	Symbol  string  // one or more runes, matched case-insensitively
	Pause   float32 // silence in beats
	Contour Contour // what it does to the end of its clause
}

// UnknownPolicy says what happens to a letter or digit the language has no
// phoneme for. Other unknown characters (quotes, dashes, symbols, emoji) are
// always skipped: revealed at once, without sound or time.
type UnknownPolicy uint8

const (
	UnknownSkip     UnknownPolicy = iota // revealed at once, no sound
	UnknownFallback                      // played as the fallback phoneme
	UnknownPause                         // a pause as long as a space
)

var unknownNames = [...]string{"skip", "fallback", "pause"}

// String returns the policy's name as used in language files.
func (p UnknownPolicy) String() string {
	if int(p) < len(unknownNames) {
		return unknownNames[p]
	}
	return fmt.Sprintf("unknown(%d)", p)
}

// Name returns the language's name from its file.
func (l *Language) Name() string { return l.name }

// Phonemes returns a copy of the language's phonemes.
func (l *Language) Phonemes() []PhonemeDef { return append([]PhonemeDef(nil), l.phonemes...) }

// Punctuation returns a copy of the language's punctuation rules.
func (l *Language) Punctuation() []PunctuationDef {
	return append([]PunctuationDef(nil), l.punctuation...)
}

// The defaults of a language file. A space is a short pause, a comma a
// medium one, a sentence end a long one; "?" rises, "!" accents.
const defaultWhitespacePause = 0.5

var defaultPunctuation = []PunctuationDef{
	{Symbol: ",", Pause: 2},
	{Symbol: ";", Pause: 2.5},
	{Symbol: ":", Pause: 2.5},
	{Symbol: ".", Pause: 4, Contour: ContourFall},
	{Symbol: "...", Pause: 5},
	{Symbol: "…", Pause: 5},
	{Symbol: "?", Pause: 4, Contour: ContourRise},
	{Symbol: "!", Pause: 4, Contour: ContourAccent},
}

// languageFile is the JSON form of a *.lang.json file.
type languageFile struct {
	Version     *int               `json:"version"`
	Name        string             `json:"name"`
	Phonemes    []phonemeJSON      `json:"phonemes"`
	Punctuation *[]punctuationJSON `json:"punctuation"`
	Whitespace  *struct {
		Pause *float32 `json:"pause"`
	} `json:"whitespace"`
	Unknown *struct {
		Policy  string `json:"policy"`
		Phoneme string `json:"phoneme"`
	} `json:"unknown"`
}

type phonemeJSON struct {
	Symbol   string   `json:"symbol"`
	Category string   `json:"category"`
	Pitch    float32  `json:"pitch"`
	Length   *float32 `json:"length"`
	Velocity *float32 `json:"velocity"`
}

type punctuationJSON struct {
	Symbol string  `json:"symbol"`
	Pause  float32 `json:"pause"`
	End    string  `json:"end"`
}

// LoadLanguage reads a *.lang.json file. Malformed JSON and unknown fields
// are reported on their own; otherwise every validation error is returned
// together (errors.Join), each with its JSON path.
func LoadLanguage(r io.Reader) (*Language, error) {
	var f languageFile
	if err := decodeStrict(r, &f); err != nil {
		return nil, fmt.Errorf("voice: language: %w", err)
	}
	return f.build()
}

// LoadLanguageFS reads the *.lang.json file name from fsys, see
// [LoadLanguage].
func LoadLanguageFS(fsys fs.FS, name string) (*Language, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("voice: language: %w", err)
	}
	l, err := LoadLanguage(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return l, nil
}

func decodeStrict(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("data after the JSON value")
	}
	return nil
}

func (f *languageFile) build() (*Language, error) {
	errs := fieldErrors{file: "language"}
	l := &Language{name: f.Name, whitespacePause: defaultWhitespacePause}
	switch {
	case f.Version == nil:
		errs.add("version", "missing (must be 0)")
	case *f.Version != 0:
		errs.add("version", "%d is not supported (must be 0)", *f.Version)
	}

	seen := map[string]string{} // folded symbol -> path of its first use
	symbol := func(path, s string) {
		switch {
		case s == "":
			errs.add(path, "empty symbol")
		case strings.IndexFunc(s, unicode.IsSpace) >= 0:
			errs.add(path, "symbol %q contains whitespace", s)
		default:
			k := foldString(s)
			if first, dup := seen[k]; dup {
				errs.add(path, "symbol %q is already defined at %s", s, first)
			} else {
				seen[k] = path
			}
		}
	}

	if len(f.Phonemes) == 0 {
		errs.add("phonemes", "no phonemes")
	}
	for i, p := range f.Phonemes {
		path := fmt.Sprintf("phonemes[%d]", i)
		symbol(path+".symbol", p.Symbol)
		def := PhonemeDef{Symbol: p.Symbol, Pitch: p.Pitch, Length: 1, Velocity: 1}
		if c, ok := parseCategory(p.Category); ok {
			def.Category = c
		} else {
			errs.add(path+".category", "%q is not one of vowel, sonorant, fricative, plosive", p.Category)
		}
		errs.inRange(path+".pitch", p.Pitch, -24, 24)
		if p.Length != nil {
			def.Length = *p.Length
			errs.inRange(path+".length", def.Length, 0.05, 10)
		}
		if p.Velocity != nil {
			def.Velocity = *p.Velocity
			errs.inRange(path+".velocity", def.Velocity, 0, 1)
		}
		l.phonemes = append(l.phonemes, def)
	}

	if f.Punctuation == nil {
		l.punctuation = append(l.punctuation, defaultPunctuation...)
		for _, p := range defaultPunctuation {
			if _, dup := seen[foldString(p.Symbol)]; dup {
				errs.add("punctuation", "default punctuation %q collides with a phoneme; list the punctuation explicitly", p.Symbol)
			}
		}
	} else {
		for i, p := range *f.Punctuation {
			path := fmt.Sprintf("punctuation[%d]", i)
			symbol(path+".symbol", p.Symbol)
			def := PunctuationDef{Symbol: p.Symbol, Pause: p.Pause}
			errs.inRange(path+".pause", p.Pause, 0, 20)
			if c, ok := parseContour(p.End); ok {
				def.Contour = c
			} else {
				errs.add(path+".end", "%q is not one of fall, rise, accent (or empty)", p.End)
			}
			l.punctuation = append(l.punctuation, def)
		}
	}

	if f.Whitespace != nil && f.Whitespace.Pause != nil {
		l.whitespacePause = *f.Whitespace.Pause
		errs.inRange("whitespace.pause", l.whitespacePause, 0, 20)
	}

	if f.Unknown != nil {
		switch f.Unknown.Policy {
		case "skip", "":
			l.unknown = UnknownSkip
		case "pause":
			l.unknown = UnknownPause
		case "fallback":
			l.unknown = UnknownFallback
			l.fallback = -1
			for i, p := range f.Phonemes {
				if p.Symbol == f.Unknown.Phoneme {
					l.fallback = i
				}
			}
			if l.fallback < 0 {
				errs.add("unknown.phoneme", "%q is not a phoneme of the language", f.Unknown.Phoneme)
			}
		default:
			errs.add("unknown.policy", "%q is not one of skip, fallback, pause", f.Unknown.Policy)
		}
		if f.Unknown.Policy != "fallback" && f.Unknown.Phoneme != "" {
			errs.add("unknown.phoneme", "only used with the fallback policy")
		}
	}

	if len(errs.errs) > 0 {
		return nil, errors.Join(errs.errs...)
	}
	l.trie = buildTrie(l)
	return l, nil
}
