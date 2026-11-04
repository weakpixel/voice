package voice

import "math"

// Accents raise a syllable by up to these amounts on a step of strength 1.
const (
	rhythmAccentVelocity = 0.4 // share of the velocity added
	rhythmAccentPitch    = 2   // semitones added
)

// syllable is a vowel with the consonants around it, the unit a rhythmic
// voice places on the grid.
type syllable struct {
	first, nucleus, last int     // token indexes of its first, nucleus and last phoneme
	onset, body          float64 // natural seconds before the nucleus and from it on
	rest                 Rest    // what follows it: punctuation's rest, or none
	pause                float32 // the punctuation's pause in beats, for RestStep
}

// grid maps step k to a time. Step 0 is the first syllable's nucleus, on a
// bar line; negative steps are the count-in.
type grid struct {
	origin, step, swing float64 // seconds, seconds, share of a step
	barSteps, division  int
}

// maxOnset is the most time onset consonants get before their step: half
// of the shortest step, so a nucleus always keeps half a step.
func (g *grid) maxOnset() float64 { return g.step * (1 - g.swing) / 2 }

func (g *grid) at(k int) float64 {
	t := g.origin + float64(k)*g.step
	if k&1 == 1 { // odd steps swing; k&1 is also right for negative k
		t += g.swing * g.step
	}
	return t
}

// nextBeat returns the time of the first beat (a step that is a multiple
// of the division) at or after t.
func (g *grid) nextBeat(t float64) float64 {
	k := nextMultiple(max(0, int(math.Ceil((t-g.origin)/g.step))), g.division)
	for g.at(k) < t-1e-9 {
		k += g.division
	}
	return g.at(k)
}

// nextMultiple returns the smallest multiple of m that is at least k.
func nextMultiple(k, m int) int { return (k + m - 1) / m * m }

// rhythmic composes the line on the voice's beat grid: every syllable's
// nucleus starts on a step, its onset consonants lead into it, its nucleus
// and coda fill its steps up to the next syllable's onset. Whitespace takes
// no time; punctuation rests as the rhythm says. Accents follow the step
// of the nucleus. The percussion loops over every bar from the count-in to
// the end of the bar in which the voice ends.
func (c *composer) rhythmic(tokens []Phoneme, opts ComposeOptions) {
	r := c.v.Rhythm
	syl := c.syllables(tokens, r)
	g := c.grid(r, opts)
	if len(syl) == 0 {
		return
	}
	barLen := g.step * float64(g.barSteps)
	pickup := min(syl[0].onset, g.maxOnset())
	if opts.Clock != nil {
		// The music's bar lines are at Phase + n bars; start on the first
		// one after the count-in that leaves room for the pickup.
		phase := math.Mod(opts.Clock.Phase.Seconds(), barLen)
		if phase < 0 {
			phase += barLen
		}
		g.origin = phase + float64(r.CountIn)*barLen
		if g.origin < pickup {
			g.origin += barLen
		}
	} else {
		g.origin = max(float64(r.CountIn)*barLen, pickup)
	}

	accents := c.accents(r, g.barSteps)
	gate := float64(clampF(c.v.Length, 0.05, 1))
	end := c.lineEnding(tokens)
	c.snap = func(t float64) float64 { return g.nextBeat(t) }
	starts := make([]float64, len(tokens)) // when each phoneme token starts
	ends := make([]float64, len(tokens))   // and when its slot ends
	k, voiceEnd := 0, 0
	ci := 0
	for si := range syl {
		s := &syl[si]
		steps := max(1, int(math.Round((s.onset+s.body)/g.step)))
		steps = max(steps, int(math.Round(float64(steps)*end.stretch[s.nucleus]))) // the line's end slows down
		nucleus := g.at(k)
		bodyEnd := g.at(k + steps)
		if si+1 < len(syl) && s.rest == RestNone {
			bodyEnd -= min(syl[si+1].onset, g.maxOnset()) // room for the next onset
		}
		onsetScale := 1.0
		if s.onset > g.maxOnset() {
			onsetScale = g.maxOnset() / s.onset
		}
		bodyScale := (bodyEnd - nucleus) / s.body
		a := float32(accents[k%g.barSteps])
		t := nucleus - s.onset*onsetScale
		for i := s.first; i <= s.last; i++ {
			tok := tokens[i]
			if tok.Kind != TokenPhoneme {
				continue
			}
			for ci < len(c.clauses) && c.clauses[ci].last < i {
				ci++
			}
			cl := &c.clauses[ci]
			slot := c.beat * float64(tok.Length)
			if i < s.nucleus {
				slot *= onsetScale
			} else {
				slot *= bodyScale
			}
			if i == cl.first {
				cl.start = t
			}
			pitch, vel := c.tone(tokens, i, cl)
			pitch += rhythmAccentPitch * a
			vel *= 1 + rhythmAccentVelocity*a
			c.note(tok, t, slot*min(1, gate+end.gate[i]), pitch, vel*end.fade[i])
			starts[i], ends[i] = t, t+slot
			t += slot
			cl.end = t
		}
		c.tl.Duration = seconds(bodyEnd)
		c.voiced, c.voiceEnd = true, bodyEnd
		k += steps
		voiceEnd = k
		k = c.rest(k, s, &g)
	}
	c.revealRhythmic(tokens, starts, ends)
	if !opts.MuteBeat {
		c.percussion(r, &g, nextMultiple(voiceEnd, g.barSteps))
	}
}

