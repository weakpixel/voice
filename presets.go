package voice

import (
	"embed"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"
)

//go:embed presets/*.json
var presetFS embed.FS

// DefaultLanguage returns the built-in language: Latin letters with common
// digraphs (sh, ch, th, ng, ai, ee, oo, ou, ck), the default punctuation,
// and unknown letters and digits played as "a". It is parsed once and
// shared; a Language is immutable.
func DefaultLanguage() *Language { return defaultLanguage() }

var defaultLanguage = sync.OnceValue(func() *Language {
	l, err := LoadLanguageFS(presetFS, "presets/default.lang.json")
	if err != nil {
		panic("voice: embedded default language: " + err.Error()) // caught by tests
	}
	return l
})

// Preset returns a new copy of a starter voice: "soft", "chirpy" or
// "gruff" (see [PresetNames]). It returns an error for an unknown name.
func Preset(name string) (*Voice, error) {
	if !slices.Contains(PresetNames(), name) {
		return nil, fmt.Errorf("voice: unknown preset %q (one of %s)", name, strings.Join(PresetNames(), ", "))
	}
	return LoadVoiceFS(presetFS, path.Join("presets", name+".voice.json"))
}

// PresetNames returns the names of the starter voices.
func PresetNames() []string { return []string{"soft", "chirpy", "gruff"} }
