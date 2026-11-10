package voice

import (
	"cmp"
	"slices"
	"time"
)

// SayOptions tune one line of [Speaker.Say].
type SayOptions struct {
	// Language splits the text; nil means DefaultLanguage().
	Language *Language
	// Speed scales playback: 2 is twice as fast. 0 means 1.
	Speed float32
	// Instant shows all text at once and plays the motif only.
	Instant bool
	// Clock and MuteBeat time a voice with a rhythm to the game's music;
	// see [ComposeOptions]. While a Clock is set, Speed is ignored, so the
	// line keeps the music's tempo.
	Clock    *Clock
	MuteBeat bool
	// Spans are parts of the text other voices speak, each on its own
	// Owner (see [VoiceSpan]). While the line plays the speaker also plays
	// on these owners and releases them on every exit path, so do not use
	// them for another line at the same time. The stream needs their
	// instruments (SetVoice) before the line starts.
	Spans []VoiceSpan
}

// EventKind is what happened to a line.
type EventKind uint8

const (
	Started   EventKind = iota // Say began a line
	Completed                  // the line played to its end
	Skipped                    // Skip ended the line early, all text shown
	Stopped                    // Stop, Close or a new Say ended the line
)

var eventNames = [...]string{"started", "completed", "skipped", "stopped"}

func (k EventKind) String() string {
	if int(k) < len(eventNames) {
		return eventNames[k]
	}
	return "event(?)"
}

// Event is something that happened to the line with number Line (counted
// from 1 per speaker, one per Say).
type Event struct {
	Kind EventKind
	Line uint64
}

// sinkEvent is one scheduled Sink call of the current line.
type sinkEvent struct {
	at   time.Duration
	on   bool
	note Note
}

// Speaker is the runtime that speaks lines: it plays a composed timeline on
// a [Sink] as time advances and tells the caller how much text to show.
//
// Drive it from one goroutine: Say a line, call Update with the frame time
// every frame, show text[:Revealed()], and read Events. The reveal depends
// only on the accumulated time, not on how it was split into updates.
// Every note it starts is ended by a NoteOff or by Release(owner) (and of
// the owners of the line's spans, see [SayOptions].Spans): Skip,
// Stop, Pause, Close and a Say over a running line release the owner's
// notes. Steady-state Update allocates nothing; Say allocates.
type Speaker struct {
	sink  Sink
	owner Owner

	tokens   []Phoneme
	owners   []Owner // owners[0] is the speaker's, owners[i] that of span i
	tl       Timeline
	schedule []sinkEvent
	next     int // next schedule entry to send
	reveal   int // next tl.Reveals entry to apply
	t        time.Duration
	speed    float64
	revealed int

	active bool // a line is playing (maybe paused)
	paused bool
	closed bool
	line   uint64

	events    []Event
	delivered int // events already returned by the previous Update
}

// NewSpeaker returns a speaker that plays its notes on sink as owner. Use a
// different owner for every speaker of one sink.
func NewSpeaker(sink Sink, owner Owner) *Speaker {
	return &Speaker{sink: sink, owner: owner, events: make([]Event, 0, 8)}
}

