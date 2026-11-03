// Package voicetest has test helpers for code that uses the voice library.
package voicetest

import (
	"fmt"
	"sync"

	"github.com/weakpixel/voice"
)

// CallKind is the Sink method of a recorded [Call].
type CallKind uint8

const (
	NoteOn CallKind = iota
	NoteOff
	Release
)

func (k CallKind) String() string {
	switch k {
	case NoteOn:
		return "on"
	case NoteOff:
		return "off"
	case Release:
		return "release"
	}
	return fmt.Sprintf("kind(%d)", k)
}

// Call is one recorded Sink call. Fields a method does not have are zero.
type Call struct {
	Kind       CallKind
	Owner      voice.Owner
	Instrument voice.Instrument
	Note       float32
	Velocity   float32
	Pan        float32
}

// RecordingSink is a voice.Sink that records every call instead of making
// sound. It is safe for concurrent use. Its methods allocate (it is for
// tests, not hot paths), so preallocate with [NewRecordingSink] for alloc
// tests.
type RecordingSink struct {
	mu    sync.Mutex
	calls []Call
}

// NewRecordingSink returns a sink with room for capacity calls before it
// allocates.
func NewRecordingSink(capacity int) *RecordingSink {
	return &RecordingSink{calls: make([]Call, 0, capacity)}
}

// NoteOn implements voice.Sink.
func (s *RecordingSink) NoteOn(owner voice.Owner, inst voice.Instrument, note, velocity, pan float32) {
	s.add(Call{Kind: NoteOn, Owner: owner, Instrument: inst, Note: note, Velocity: velocity, Pan: pan})
}

// NoteOff implements voice.Sink.
func (s *RecordingSink) NoteOff(owner voice.Owner, note float32) {
	s.add(Call{Kind: NoteOff, Owner: owner, Note: note})
}

// Release implements voice.Sink.
func (s *RecordingSink) Release(owner voice.Owner) {
	s.add(Call{Kind: Release, Owner: owner})
}

func (s *RecordingSink) add(c Call) {
	s.mu.Lock()
	s.calls = append(s.calls, c)
	s.mu.Unlock()
}

// Calls returns a copy of the calls so far, oldest first.
func (s *RecordingSink) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.calls...)
}

// Reset forgets the recorded calls, keeping the capacity.
func (s *RecordingSink) Reset() {
	s.mu.Lock()
	s.calls = s.calls[:0]
	s.mu.Unlock()
}

// Sounding returns the NoteOn calls of owner that are still sounding: not
// followed by a NoteOff of the same pitch (which ends every sounding note
// of that pitch, as synth.Stream does) or a Release. A test that ends a
// line checks that it is empty.
func (s *RecordingSink) Sounding(owner voice.Owner) []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	var on []Call
	for _, c := range s.calls {
		if c.Owner != owner {
			continue
		}
		switch c.Kind {
		case NoteOn:
			on = append(on, c)
		case NoteOff:
			on = deleteNote(on, c.Note)
		case Release:
			on = on[:0]
		}
	}
	return on
}

func deleteNote(on []Call, note float32) []Call {
	out := on[:0]
	for _, c := range on {
		if c.Note != note {
			out = append(out, c)
		}
	}
	return out
}

var _ voice.Sink = (*RecordingSink)(nil)
