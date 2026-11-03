package voice

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestDefaultLanguageAndPresetsLoad(t *testing.T) {
	l := DefaultLanguage()
	if l.Name() != "default" || len(l.Phonemes()) < 20 || len(l.Punctuation()) != len(defaultPunctuation) {
		t.Errorf("default language: name %q, %d phonemes, %d punctuation", l.Name(), len(l.Phonemes()), len(l.Punctuation()))
	}
	for _, name := range PresetNames() {
		v, err := Preset(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if v.Name != name {
			t.Errorf("preset %s has name %q", name, v.Name)
		}
		if err := v.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Preset("loud"); err == nil || !strings.Contains(err.Error(), "soft, chirpy, gruff, divine") {
		t.Errorf("unknown preset: %v", err)
	}
	a, _ := Preset("soft")
	a.Pitch = 5
	if b, _ := Preset("soft"); b.Pitch != 0 {
		t.Error("Preset returned a shared voice")
	}
}

func TestLoadLanguageErrors(t *testing.T) {
	tests := []struct {
		name string
		json string
		want []string
	}{
		{"malformed", `{"version": 0,`, []string{"voice: language: unexpected EOF"}},
		{"unknown field", `{"version": 0, "phonemes": [], "colour": 1}`, []string{`unknown field "colour"`}},
		{"everything", `{
			"phonemes": [
				{"symbol": "", "category": "vowel"},
				{"symbol": "A", "category": "vowel", "length": 0, "velocity": 2, "pitch": 30},
				{"symbol": "a", "category": "hum"},
				{"symbol": "s h", "category": "fricative"}
			],
			"punctuation": [{"symbol": "?", "pause": -1, "end": "up"}],
			"whitespace": {"pause": 100},
			"unknown": {"policy": "fallback", "phoneme": "zz"}
		}`, []string{
			"version: missing",
			"phonemes[0].symbol: empty symbol",
			"phonemes[1].length: 0 is outside",
			"phonemes[1].velocity: 2 is outside",
			"phonemes[1].pitch: 30 is outside",
			`phonemes[2].symbol: symbol "a" is already defined at phonemes[1].symbol`,
			`phonemes[2].category: "hum" is not one of`,
			`phonemes[3].symbol: symbol "s h" contains whitespace`,
			"punctuation[0].pause: -1 is outside",
			`punctuation[0].end: "up" is not one of`,
			"whitespace.pause: 100 is outside",
			`unknown.phoneme: "zz" is not a phoneme`,
		}},
		{"version", `{"version": 1, "phonemes": [{"symbol": "a", "category": "vowel"}]}`, []string{"version: 1 is not supported"}},
		{"policy", `{"version": 0, "phonemes": [{"symbol": "a", "category": "vowel"}], "unknown": {"policy": "beep", "phoneme": "a"}}`, []string{
			`unknown.policy: "beep" is not one of`,
			"unknown.phoneme: only used with the fallback policy",
		}},
		{"default punctuation collides", `{"version": 0, "phonemes": [{"symbol": "!", "category": "plosive"}]}`, []string{
			`default punctuation "!" collides`,
		}},
		{"no phonemes", `{"version": 0}`, []string{"phonemes: no phonemes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadLanguage(strings.NewReader(tt.json))
			if err == nil {
				t.Fatal("no error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error does not contain %q:\n%v", w, err)
				}
			}
		})
	}
}

func TestLoadFS(t *testing.T) {
	fsys := fstest.MapFS{
		"lang/tiny.lang.json": {Data: []byte(`{"version": 0, "name": "tiny", "phonemes": [{"symbol": "a", "category": "vowel"}], "punctuation": []}`)},
		"bad.voice.json":      {Data: []byte(`{"version": 0, "instruments": {}}`)},
	}
	l, err := LoadLanguageFS(fsys, "lang/tiny.lang.json")
	if err != nil || l.Name() != "tiny" || len(l.Punctuation()) != 0 {
		t.Fatalf("LoadLanguageFS: %v %v", l, err)
	}
	if _, err := LoadVoiceFS(fsys, "bad.voice.json"); err == nil || !strings.Contains(err.Error(), "bad.voice.json: voice: voice: instruments.vowel: missing") {
		t.Errorf("LoadVoiceFS error: %v", err)
	}
	if _, err := LoadLanguageFS(fsys, "missing.lang.json"); err == nil {
		t.Error("missing file accepted")
	}
}

func TestLoadVoice(t *testing.T) {
	v, err := LoadVoice(strings.NewReader(`{"version": 0, "name": "x", "instruments": {
		"vowel": {"wave": "square"}, "motif": {"sample": "hum.wav", "root": 72, "loop": true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if v.BaseNote != DefaultBaseNote || v.Speed != DefaultSpeed || v.MotifVolume != DefaultMotifVolume {
		t.Errorf("defaults not applied: %+v", v)
	}
	if v.Instruments[InstPlosive].Wave != "square" || v.Instruments[InstMotif].Sample != "hum.wav" {
		t.Errorf("instruments: %+v", v.Instruments)
	}
}

func TestLoadVoiceErrors(t *testing.T) {
	_, err := LoadVoice(strings.NewReader(`{"version": 2, "baseNote": 10, "variation": -1, "velocity": 2,
		"length": 0, "speed": 100, "pan": 3, "motifVolume": 5, "instruments": {
		"vowel": {"wave": "buzz", "sample": "a.wav", "pulseWidth": 2, "attackMs": -1, "sustain": 2, "resonance": 1, "gain": -1},
		"plosive": {"root": 70, "loopStart": 5, "loopEnd": 2},
		"trumpet": {}}}`))
	if err == nil {
		t.Fatal("no error")
	}
	for _, w := range []string{
		"version: 2 is not supported", "baseNote: 10", "variation: -1", "velocity: 2", "length: 0", "speed: 100",
		"pan: 3", "motifVolume: 5", `instruments.vowel.wave: "buzz"`, "instruments.vowel.wave: an instrument is a wave or a sample",
		"instruments.vowel.pulseWidth", "instruments.vowel.attackMs", "instruments.vowel.sustain", "instruments.vowel.resonance",
		"instruments.vowel.gain", "instruments.plosive.loopStart", "instruments.plosive.sample: root and loop need a sample",
		"instruments.trumpet: unknown instrument",
	} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error does not contain %q", w)
		}
	}
	// Errors come in a stable order.
	_, err2 := LoadVoice(strings.NewReader(`{"version": 2, "baseNote": 10, "variation": -1, "velocity": 2,
		"length": 0, "speed": 100, "pan": 3, "motifVolume": 5, "instruments": {
		"vowel": {"wave": "buzz", "sample": "a.wav", "pulseWidth": 2, "attackMs": -1, "sustain": 2, "resonance": 1, "gain": -1},
		"plosive": {"root": 70, "loopStart": 5, "loopEnd": 2},
		"trumpet": {}}}`))
	if err.Error() != err2.Error() {
		t.Error("error order differs between loads")
	}
}

func TestLoadVoiceEffectFields(t *testing.T) {
	v, err := LoadVoice(strings.NewReader(`{"version": 0, "instruments": {
		"vowel": {"wave": "saw", "reverb": 0.5, "layers": [0, -12, 7], "unison": 5, "spread": 0.3}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if in := v.Instruments[InstVowel]; in.Reverb != 0.5 || len(in.Layers) != 3 || in.Unison != 5 || in.Spread != 0.3 {
		t.Errorf("fields not loaded: %+v", in)
	}
	_, err = LoadVoice(strings.NewReader(`{"version": 0, "instruments": {
		"vowel": {"reverb": 2, "layers": [0, 1, 2, 3, 30], "unison": 8, "spread": -1},
		"motif": {"sample": "a.wav", "unison": 3}}}`))
	for _, w := range []string{"instruments.vowel.reverb: 2", "instruments.vowel.layers: 5 layers", "instruments.vowel.layers[4]: 30",
		"instruments.vowel.unison: 8", "instruments.vowel.spread: -1", "instruments.motif.unison: needs a wave"} {
		if err == nil || !strings.Contains(err.Error(), w) {
			t.Errorf("error does not contain %q: %v", w, err)
		}
	}
}
