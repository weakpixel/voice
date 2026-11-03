package voice

import (
	"strings"
	"testing"
)

func TestLoadRhythm(t *testing.T) {
	v, err := LoadVoice(strings.NewReader(`{"version": 0, "instruments": {"vowel": {}},
		"percussion": {"boom": {"base": "kick", "note": 30}, "snare": {"gain": 3}, "blip": {"wave": "square"}},
		"rhythm": {"bpm": 100, "division": 2, "swing": 0.2, "rest": {"end": "step"}, "countIn": 1,
			"beat": {"boom": "x...x...", "hat": "x-x-x-x-", "snare": "..x...x."}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range v.Percussion {
		names = append(names, p.Name)
	}
	if got := strings.Join(names, ","); got != "blip,boom,hat,snare" {
		t.Errorf("percussion %s, want blip,boom,hat,snare (sorted, built-in hat added)", got)
	}
	kick, _ := BuiltinPercussion("kick")
	snare, _ := BuiltinPercussion("snare")
	if boom := v.Percussion[1]; boom.Note != 30 || boom.Spec.GlideFrom != kick.Spec.GlideFrom {
		t.Errorf("boom = %+v, want the kick with note 30", boom)
	}
	if s := v.Percussion[3]; s.Spec.Gain != 3 || s.Spec.Cutoff != snare.Spec.Cutoff || s.Note != snare.Note {
		t.Errorf("snare = %+v, want the built-in snare with gain 3", s)
	}
	if b := v.Percussion[0]; b.Note != defaultPercussionNote || b.Spec.Wave != "square" {
		t.Errorf("blip = %+v", b)
	}
	r := v.Rhythm
	if r.BPM != 100 || r.Division != 2 || r.Swing != 0.2 || r.PauseRest != RestBeat || r.EndRest != RestStep ||
		r.CountIn != 1 || r.BeatVolume != DefaultBeatVolume || len(r.Beat) != 3 || r.Beat[0].Sound != "boom" {
		t.Errorf("rhythm = %+v", r)
	}
}

func TestLoadRhythmErrors(t *testing.T) {
	_, err := LoadVoice(strings.NewReader(`{"version": 0, "instruments": {"vowel": {}},
		"percussion": {"a": {"base": "cowbell"}, "b": {"note": 200}, "c": {}, "d": {}, "e": {}, "f": {}, "g": {}, "h": {}, "i": {"bogus": 1}},
		"rhythm": {"bpm": 20, "swing": 0.7, "accents": [1, 0], "rest": {"pause": "long"}, "countIn": 3, "beatVolume": 2,
			"beat": {"nope": "x...............", "b": "x..", "c": "x..o............"}}}`))
	for _, w := range []string{
		`percussion.a.base: "cowbell" is not a built-in sound`,
		"percussion.b.note: 200 is outside",
		`percussion.i: json: unknown field "bogus"`,
		"rhythm.bpm: 20 is outside",
		"rhythm.swing: 0.7",
		"rhythm.accents: 2 values, want 16",
		`rhythm.rest.pause: "long" is not one of`,
		"rhythm.countIn: 3",
		"rhythm.beatVolume: 2",
		"rhythm.beat.nope: unknown sound",
		"rhythm.beat.b: 3 steps, want 16",
		`rhythm.beat.c: step 4 is 'o'`,
	} {
		if err == nil || !strings.Contains(err.Error(), w) {
			t.Errorf("error does not contain %q: %v", w, err)
		}
	}
	_, err = LoadVoice(strings.NewReader(`{"version": 0, "instruments": {"vowel": {}},
		"percussion": {"a": {}, "b": {}, "c": {}, "d": {}, "e": {}, "f": {}, "g": {}, "h": {}, "i": {}},
		"rhythm": {"division": 5}}`))
	for _, w := range []string{"percussion: 9 sounds, at most 8", "rhythm.division: 5 is not 2, 3 or 4"} {
		if err == nil || !strings.Contains(err.Error(), w) {
			t.Errorf("error does not contain %q: %v", w, err)
		}
	}
}

func TestBuiltinPercussionLoads(t *testing.T) {
	for _, name := range PercussionNames() {
		p, ok := BuiltinPercussion(name)
		if !ok || p.Name != name {
			t.Fatalf("%s: %+v %v", name, p, ok)
		}
		v := &Voice{BaseNote: 60, Length: 0.5, Speed: 10, Percussion: []Percussion{p}}
		if err := v.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, ok := BuiltinPercussion("cowbell"); ok {
		t.Error("cowbell is not built in")
	}
}
