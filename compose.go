package voice

import (
	"hash/fnv"
	"math/rand/v2"
	"slices"
	"time"
)

// Note is one note of a [Timeline].
type Note struct {
	On, Off    time.Duration // from the start of the line
	Instrument Instrument
	Note       float32 // semitones, MIDI numbering
	Velocity   float32 // 0 to 1
	Pan        float32 // -1 to 1
}

// Reveal says that from At on, the text up to byte Offset is shown.
type Reveal struct {
	At     time.Duration
	Offset int
}

// Timeline is a composed line: the notes to play and when to reveal how
// much text. Notes are sorted by On, Reveals by At (and Offset); the last
// reveal shows the whole text.
type Timeline struct {
	Notes   []Note
	Reveals []Reveal
	// Duration is when the line is over: the end of its last sound. A
	// pause after the last phoneme (the final period) does not count.
	Duration time.Duration
	TextLen  int
}

// ComposeText parses text with lang and composes it with v; see [Compose].
func ComposeText(v *Voice, lang *Language, text string) Timeline {
	return Compose(v, text, Parse(lang, text))
}

// ComposeOptions tune how [ComposeWith] times a line of a voice with a
// [Rhythm]. They do nothing for a voice without one.
type ComposeOptions struct {
	// Clock ties the line to the game's music: its tempo replaces the
	// voice's and the first syllable lands on the music's next bar line
	// (after the count-in). Nil keeps the voice's own tempo and starts at
	// once.
	Clock *Clock
	// MuteBeat leaves out the voice's percussion, for example when the
	// game's music already has a beat.
	MuteBeat bool
}

// Compose turns the parsed tokens of text into a timeline for voice v; it
// is ComposeWith without options.
func Compose(v *Voice, text string, tokens []Phoneme) Timeline {
	return ComposeWith(v, text, tokens, ComposeOptions{})
}

// ComposeWith turns the parsed tokens of text into a timeline for voice v.
// It is pure: the same voice, text, tokens and options always give the same
// timeline, because its randomness comes from a generator seeded with
// v.Seed xor a hash of the text.
//
// Each phoneme gets a slot of Length beats (a beat is 1/v.Speed seconds)
// and a note on its category's instrument for v.Length of the slot, at
// v.BaseNote + v.Pitch + the phoneme's pitch + a random offset within
// v.Variation. Pauses add silence. The text of a token is revealed when it
// starts. The end of each clause follows its punctuation's contour, and
// when v.MotifVolume is above 0 a motif of two to four notes, one per
// clause, plays under the line. A voice with a Rhythm places its syllables
// on a beat grid instead and adds its percussion (see the guide's rhythm
// section). Out-of-range voice fields are clamped; beat patterns that do
// not fit the rhythm are left out.
func ComposeWith(v *Voice, text string, tokens []Phoneme, opts ComposeOptions) Timeline {
	c := composer{
		v:    v,
		rng:  rand.New(rand.NewPCG(v.Seed^hashText(text), 0x9e3779b97f4a7c15)),
		beat: 1 / float64(clampF(v.Speed, 1, 60)),
	}
	c.clauses = findClauses(tokens)
	if v.Rhythm != nil {
		c.rhythmic(tokens, opts)
	} else {
		c.phonemes(tokens)
	}
	if c.voiced {
		c.motifEnd = c.breathEnd(c.voiceEnd, c.snap)
		c.tl.Duration = max(c.tl.Duration, seconds(c.motifEnd))
	}
	c.motif()
	slices.SortStableFunc(c.tl.Notes, func(a, b Note) int { return cmpDur(a.On, b.On) })
	c.tl.TextLen = len(text)
	if n := len(c.tl.Reveals); n == 0 || c.tl.Reveals[n-1].Offset != len(text) {
		c.tl.Reveals = append(c.tl.Reveals, Reveal{At: c.tl.Duration, Offset: len(text)})
	}
	return c.tl
}

