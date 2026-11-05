package session

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/weakpixel/voice"
)

// field is one entry of editor/web/fields.json, the metadata the visual
// editor's form is generated from.
type field struct {
	Key           string   `json:"key"`
	Kind          string   `json:"kind"`
	Min           *float64 `json:"min"`
	Max           *float64 `json:"max"`
	Default       any      `json:"default"`
	Options       string   `json:"options"`
	ZeroIsDefault bool     `json:"zeroIsDefault"`
	Sample        bool     `json:"sample"`
	ErrorPath     string   `json:"errorPath"` // "*": any problem (a JSON decoding error has no path)
	MaxBelow      *float64 `json:"maxBelow"`  // the valid range ends below this value
	MaxItems      int      `json:"maxItems"`

	// What only the page uses.
	Label string   `json:"label"`
	Group string   `json:"group"`
	Help  string   `json:"help"`
	Unit  string   `json:"unit"`
	Step  float64  `json:"step"`
	UIMin *float64 `json:"uiMin"`
	UIMax *float64 `json:"uiMax"`
}

type fieldsFile struct {
	Options    map[string][]any             `json:"options"`
	OptionHelp map[string]map[string]string `json:"optionHelp"`
	Voice      []field                      `json:"voice"`
	Instrument []field                      `json:"instrument"`
	Percussion []field                      `json:"percussion"`
	Rhythm     []field                      `json:"rhythm"`
}

func readFields(t *testing.T) fieldsFile {
	t.Helper()
	data, err := os.ReadFile("../web/fields.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fieldsFile
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		t.Fatal(err)
	}
	return f
}

// base is a valid voice with every section the fields live in: an
// oscillator vowel, a sample motif, a percussion sound and a rhythm.
func base() map[string]any {
	return map[string]any{
		"version": 0,
		"instruments": map[string]any{
			"vowel": map[string]any{"wave": "square"},
			"motif": map[string]any{"sample": "hum.wav"},
		},
		"percussion": map[string]any{"boom": map[string]any{"base": "tom"}},
		"rhythm":     map[string]any{},
	}
}

// with returns base() with value at the dotted path.
func with(path string, value any) string {
	doc := base()
	m := doc
	keys := strings.Split(path, ".")
	for _, k := range keys[:len(keys)-1] {
		next, ok := m[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[k] = next
		}
		m = next
	}
	m[keys[len(keys)-1]] = value
	data, _ := json.Marshal(doc)
	return string(data)
}

func problemAt(problems []Problem, path string) bool {
	return slices.ContainsFunc(problems, func(p Problem) bool { return p.Path == path || path == "*" })
}

// TestFieldsMatchTheLibrary keeps fields.json honest: every numeric field
// loads at its min and max and fails just outside them at its own path,
// every select's options load, and every instrument field of the format
// has an entry.
func TestFieldsMatchTheLibrary(t *testing.T) {
	f := readFields(t)
	if v, p := LoadVoice(with("name", "x")); v == nil {
		t.Fatalf("base voice does not load: %v", p)
	}
	sections := []struct {
		prefix string
		fields []field
	}{
		{"", f.Voice},
		{"instruments.vowel.", f.Instrument},
		{"percussion.boom.", f.Instrument},
		{"percussion.boom.", f.Percussion},
		{"rhythm.", f.Rhythm},
	}
	for _, s := range sections {
		for _, fd := range s.fields {
			prefix := s.prefix
			if fd.Sample && prefix == "instruments.vowel." {
				prefix = "instruments.motif."
			} else if fd.Sample {
				continue // a percussion sample needs a base without a wave; covered by the motif
			}
			path := prefix + fd.Key
			t.Run(path, func(t *testing.T) { checkField(t, f, fd, path, prefix) })
		}
	}
	// Every instrument field of the file format has an entry.
	var keys []string
	for _, fd := range f.Instrument {
		keys = append(keys, fd.Key)
	}
	typ := reflect.TypeFor[voice.InstrumentSpec]()
	for i := range typ.NumField() {
		tag, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if !slices.Contains(keys, tag) {
			t.Errorf("instrument field %q has no entry in fields.json", tag)
		}
	}
	// Options lists match the library.
	opts := func(name string) []string {
		var out []string
		for _, o := range f.Options[name] {
			out = append(out, fmt.Sprint(o))
		}
		return out
	}
	if !slices.Equal(opts("wave"), voice.Waves) || !slices.Equal(opts("filter"), voice.Filters) || !slices.Equal(opts("percussion"), voice.PercussionNames()) {
		t.Errorf("options differ from the library: %v", f.Options)
	}
}