// Say starts speaking text with voice v, replacing a running line (which
// is released and gets a Stopped event). It emits Started. A Say after
// Close does nothing.
func (s *Speaker) Say(v *Voice, text string, opts SayOptions) {
	if s.closed {
		return
	}
	s.end(Stopped, false)
	lang := opts.Language
	if lang == nil {
		lang = DefaultLanguage()
	}
	s.tokens = AppendParse(s.tokens[:0], lang, text)
	s.tl = ComposeWith(v, text, s.tokens, ComposeOptions{Clock: opts.Clock, MuteBeat: opts.MuteBeat, Spans: opts.Spans})
	s.owners = append(s.owners[:0], s.owner)
	for _, sp := range opts.Spans {
		s.owners = append(s.owners, sp.Owner)
	}
	s.schedule = s.schedule[:0]
	for _, n := range s.tl.Notes {
		if opts.Instant && n.Instrument != InstMotif {
			continue
		}
		s.schedule = append(s.schedule, sinkEvent{n.On, true, n}, sinkEvent{n.Off, false, n})
	}
	// Note offs before note ons at the same time, so a repeated pitch is not
	// cut by its predecessor's off.
	slices.SortStableFunc(s.schedule, func(a, b sinkEvent) int {
		if c := cmp.Compare(a.at, b.at); c != 0 {
			return c
		}
		return cmpBool(a.on, b.on)
	})
	s.next, s.reveal, s.t, s.revealed = 0, 0, 0, 0
	s.speed = 1
	if opts.Speed > 0 && opts.Clock == nil {
		s.speed = float64(opts.Speed)
	}
	if opts.Instant {
		s.reveal, s.revealed = len(s.tl.Reveals), len(text)
	}
	s.active, s.paused = true, false
	s.line++
	s.events = append(s.events, Event{Started, s.line})
	s.send() // notes at time 0 start now, not one update later
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// Update advances the line by dt (scaled by SayOptions.Speed) unless it is
// paused, sends the notes that are due and moves the reveal. Events from
// the previous Update are dropped; the events of this Update and of calls
// since the previous one are in Events. Allocates nothing.
func (s *Speaker) Update(dt time.Duration) {
	n := copy(s.events, s.events[s.delivered:])
	s.events = s.events[:n]
	defer func() { s.delivered = len(s.events) }()
	if !s.active || s.paused || dt <= 0 {
		return
	}
	s.t += time.Duration(float64(dt) * s.speed)
	s.send()
	if s.t >= s.tl.Duration && s.next == len(s.schedule) {
		s.revealed = s.tl.TextLen
		s.active = false
		s.events = append(s.events, Event{Completed, s.line})
	}
}

// send makes the Sink calls and reveals that are due at s.t.
func (s *Speaker) send() {
	for ; s.next < len(s.schedule) && s.schedule[s.next].at <= s.t; s.next++ {
		e := &s.schedule[s.next]
		owner := s.owners[e.note.Span]
		if e.on {
			s.sink.NoteOn(owner, e.note.Instrument, e.note.Note, e.note.Velocity, e.note.Pan)
		} else {
			s.sink.NoteOff(owner, e.note.Instrument, e.note.Note)
		}
	}
	for ; s.reveal < len(s.tl.Reveals) && s.tl.Reveals[s.reveal].At <= s.t; s.reveal++ {
		s.revealed = s.tl.Reveals[s.reveal].Offset
	}
}

// end finishes a running line with event kind, releasing its notes; with
// revealAll it shows the whole text.
func (s *Speaker) end(kind EventKind, revealAll bool) {
	if !s.active {
		return
	}
	s.release()
	s.active, s.paused = false, false
	if revealAll {
		s.revealed = s.tl.TextLen
	}
	s.events = append(s.events, Event{kind, s.line})
}

// release releases the speaker's owner and every other owner of the line's
// spans (each once).
func (s *Speaker) release() {
	for i, o := range s.owners {
		if !slices.Contains(s.owners[:i], o) {
			s.sink.Release(o)
		}
	}
}

// Skip ends the running line at once: all text is shown, its notes are
// released, and it emits Skipped. Without a running line it does nothing.
func (s *Speaker) Skip() { s.end(Skipped, true) }

// Stop ends the running line where it is (the shown text stays as it is),
// releases its notes and emits Stopped.
func (s *Speaker) Stop() { s.end(Stopped, false) }

// Pause holds the line: time stops, and the sounding notes are released (a
// held note would hang). Resume continues with the next notes.
func (s *Speaker) Pause() {
	if s.active && !s.paused {
		s.paused = true
		s.release()
	}
}

// Resume continues a paused line.
func (s *Speaker) Resume() { s.paused = false }

// Close stops the running line like Stop and makes later Say calls do
// nothing. The speaker stays readable.
func (s *Speaker) Close() {
	s.end(Stopped, false)
	s.closed = true
}

// Revealed returns how many bytes of the line's text to show: text[:n] is
// always whole tokens (never half a UTF-8 character).
func (s *Speaker) Revealed() int { return s.revealed }

// Done reports whether no line is playing (none started, or the last one
// completed, was skipped or stopped).
func (s *Speaker) Done() bool { return !s.active }

// Paused reports whether the running line is paused.
func (s *Speaker) Paused() bool { return s.active && s.paused }

// Events returns what happened since the previous Update began: events of
// Say, Skip, Stop and Close calls since then and of the Update itself. The
// slice is reused: it is valid until the next Update.
func (s *Speaker) Events() []Event { return s.events }

// Timeline returns the current line's timeline (empty before the first
// Say). Do not modify its slices.
func (s *Speaker) Timeline() Timeline { return s.tl }