func hashText(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func cmpDur(a, b time.Duration) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func clampF(x, lo, hi float32) float32 { return min(max(x, lo), hi) }

func seconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// clause is a run of tokens ending with punctuation (or the end of the
// line) that has at least one phoneme.
type clause struct {
	first, last int // token indexes of its first and last phoneme
	contour     Contour
	start, end  float64 // seconds: first note on, last slot end
}

// findClauses splits tokens at punctuation. Punctuation that follows
// punctuation ("?!", "!!") joins the previous clause; the last non-none
// contour wins.
func findClauses(tokens []Phoneme) []clause {
	var out []clause
	cur := clause{first: -1}
	for i, t := range tokens {
		switch {
		case t.Kind == TokenPhoneme:
			if cur.first < 0 {
				cur.first = i
			}
			cur.last = i
		case t.Clause:
			if cur.first >= 0 {
				cur.contour = t.Contour
				out = append(out, cur)
				cur = clause{first: -1}
			} else if len(out) > 0 && t.Contour != ContourNone {
				out[len(out)-1].contour = t.Contour
			}
		}
	}
	if cur.first >= 0 {
		out = append(out, cur)
	}
	return out
}

type composer struct {
	v       *Voice
	rng     *rand.Rand
	beat    float64 // seconds
	clauses []clause
	tl      Timeline

	voiced   bool                  // the line has at least one phoneme note
	voiceEnd float64               // seconds: the end of the last phoneme's slot
	snap     func(float64) float64 // the next beat at or after a time; nil without rhythm
	motifEnd float64               // seconds: when the motif's last note ends (the breath)
}

// contourShift is the pitch shift of the last three phonemes of a clause,
// from the third-last to the last.
var contourShift = map[Contour][3]float32{
	ContourFall: {-1, -2, -4},
	ContourRise: {1, 3, 5},
}

const (
	accentPitch    = 1   // semitones added to every note of an accented clause
	accentVelocity = 1.2 // velocity factor of an accented clause
)

func (c *composer) phonemes(tokens []Phoneme) {
	gate := float64(clampF(c.v.Length, 0.05, 1))
	end := c.lineEnding(tokens)
	t := 0.0
	ci := 0
	for i, tok := range tokens {
		for ci < len(c.clauses) && c.clauses[ci].last < i && tok.Kind == TokenPhoneme {
			ci++
		}
		c.reveal(t, tok.End)
		switch tok.Kind {
		case TokenPause:
			t += c.beat * float64(tok.Length)
		case TokenPhoneme:
			cl := &c.clauses[ci]
			if i == cl.first {
				cl.start = t
			}
			pitch, vel := c.tone(tokens, i, cl)
			slot := c.beat * float64(tok.Length) * end.stretch[i]
			c.note(tok, t, slot*min(1, gate+end.gate[i]), pitch, vel*end.fade[i])
			t += slot
			cl.end = t
			c.tl.Duration = seconds(t)
			c.voiced, c.voiceEnd = true, t
		}
	}
}

// tone returns the pitch and velocity of phoneme token i of clause cl: the
// voice's base, the phoneme's pitch and loudness, a random offset within
// the variation, and the clause's contour. It draws one random number, so
// the order of calls decides the line.
func (c *composer) tone(tokens []Phoneme, i int, cl *clause) (pitch, vel float32) {
	v, tok := c.v, &tokens[i]
	base := clampF(v.BaseNote, 24, 108) + clampF(v.Pitch, -24, 24)
	pitch = base + tok.Pitch + clampF(v.Variation, 0, 12)*(2*c.rng.Float32()-1)
	vel = clampF(v.Velocity, 0, 1) * tok.Velocity
	if cl.contour == ContourAccent {
		pitch += accentPitch
		vel *= accentVelocity
	}
	if shift, ok := contourShift[cl.contour]; ok {
		if k := c.fromEnd(tokens, i, cl); k < 3 {
			pitch += shift[2-k]
		}
	}
	return pitch, vel
}

// note adds the note of phoneme tok from t for length seconds.
func (c *composer) note(tok Phoneme, t, length float64, pitch, vel float32) {
	c.tl.Notes = append(c.tl.Notes, Note{
		On: seconds(t), Off: seconds(t + length), Instrument: tok.Category.Instrument(),
		Note: pitch, Velocity: clampF(vel, 0, 1), Pan: clampF(c.v.Pan, -1, 1),
	})
}

// fromEnd counts the phonemes after token i in its clause.
func (c *composer) fromEnd(tokens []Phoneme, i int, cl *clause) int {
	n := 0
	for j := i + 1; j <= cl.last; j++ {
		if tokens[j].Kind == TokenPhoneme {
			n++
		}
	}
	return n
}

func (c *composer) reveal(t float64, offset int) {
	at := seconds(t)
	if n := len(c.tl.Reveals); n > 0 && c.tl.Reveals[n-1].At == at {
		c.tl.Reveals[n-1].Offset = offset
		return
	}
	c.tl.Reveals = append(c.tl.Reveals, Reveal{At: at, Offset: offset})
}

// motifSpan is the time span and end rule of one motif note.
type motifSpan struct {
	start, end float64
	contour    Contour
}

// motif adds two to four soft notes, one per clause, starting an octave
// below the voice. A line of one clause is split in two halves; more than
// four clauses are grouped evenly into four. Each note moves from the
// previous one by its clause's contour: a fall goes down, a rise and an
// accent go up (an accent also louder), no contour steps a little either
// way.
func (c *composer) motif() {
	vol := clampF(c.v.MotifVolume, 0, 1)
	if vol == 0 || len(c.clauses) == 0 {
		return
	}
	var spans []motifSpan
	switch n := len(c.clauses); {
	case n == 1:
		cl := c.clauses[0]
		mid := (cl.start + cl.end) / 2
		spans = []motifSpan{{cl.start, mid, ContourNone}, {mid, cl.end, cl.contour}}
	default:
		groups := min(n, 4)
		for g := range groups {
			first, last := g*n/groups, (g+1)*n/groups-1
			spans = append(spans, motifSpan{c.clauses[first].start, c.clauses[last].end, c.clauses[last].contour})
		}
	}
	root := clampF(c.v.BaseNote, 24, 108) + clampF(c.v.Pitch, -24, 24) - 12
	degree := float32(0)
	for i, s := range spans {
		vel := vol
		if i > 0 {
			switch s.contour {
			case ContourFall:
				degree -= pick(c.rng, 3, 5)
			case ContourRise:
				degree += pick(c.rng, 5, 7)
			case ContourAccent:
				degree += pick(c.rng, 4, 7)
				vel = min(vol*accentVelocity, 1)
			default:
				degree += pick(c.rng, -2, 2, 3)
			}
			degree = clampF(degree, -7, 12)
		}
		if i == len(spans)-1 {
			s.end = max(s.end, c.motifEnd) // the last note rings on for the breath
		}
		c.tl.Notes = append(c.tl.Notes, Note{
			On: seconds(s.start), Off: seconds(s.end), Instrument: InstMotif,
			Note: root + degree, Velocity: vel, Pan: clampF(c.v.Pan, -1, 1),
		})
	}
}

func pick(r *rand.Rand, choices ...float32) float32 { return choices[r.IntN(len(choices))] }
