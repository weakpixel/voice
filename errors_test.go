package voice

import (
	"errors"
	"strings"
	"testing"
)

// Every validation error of a file unwraps to a FieldError with the JSON
// path of its value, and its text is unchanged.
func TestFieldErrors(t *testing.T) {
	_, verr := LoadVoice(strings.NewReader(`{"version": 0, "speed": 100, "instruments": {
		"vowel": {"sustain": 2}, "trumpet": {}}, "rhythm": {"bpm": 10}}`))
	_, lerr := LoadLanguage(strings.NewReader(`{"version": 0, "phonemes": [{"symbol": "a", "category": "tone"}]}`))
	v, _ := Preset("soft")
	v.Pan = 4
	cases := []struct {
		name  string
		err   error
		file  string
		paths []string
	}{
		{"voice", verr, "voice", []string{"speed", "instruments.vowel.sustain", "instruments.trumpet", "rhythm.bpm"}},
		{"language", lerr, "language", []string{"phonemes[0].category"}},
		{"validate", v.Validate(), "voice", []string{"pan"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.err == nil {
				t.Fatal("no error")
			}
			joined, ok := c.err.(interface{ Unwrap() []error })
			if !ok {
				t.Fatalf("%T does not unwrap to a list", c.err)
			}
			var paths []string
			for _, e := range joined.Unwrap() {
				var fe *FieldError
				if !errors.As(e, &fe) {
					t.Fatalf("%v is not a FieldError", e)
				}
				if fe.File != c.file {
					t.Errorf("%v: file %q, want %q", fe, fe.File, c.file)
				}
				if want := "voice: " + c.file + ": " + fe.Path + ": " + fe.Message; fe.Error() != want {
					t.Errorf("text %q, want %q", fe.Error(), want)
				}
				paths = append(paths, fe.Path)
			}
			for _, p := range c.paths {
				if !strings.Contains(strings.Join(paths, "|")+"|", p+"|") {
					t.Errorf("no error at %q in %v", p, paths)
				}
			}
		})
	}
}
