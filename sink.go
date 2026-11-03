package voice

import "strconv"

// Owner identifies who plays a note, usually one speaker. A [Sink] keeps the
// notes of different owners apart, so stopping one line never touches
// another. Owners are small integers chosen by the caller.
type Owner uint16

// Instrument selects one of a voice's instruments: one per phoneme category
// plus the motif.
type Instrument uint8

// The instruments of a voice.
const (
	InstVowel     Instrument = iota // vowels
	InstSonorant                    // sonorants (m, n, l, r, ...)
	InstFricative                   // fricatives (s, f, h, ...)
	InstPlosive                     // plosives (p, t, k, ...)
	InstMotif                       // the quiet melody under a line
)

// InstrumentCount is the number of instruments of a voice. Valid
// [Instrument] values are 0 to InstrumentCount-1.
const InstrumentCount = int(InstMotif) + 1

var instrumentNames = [InstrumentCount]string{"vowel", "sonorant", "fricative", "plosive", "motif"}

// String returns the instrument's name as used in voice files ("vowel",
// "sonorant", "fricative", "plosive", "motif"), or "instrument(N)" for an
// out-of-range value.
func (i Instrument) String() string {
	if int(i) < InstrumentCount {
		return instrumentNames[i]
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
// NoteOff with the same owner and note, or by a Release of the owner.
type Sink interface {
	// NoteOn starts note on instrument inst of owner.
	NoteOn(owner Owner, inst Instrument, note float32, velocity float32, pan float32)
	// NoteOff releases the sounding notes of owner that have this pitch.
	NoteOff(owner Owner, note float32)
	// Release releases every note of owner.
	Release(owner Owner)
}
