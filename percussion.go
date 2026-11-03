package voice

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
)

// Percussion is one percussion sound of a voice: an instrument played at a
// fixed note by the voice's [Rhythm] beat. Sound i of [Voice.Percussion]
// plays on instrument InstPercussion + i.
type Percussion struct {
	Name string
	Note float32 // the note every hit plays, 0 to 127
	Spec InstrumentSpec
}

// percussionFile is the JSON form of one entry of a voice file's
// "percussion" object: the instrument fields, a built-in sound to start
// from, and the note. An entry named like a built-in sound starts from it
// unless it names another base.
type percussionFile struct {
	InstrumentSpec
	Base string   `json:"base"`
	Note *float32 `json:"note"`
}

// defaultPercussionNote is the note of a percussion sound that names
// neither a note nor a base.
const defaultPercussionNote = 60

// builtinPercussion holds the built-in sounds as voice file JSON, so a
// "base" is applied by decoding the built-in and then the entry on top of
// it: fields the entry sets win, the others keep the built-in's value.
// Sustain 0 ends each hit after its decay, whatever the note length.
var builtinPercussion = map[string]string{
	// A sine that drops two octaves in 45 ms: the click and boom of a kick.
	"kick": `{"wave": "sine", "note": 36, "glideFrom": 24, "glideMs": 45, "decayMs": 260, "gain": 1}`,
	// Band-passed noise around 1.8 kHz with a short tail.
	"snare": `{"wave": "noise", "note": 38, "filter": "bandpass", "cutoff": 1800, "resonance": 0.2, "decayMs": 150, "gain": 2.4}`,
	// High-passed noise, very short.
	"hat": `{"wave": "noise", "note": 42, "filter": "highpass", "cutoff": 7000, "decayMs": 45, "gain": 0.7}`,
	// Lower, wider noise band than the snare.
	"clap": `{"wave": "noise", "note": 39, "filter": "bandpass", "cutoff": 1100, "resonance": 0.35, "decayMs": 110, "gain": 2.6}`,
	// A sine that drops a fifth.
	"tom": `{"wave": "sine", "note": 45, "glideFrom": 7, "glideMs": 60, "decayMs": 300, "gain": 1.1}`,
	// A short, high triangle tick.
	"rim": `{"wave": "triangle", "note": 76, "filter": "highpass", "cutoff": 400, "decayMs": 35, "gain": 0.8}`,
	// High-passed noise with a soft attack.
	"shaker": `{"wave": "noise", "note": 70, "filter": "highpass", "cutoff": 5000, "attackMs": 12, "decayMs": 70, "gain": 0.6}`,
}

// PercussionNames returns the names of the built-in percussion sounds,
// sorted.
func PercussionNames() []string {
	names := make([]string, 0, len(builtinPercussion))
	for name := range builtinPercussion {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// BuiltinPercussion returns the built-in percussion sound name, or false
// for an unknown name.
func BuiltinPercussion(name string) (Percussion, bool) {
	raw, ok := builtinPercussion[name]
	if !ok {
		return Percussion{}, false
	}
	var f percussionFile
	if err := decodeStrict(strings.NewReader(raw), &f); err != nil {
		panic("voice: built-in percussion " + name + ": " + err.Error()) // caught by tests
	}
	return Percussion{Name: name, Note: *f.Note, Spec: f.InstrumentSpec}, true
}

// loadPercussion builds the sound name from its voice file entry.
func loadPercussion(errs *fieldErrors, name string, raw json.RawMessage) (Percussion, bool) {
	path := "percussion." + name
	var f percussionFile
	if err := decodeStrict(bytes.NewReader(raw), &f); err != nil {
		errs.add(path, "%v", err)
		return Percussion{}, false
	}
	if _, builtin := builtinPercussion[name]; builtin && f.Base == "" {
		f.Base = name // "kick": {"gain": 2} is a louder built-in kick
	}
	if f.Base != "" {
		base, ok := builtinPercussion[f.Base]
		if !ok {
			errs.add(path+".base", "%q is not a built-in sound (one of %s)", f.Base, strings.Join(PercussionNames(), ", "))
			return Percussion{}, false
		}
		f = percussionFile{}
		// Both decodes succeed: the built-in is checked by tests and the
		// entry was decoded above.
		_ = decodeStrict(strings.NewReader(base), &f)
		_ = decodeStrict(bytes.NewReader(raw), &f)
	}
	p := Percussion{Name: name, Note: orDefault(f.Note, defaultPercussionNote), Spec: f.InstrumentSpec}
	return p, true
}

// validatePercussion checks the voice's percussion list.
func (v *Voice) validatePercussion(errs *fieldErrors) {
	if len(v.Percussion) > MaxPercussion {
		errs.add("percussion", "%d sounds, at most %d", len(v.Percussion), MaxPercussion)
	}
	for i, p := range v.Percussion {
		path := "percussion." + p.Name
		if p.Name == "" {
			errs.add("percussion", "sound %d has no name", i)
		} else if slices.IndexFunc(v.Percussion[:i], func(q Percussion) bool { return q.Name == p.Name }) >= 0 {
			errs.add(path, "defined twice")
		}
		errs.inRange(path+".note", p.Note, 0, 127)
		p.Spec.validate(errs, path)
	}
}

// percussionIndex returns the index of the sound name in v.Percussion, or
// -1.
func (v *Voice) percussionIndex(name string) int {
	return slices.IndexFunc(v.Percussion, func(p Percussion) bool { return p.Name == name })
}
