package session

import (
	"fmt"
	"time"

	"github.com/weakpixel/voice"
)

// The audition phrase is the one voiceplay -patches plays (copied, see the
// plan's Q5): a rising arpeggio of short notes from middle C on a melodic
// instrument, four hits at its note on a percussion sound.
var auditionSteps = []float32{0, 4, 7, 12, 7}

const (
	auditionNote = 60
	auditionLen  = 110 * time.Millisecond
	auditionGap  = 40 * time.Millisecond
	auditionHits = 4
	auditionVel  = 0.8
)

// noteEvent is one note on or off of an audition, at a time from its start.
type noteEvent struct {
	at   time.Duration
	inst voice.Instrument
	note float32
	on   bool
}

// audition plays a fixed list of note events on the session's stream,
// advanced by Update like a line.
type audition struct {
	events []noteEvent
	next   int
	t      time.Duration
}

func (a *audition) playing() bool { return a.next < len(a.events) }

// Audition plays a short phrase on one instrument of the current voice:
// "vowel", "sonorant", "fricative", "plosive", "motif" or the name of one
// of its percussion sounds. It stops a running line or audition first.
// Update advances it; Stop and Skip end it.
func (s *Session) Audition(name string) error {
	if s.v == nil {
		return fmt.Errorf("session: no voice")
	}
	inst, note, hits, err := s.instrument(name)
	if err != nil {
		return err
	}
	s.Stop()
	var ev []noteEvent
	var t time.Duration
	play := func(n float32) {
		ev = append(ev, noteEvent{t, inst, n, true})
		t += auditionLen
		ev = append(ev, noteEvent{t, inst, n, false})
		t += auditionGap
	}
	if hits {
		for range auditionHits {
			play(note)
		}
	} else {
		for _, step := range auditionSteps {
			play(note + step)
		}
	}
	s.aud = audition{events: ev}
	s.text, s.t = "", 0
	s.advanceAudition(0) // the first note sounds at once
	return nil
}

// instrument resolves an audition name to an instrument and the note to
// play; hits is true for a percussion sound.
func (s *Session) instrument(name string) (inst voice.Instrument, note float32, hits bool, err error) {
	for i := range voice.InstPercussion {
		if i.String() == name {
			return i, auditionNote, false, nil
		}
	}
	for i, p := range s.v.Percussion {
		if p.Name == name {
			return voice.InstPercussion + voice.Instrument(i), p.Note, true, nil
		}
	}
	return 0, 0, false, fmt.Errorf("session: the voice has no instrument %q", name)
}

// advanceAudition moves the audition on by dt and sends the notes that are
// due. Allocates nothing.
func (s *Session) advanceAudition(dt time.Duration) {
	a := &s.aud
	a.t += dt
	for a.playing() && a.events[a.next].at <= a.t {
		e := a.events[a.next]
		if e.on {
			s.stream.NoteOn(owner, e.inst, e.note, auditionVel, 0)
		} else {
			s.stream.NoteOff(owner, e.inst, e.note)
		}
		a.next++
	}
}

// stopAudition ends an audition and releases its notes.
func (s *Session) stopAudition() {
	if s.aud.playing() {
		s.aud = audition{}
		s.stream.Release(owner)
	}
}
