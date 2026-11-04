package voice

import (
	"strings"
	"testing"
	"time"
)

// vowelLanguage has one vowel "a" and the default punctuation.
func vowelLanguage(t *testing.T) *Language {
	t.Helper()
	l, err := LoadLanguage(stringsReader(`{"version": 0, "phonemes": [{"symbol": "a", "category": "vowel"}, {"symbol": "t", "category": "plosive"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func endingVoice(t *testing.T) *Voice {
	v := preset(t, "soft")
	v.Variation = 0
	v.Length = 1 // a note fills its slot, so Off - On is the slot
	return v
}

func TestEndingLengthens(t *testing.T) {
	v := endingVoice(t)
	l := vowelLanguage(t)
	tl := ComposeText(v, l, "tata tata")
	vowels := notesOf(tl, InstVowel)
	mid, last := vowels[0].Off-vowels[0].On, vowels[len(vowels)-1].Off-vowels[len(vowels)-1].On
	if r := float64(last) / float64(mid); r < 1.49 || r > 1.51 {
		t.Errorf("last syllable %v, a middle one %v: ratio %.3f, want 1.5", last, mid, r)
	}
	// A sentence end inside the line gets 1.25.
	tl = ComposeText(v, l, "tata. tata")
	vowels = notesOf(tl, InstVowel)
	if r := float64(vowels[1].Off-vowels[1].On) / float64(mid); r < 1.24 || r > 1.26 {
		t.Errorf("mid-line sentence end: ratio %.3f, want 1.25", r)
	}
	// A comma does not lengthen.
	tl = ComposeText(v, l, "tata, tata")
	vowels = notesOf(tl, InstVowel)
	if d := vowels[1].Off - vowels[1].On; d != mid {
		t.Errorf("before a comma: %v, want %v", d, mid)
	}
	// One phoneme is the whole last syllable.
	tl = ComposeText(v, l, "a")
	if d := tl.Notes[0].Off - tl.Notes[0].On; float64(d) < 1.49*float64(mid) {
		t.Errorf("single phoneme: %v", d)
	}
}

func TestEndingFades(t *testing.T) {
	v := endingVoice(t)
	tl := ComposeText(v, vowelLanguage(t), "aaaaaa")
	vowels := notesOf(tl, InstVowel)
	first := vowels[0].Velocity
	for i, want := range finalFade {
		n := vowels[len(vowels)-3+i]
		if d := n.Velocity - first*want; d < -1e-6 || d > 1e-6 {
			t.Errorf("%d from the end: velocity %g, want %g", 3-i, n.Velocity, first*want)
		}
	}
	if vowels[2].Velocity != first {
		t.Errorf("an earlier note fades: %g", vowels[2].Velocity)
	}
}

func TestEndingBreath(t *testing.T) {
	v := endingVoice(t)
	l := vowelLanguage(t)
	for _, breath := range []float32{0, 0.4, 1} {
		v.Breath = breath
		tl := ComposeText(v, l, "tata tata. tata")
		vowels := notesOf(tl, InstVowel)
		voiceEnd := vowels[len(vowels)-1].Off // Length 1: the end of the last slot
		motif := notesOf(tl, InstMotif)
		want := voiceEnd + time.Duration(float64(breath)*float64(time.Second))
		if d := motif[len(motif)-1].Off - want; d < -time.Microsecond || d > time.Microsecond {
			t.Errorf("breath %g: motif ends at %v, want %v", breath, motif[len(motif)-1].Off, want)
		}
		if d := tl.Duration - want; d < -time.Microsecond || d > time.Microsecond {
			t.Errorf("breath %g: duration %v, want %v", breath, tl.Duration, want)
		}
		if last := tl.Reveals[len(tl.Reveals)-1]; last.At > voiceEnd {
			t.Errorf("breath %g: text finishes at %v, after the voice (%v)", breath, last.At, voiceEnd)
		}
	}
	// No voice, no breath.
	if tl := ComposeText(v, l, "..."); tl.Duration != 0 {
		t.Errorf("silent line: duration %v", tl.Duration)
	}
}

func TestEndingRhythmic(t *testing.T) {
	v := rhythmVoice(t)
	v.MotifVolume = 0.2
	v.Length = 1 // the last vowel's note ends at the end of its steps
	text := "ta ta ta ta"
	tokens := Parse(DefaultLanguage(), text)
	plain := ComposeWith(v, text, tokens, ComposeOptions{MuteBeat: true})
	vowels := notesOf(plain, InstVowel)
	origin := vowels[0].On
	// The last syllable takes 1.5 times the steps of the others.
	mid, _ := steps(vowels[1].On - vowels[0].On)
	last0, _ := steps(vowels[len(vowels)-1].Off - vowels[len(vowels)-1].On)
	if want := int(float64(mid)*1.5 + 0.5); last0 != want {
		t.Errorf("last syllable %d steps, others %d, want %d", last0, mid, want)
	}
	motif := notesOf(plain, InstMotif)
	end := motif[len(motif)-1].Off
	k, onGrid := steps(end - origin)
	if !onGrid || k%4 != 0 {
		t.Errorf("motif ends at step %d (on grid %v), want a beat", k, onGrid)
	}
	last := vowels[len(vowels)-1]
	if end < last.Off+400*time.Millisecond || end > last.Off+400*time.Millisecond+4*rhythmStep {
		t.Errorf("motif ends at %v, want the first beat at least 400 ms after the voice (%v)", end, last.Off)
	}
	if plain.Duration != end {
		t.Errorf("duration %v, want the breath's end %v", plain.Duration, end)
	}
}

func TestLoadBreath(t *testing.T) {
	v, err := LoadVoice(stringsReader(`{"version": 0, "instruments": {"vowel": {}}}`))
	if err != nil || v.Breath != DefaultBreath {
		t.Fatalf("default breath %v, %v", v, err)
	}
	_, err = LoadVoice(stringsReader(`{"version": 0, "breath": 3, "instruments": {"vowel": {}}}`))
	if err == nil || !strings.Contains(err.Error(), "breath: 3 is outside 0 to 2") {
		t.Errorf("error %v", err)
	}
}
