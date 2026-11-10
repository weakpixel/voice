package voice

import "fmt"

// VoiceSpan is a part of a line that another voice speaks: the tokens that
// start in the bytes Start to End of the text. A phoneme is never split, so
// a span snaps to the tokens it starts.
//
// The span's voice decides the sound of its phonemes: speed (the length of
// their slots), Length, base note and pitch, variation, velocity, and the
// instruments (through Owner). The line's voice keeps the line: its pan
// (for every note, so the sound stays with the speaker), the clauses and
// their contours, the motif, the ending and breath, a rhythm's grid and
// percussion, and the random seed. A switch adds no time: the span's first
// slot starts where the previous one ends.
type VoiceSpan struct {
	Start, End int
	Voice      *Voice
	// Owner is the owner a [Speaker] plays the span's notes on; Compose
	// ignores it. Use an owner whose instruments are the span voice's
	// (Stream.SetVoice), not the speaker's own.
	Owner Owner
	// Impression makes the line's voice imitate Voice: the span keeps
	// Voice's sound and timing, but its register (base note plus pitch)
	// lies halfway between the line's voice and Voice.
	Impression bool
}

// impressionShare is how far an impression moves the register from the
// line's voice towards the imitated one.
const impressionShare = 0.5

// checkSpans panics unless spans are sorted, apart, inside a text of n
// bytes and have a voice.
func checkSpans(spans []VoiceSpan, n int) {
	prev := 0
	for i, s := range spans {
		switch {
		case s.Voice == nil:
			panic(fmt.Sprintf("voice: ComposeWith: span %d has no voice", i))
		case s.Start < prev || s.End < s.Start || s.End > n:
			panic(fmt.Sprintf("voice: ComposeWith: span %d (%d to %d) is not sorted, overlaps or is outside the text of %d bytes", i, s.Start, s.End, n))
		}
		prev = s.End
	}
}

// tokenSpans returns, for each token, 0 or the number (from 1) of the span
// its start lies in; nil without spans.
func tokenSpans(tokens []Phoneme, spans []VoiceSpan) []int {
	if len(spans) == 0 {
		return nil
	}
	out := make([]int, len(tokens))
	k := 0
	for i, t := range tokens {
		for k < len(spans) && spans[k].End <= t.Start {
			k++
		}
		if k < len(spans) && spans[k].Start <= t.Start {
			out[i] = k + 1
		}
	}
	return out
}

// spanAt returns the span number of token i (0: the line's voice).
func (c *composer) spanAt(i int) int {
	if c.tokSpan == nil {
		return 0
	}
	return c.tokSpan[i]
}

// voiceAt returns the voice that speaks token i.
func (c *composer) voiceAt(i int) *Voice {
	if k := c.spanAt(i); k > 0 {
		return c.spans[k-1].Voice
	}
	return c.v
}

// beatAt returns the beat in seconds of the voice of token i.
func (c *composer) beatAt(i int) float64 {
	if c.spanAt(i) == 0 {
		return c.beat
	}
	return 1 / float64(clampF(c.voiceAt(i).Speed, 1, 60))
}

// gateAt returns the note share of a slot (Length) of token i's voice.
func (c *composer) gateAt(i int) float64 {
	return float64(clampF(c.voiceAt(i).Length, 0.05, 1))
}

// baseAt returns the register (base note plus pitch) of token i: its
// voice's, or for an impression halfway from the line's voice to it.
func (c *composer) baseAt(i int) float32 {
	host := register(c.v)
	k := c.spanAt(i)
	if k == 0 {
		return host
	}
	s := &c.spans[k-1]
	if s.Impression {
		return host + impressionShare*(register(s.Voice)-host)
	}
	return register(s.Voice)
}

func register(v *Voice) float32 {
	return clampF(v.BaseNote, 24, 108) + clampF(v.Pitch, -24, 24)
}
