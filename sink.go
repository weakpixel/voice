package voice

import "strconv"

// Owner identifies who plays a note, usually one speaker. A [Sink] keeps the
// notes of different owners apart, so stopping one line never touches
// another. Owners are small integers chosen by the caller.
type Owner uint16

// Instrument selects one of a voice's instruments: one per phoneme category,
// the motif, and the voice's percussion sounds.
type Instrument uint8

// The instruments of a voice.
const (
	InstVowel     Instrument = iota // vowels
	InstSonorant                    // sonorants (m, n, l, r, ...)
	InstFricative                   // fricatives (s, f, h, ...)
	InstPlosive                     // plosives (p, t, k, ...)
	InstMotif                       // the quiet melody under a line
)

// InstPercussion is the instrument of a voice's first percussion sound:
// sound i of [Voice.Percussion] plays on InstPercussion + i.
const InstPercussion = InstMotif + 1

// MaxPercussion is the most percussion sounds a voice can have.
const MaxPercussion = 8

// InstrumentCount is the number of instruments a [Sink] must handle per
// owner: the five voice instruments and MaxPercussion percussion sounds.
// Valid [Instrument] values are 0 to InstrumentCount-1.
const InstrumentCount = int(InstPercussion) + MaxPercussion

var instrumentNames = [InstPercussion]string{"vowel", "sonorant", "fricative", "plosive", "motif"}

// String returns the instrument's name as used in voice files ("vowel",
// "sonorant", "fricative", "plosive", "motif"), "percussion(N)" for the
// percussion sounds, or "instrument(N)" for an out-of-range value.
func (i Instrument) String() string {
	switch {
	case i < InstPercussion:
		return instrumentNames[i]
	case int(i) < InstrumentCount:
		return "percussion(" + strconv.Itoa(int(i-InstPercussion)) + ")"
	}
	return "instrument(" + strconv.Itoa(int(i)) + ")"
}

// Sink receives the notes of a line. It is the seam between composing and
// sound: the built-in synthesizer (synth.Stream) implements it, a SoundFont
// backend or a recording test sink can too.
//
// Notes are float semitones in MIDI numbering (60 is middle C), so pitch
// variation is not rounded. Velocity is 0 to 1, pan is -1 (left) to 1
// (right).
//
// Contract for implementations: the methods are called from the caller's
// goroutine (a game loop, a test), possibly while audio is being rendered on
// another goroutine; they must be safe for that, must not block for long and
// must not allocate in steady state. Every NoteOn is eventually matched by a
// NoteOff with the same owner, instrument and note, or by a Release of the
// owner.
type Sink interface {
	// NoteOn starts note on instrument inst of owner.
	NoteOn(owner Owner, inst Instrument, note float32, velocity float32, pan float32)
	// NoteOff releases the sounding notes of owner on instrument inst that
	// have this pitch. The instrument keeps a percussion hit from ending a
	// motif note at the same pitch.
	NoteOff(owner Owner, inst Instrument, note float32)
	// Release releases every note of owner.
	Release(owner Owner)
}
