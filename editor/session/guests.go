package session

import (
	"errors"
	"fmt"

	"github.com/weakpixel/voice"
)

// A test line may give words to other voices with the library's markup
// ({name}…{/}). The page loads the voices the names stand for (a starter, a
// workspace voice file, a dialogue speaker) with SetGuest; each guest plays
// on its own owner of the stream, after the session's own.

// guest is a voice a line's markup may name, on its own owner.
type guest struct {
	v     *voice.Voice
	owner voice.Owner
}

// SetGuest makes v the voice of the markup name in the next lines and sets
// its instruments on the stream (on the guest's own owner). Setting a name
// again replaces its voice. On error nothing changes.
func (s *Session) SetGuest(name string, v *voice.Voice) error {
	if v == nil {
		return errors.New("session: no voice")
	}
	if s.guests == nil {
		s.guests = map[string]guest{}
	}
	g, ok := s.guests[name]
	if !ok {
		g.owner = owner + 1 + voice.Owner(len(s.guests))
	}
	if err := s.stream.SetVoice(g.owner, v, s.load); err != nil {
		return err
	}
	g.v = v
	s.guests[name] = g
	return nil
}

// ClearGuests forgets every guest voice. A running line keeps its notes.
func (s *Session) ClearGuests() { clear(s.guests) }

// line parses the markup of src: the text to show and speak, the voice
// spans of the names that have a guest, and the problems (markup errors,
// names without a guest; those spans are spoken by the session's voice).
func (s *Session) line(src string) (string, []voice.VoiceSpan, []Problem) {
	text, ms, err := voice.ParseMarkup(src)
	var probs []Problem
	if err != nil {
		for _, e := range err.(interface{ Unwrap() []error }).Unwrap() {
			if me, ok := e.(*voice.MarkupError); ok {
				probs = append(probs, Problem{Message: fmt.Sprintf("byte %d: %s", me.Offset, me.Message)})
			}
		}
	}
	var spans []voice.VoiceSpan
	for _, m := range ms {
		g, ok := s.guests[m.Name]
		if !ok {
			probs = append(probs, Problem{Message: fmt.Sprintf("{%s}: no voice of that name", m.Name)})
			continue
		}
		spans = append(spans, voice.VoiceSpan{Start: m.Start, End: m.End, Voice: g.v, Owner: g.owner, Impression: m.Impression})
	}
	return text, spans, probs
}

// Problems returns the markup problems of the current line.
func (s *Session) Problems() []Problem { return s.problems }