// syllables splits the phonemes of tokens into syllables (see [Rhythm]):
// within a word every vowel is a nucleus, the last consonant before a vowel
// starts its syllable and the others end the previous one. A word without
// a vowel is one syllable around its first sonorant (or first phoneme).
// Punctuation after a syllable sets its rest.
func (c *composer) syllables(tokens []Phoneme, r *Rhythm) []syllable {
	var out []syllable
	var word []int // phoneme token indexes of the current word
	flush := func() {
		if len(word) > 0 {
			out = c.splitWord(out, tokens, word)
			word = word[:0]
		}
	}
	for i, tok := range tokens {
		switch tok.Kind {
		case TokenPhoneme:
			word = append(word, i)
		case TokenPause:
			flush()
			if tok.Clause && len(out) > 0 {
				last := &out[len(out)-1]
				rest := r.PauseRest
				if tok.Contour != ContourNone {
					rest = r.EndRest
				}
				last.rest = max(last.rest, min(rest, RestBar))
				last.pause += tok.Length
			}
		}
	}
	flush()
	return out
}

// splitWord appends the syllables of one word (phoneme token indexes).
func (c *composer) splitWord(out []syllable, tokens []Phoneme, word []int) []syllable {
	var vowels []int // positions in word
	for j, i := range word {
		if tokens[i].Category == Vowel {
			vowels = append(vowels, j)
		}
	}
	if len(vowels) == 0 {
		nucleus := 0
		for j, i := range word {
			if tokens[i].Category == Sonorant {
				nucleus = j
				break
			}
		}
		vowels = []int{nucleus}
	}
	start := 0
	for n, v := range vowels {
		end := len(word) // exclusive
		if n+1 < len(vowels) {
			next := vowels[n+1]
			end = next
			if next-1 > v { // a consonant between: the last one starts the next syllable
				end = next - 1
			}
		}
		s := syllable{first: word[start], nucleus: word[v], last: word[end-1]}
		for j := start; j < end; j++ {
			d := c.beat * float64(tokens[word[j]].Length)
			if j < v {
				s.onset += d
			} else {
				s.body += d
			}
		}
		s.body = max(s.body, 1e-3) // a zero-length nucleus still gets its steps
		out = append(out, s)
		start = end
	}
	return out
}

// grid returns the step length, swing and bar of r, with the clock's tempo
// when there is one. The origin is set by the caller.
func (c *composer) grid(r *Rhythm, opts ComposeOptions) grid {
	bpm := r.BPM
	if opts.Clock != nil {
		bpm = opts.Clock.BPM
	}
	div := r.Division
	if div < 2 || div > 4 {
		div = DefaultDivision
	}
	return grid{
		step:     60 / float64(clampF(bpm, 40, 240)) / float64(div),
		swing:    float64(clampF(r.Swing, 0, 0.5)),
		barSteps: 4 * div,
		division: div,
	}
}

// accents returns the accent of every step of a bar: r.Accents when it
// fits, otherwise beats 1 and 3 strong, 2 and 4 medium.
func (c *composer) accents(r *Rhythm, barSteps int) []float64 {
	out := make([]float64, barSteps)
	if len(r.Accents) == barSteps {
		for i, a := range r.Accents {
			out[i] = float64(clampF(a, 0, 1))
		}
		return out
	}
	beat := barSteps / 4
	out[0], out[2*beat] = 1, 1
	out[beat], out[3*beat] = 0.6, 0.6
	return out
}

// rest returns the step after syllable s ends at step k, following its
// rest. Every rest but RestNone leaves at least one silent step.
func (c *composer) rest(k int, s *syllable, g *grid) int {
	switch s.rest {
	case RestStep:
		return k + max(1, int(math.Round(c.beat*float64(s.pause)/g.step)))
	case RestBeat:
		return nextMultiple(k+1, g.division)
	case RestBar:
		return nextMultiple(k+1, g.barSteps)
	}
	return k
}

// revealRhythmic reveals each phoneme token when it starts and every other
// token when the phoneme before it ends, so the text follows the voice and
// nothing shows during the count-in.
func (c *composer) revealRhythmic(tokens []Phoneme, starts, ends []float64) {
	t := 0.0
	for i, tok := range tokens {
		if tok.Kind == TokenPhoneme {
			c.reveal(starts[i], tok.End)
			t = ends[i]
			continue
		}
		c.reveal(t, tok.End)
	}
}

// percussion adds the beat from the first count-in step to endStep
// (exclusive) and extends the line to endStep. Tracks naming an unknown
// sound or with a pattern of the wrong length are left out.
func (c *composer) percussion(r *Rhythm, g *grid, endStep int) {
	vel := clampF(r.BeatVolume, 0, 1)
	played := false
	for _, tr := range r.Beat {
		idx := c.v.percussionIndex(tr.Sound)
		if idx < 0 || idx >= MaxPercussion || len(tr.Pattern) != g.barSteps {
			continue
		}
		note := c.v.Percussion[idx].Note
		for k := -max(r.CountIn, 0) * g.barSteps; k < endStep; k++ {
			v := vel
			switch tr.Pattern[(k%g.barSteps+g.barSteps)%g.barSteps] {
			case 'x':
			case '-':
				v /= 2
			default:
				continue
			}
			on := g.at(k)
			c.tl.Notes = append(c.tl.Notes, Note{
				On: seconds(on), Off: seconds(on + g.step/2), Instrument: InstPercussion + Instrument(idx),
				Note: note, Velocity: v, Pan: clampF(c.v.Pan, -1, 1),
			})
			played = true
		}
	}
	if played {
		c.tl.Duration = max(c.tl.Duration, seconds(g.at(endStep)))
	}
}