func checkField(t *testing.T, f fieldsFile, fd field, path, prefix string) {
	load := func(v any) []Problem {
		_, p := LoadVoice(with(path, v))
		return p
	}
	errPath := path
	switch fd.ErrorPath {
	case "":
	case "*":
		errPath = "*"
	default:
		errPath = prefix + fd.ErrorPath
	}
	switch fd.Kind {
	case "number", "int":
		if fd.Min == nil {
			t.Fatal("no min")
		}
		delta := 1.0
		if fd.Kind == "number" && fd.Max != nil {
			delta = (*fd.Max - *fd.Min) / 1000
		}
		if p := load(*fd.Min); p != nil {
			t.Errorf("min %g: %v", *fd.Min, p)
		}
		if p := load(*fd.Min - delta); !problemAt(p, errPath) {
			t.Errorf("below min (%g): no problem at %s: %v", *fd.Min-delta, errPath, p)
		}
		if fd.Max != nil {
			if p := load(*fd.Max); p != nil {
				t.Errorf("max %g: %v", *fd.Max, p)
			}
			above := *fd.Max + delta
			if fd.MaxBelow != nil {
				above = *fd.MaxBelow
			}
			if p := load(above); !problemAt(p, errPath) {
				t.Errorf("above max (%g): no problem at %s: %v", above, errPath, p)
			}
		}
		if d, ok := fd.Default.(float64); ok && !fd.ZeroIsDefault {
			if p := load(d); p != nil {
				t.Errorf("default %g: %v", d, p)
			}
		}
	case "select":
		values := f.Options[fd.Options]
		if len(values) == 0 {
			t.Fatalf("no options %q", fd.Options)
		}
		for _, v := range values {
			if p := load(v); p != nil {
				t.Errorf("option %v: %v", v, p)
			}
		}
		if p := load("bogus"); p == nil {
			t.Error("an unknown option loads")
		}
	case "list":
		if p := load([]float64{*fd.Min, 0, *fd.Max}); p != nil {
			t.Errorf("list: %v", p)
		}
		if p := load(make([]float64, fd.MaxItems+1)); !problemAt(p, path) {
			t.Errorf("%d items: no problem at %s: %v", fd.MaxItems+1, path, p)
		}
	case "text", "bool":
		v := any("x")
		if fd.Kind == "bool" {
			v = true
		}
		if p := load(v); p != nil {
			t.Errorf("%v: %v", v, p)
		}
	default:
		t.Errorf("unknown kind %q", fd.Kind)
	}
}

// TestFieldsHaveHelp checks that every field and every menu choice has a
// tooltip, and that no tooltip names a choice that does not exist.
func TestFieldsHaveHelp(t *testing.T) {
	f := readFields(t)
	for _, fields := range [][]field{f.Voice, f.Instrument, f.Percussion, f.Rhythm} {
		for _, fd := range fields {
			if strings.TrimSpace(fd.Help) == "" {
				t.Errorf("%s: no help", fd.Key)
			}
		}
	}
	for name, values := range f.Options {
		help := f.OptionHelp[name]
		for _, v := range values {
			if strings.TrimSpace(help[fmt.Sprint(v)]) == "" {
				t.Errorf("options.%s: %v has no help", name, v)
			}
		}
		for k := range help {
			if !slices.ContainsFunc(values, func(v any) bool { return fmt.Sprint(v) == k }) {
				t.Errorf("optionHelp.%s: %q is not an option", name, k)
			}
		}
	}
	for name := range f.OptionHelp {
		if _, ok := f.Options[name]; !ok {
			t.Errorf("optionHelp.%s: no such options", name)
		}
	}
}
