package voice

// How a line ends. Speech slows down and softens at the end of a phrase and
// a melody lets its last note ring, so the last syllable of a line is
// lengthened, the last phonemes fade, and the motif's last note rings on
// for the voice's Breath after the voice. Sentence ends inside a line get a
// smaller lengthening.
const (
	finalStretch    = 1.5  // slot factor of the line's last syllable
	sentenceStretch = 1.25 // slot factor of a mid-line sentence end's last syllable
	finalGateExtra  = 0.15 // added to the voice's Length for the last syllable's notes
)

// finalFade is the velocity factor of the line's last three phonemes, from
// the third-last to the last.
var finalFade = [3]float32{0.9, 0.8, 0.7}

// ending is the per-token shaping of a line's end: a slot factor, a gate
// addition and a velocity factor for every token (1, 0, 1 for tokens that
// are not shaped).
type ending struct {
	stretch []float64
	gate    []float64
	fade    []float32
}

// lineEnding finds the syllables to lengthen and the phonemes to fade. A
// syllable here is the last vowel of a clause and the consonants after it
// (the last phoneme when the clause has no vowel); the rhythmic path
// stretches its own syllables with stretchOf instead.
func (c *composer) lineEnding(tokens []Phoneme) ending {
	e := ending{
		stretch: make([]float64, len(tokens)),
		gate:    make([]float64, len(tokens)),
		fade:    make([]float32, len(tokens)),
	}
	for i := range tokens {
		e.stretch[i], e.fade[i] = 1, 1
	}
	for ci, cl := range c.clauses {
		last := ci == len(c.clauses)-1
		if !last && cl.contour == ContourNone {
			continue // a comma: no sentence end
		}
		from := lastVowel(tokens, cl.first, cl.last)
		for i := from; i <= cl.last; i++ {
			if tokens[i].Kind != TokenPhoneme {
				continue
			}
			if last {
				e.stretch[i], e.gate[i] = finalStretch, finalGateExtra
			} else {
				e.stretch[i] = sentenceStretch
			}
		}
	}
	n := 0
	for i := len(tokens) - 1; i >= 0 && n < len(finalFade); i-- {
		if tokens[i].Kind == TokenPhoneme {
			e.fade[i] = finalFade[len(finalFade)-1-n]
			n++
		}
	}
	return e
}

// lastVowel returns the index of the last vowel among the tokens first to
// last, or last when there is none.
func lastVowel(tokens []Phoneme, first, last int) int {
	for i := last; i >= first; i-- {
		if tokens[i].Kind == TokenPhoneme && tokens[i].Category == Vowel {
			return i
		}
	}
	return last
}

// breathEnd returns when the line's breath ends: voiceEnd plus the voice's
// Breath, snapped to the next beat when snap is set.
func (c *composer) breathEnd(voiceEnd float64, snap func(float64) float64) float64 {
	t := voiceEnd + float64(clampF(c.v.Breath, 0, 2))
	if snap != nil && c.v.Breath > 0 {
		t = snap(t)
	}
	return t
}
