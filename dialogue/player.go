package dialogue

import (
	"fmt"
	"time"

	"github.com/weakpixel/voice"
)

// Phase is where the player is in the current line.
type Phase uint8

const (
	// Waiting is the line's delay, before it starts.
	Waiting Phase = iota
	// Speaking is the line typing out and sounding.
	Speaking
	// Holding is the finished line, until Next or its hold time passes.
	Holding
	// Finished is after the last line, or after Stop.
	Finished
)

var phaseNames = [...]string{"waiting", "speaking", "holding", "finished"}

func (p Phase) String() string {
	if int(p) < len(phaseNames) {
		return phaseNames[p]
	}
	return "phase(?)"
}

// EventKind is what happened in a dialogue.
type EventKind uint8

const (
	LineStarted      EventKind = iota // the line's delay passed and it began to speak
	LineDone                          // the line completed or was skipped; its text is complete
	DialogueFinished                  // the last line is done and gave way
)

var eventNames = [...]string{"line started", "line done", "finished"}

func (k EventKind) String() string {
	if int(k) < len(eventNames) {
		return eventNames[k]
	}
	return "event(?)"
}

// Event is something that happened in the dialogue.
type Event struct {
	Kind EventKind
	// Line is the index of the line (for DialogueFinished, the last one).
	Line int
	// At is the player's clock when it happened: the sum of the Update
	// times so far.
	At time.Duration
}

// Options tune a [Player].
type Options struct {
	// Languages resolves a voice's language name (voice.Voice.Language) to
	// a language. Nil, or a nil result, means the default language.
	Languages func(name string) *voice.Language
	// AutoAdvance goes on after every line by itself, whatever the script
	// says, using each line's hold time.
	AutoAdvance bool
	// Start is the index of the first line to play.
	Start int
}

// SpeakerVoice is the voice a speaker plays with.
type SpeakerVoice struct {
	Owner   voice.Owner
	Speaker string // the speaker's ID
	// Voice is a copy of the loaded voice with the speaker's pan applied.
	Voice *voice.Voice
}

type voiceSpeaker struct {
	sp    *voice.Speaker
	voice *voice.Voice
}

// Player speaks a [Script] on a [voice.Sink]: it runs each line's delay, the
// line itself and the hold, and tells the caller what to show.
//
// Drive it from one goroutine: call Update with the frame time every frame,
// Next on a click or key press, show Line().Text[:Revealed()], and read
// Events. Every note it starts is ended: Next (skipping), Start, Pause, Stop
// and Close release the notes of the speaker involved. Steady-state Update
// allocates nothing; starting a line allocates.
type Player struct {
	script *Script
	opts   Options
	pv     []voiceSpeaker // by speaker index
	voices []SpeakerVoice

	idx     int
	spk     int // speaker index of the current line
	phase   Phase
	left    time.Duration // delay or hold still to pass
	auto    bool          // the finished line advances by itself
	paused  bool
	closed  bool
	now     time.Duration
	events  []Event
	deliver int
}

// NewPlayer returns a player for s on sink. Speaker i is owner i + 1. voices
// maps each speaker's voice reference to its voice (see [LoadVoices]); the
// player works on copies, with the speaker's pan applied. It starts the line
// opts.Start at once. It returns an error for a missing voice or an invalid
// voice or start.
//
// The sink needs the speakers' instruments before the first note: for each
// entry of [Player.Voices] call the sink's own SetVoice (synth.Stream has
// one) with the owner and voice.
func NewPlayer(sink voice.Sink, s *Script, voices map[string]*voice.Voice, opts Options) (*Player, error) {
	if opts.Start < 0 || opts.Start >= len(s.Lines) {
		return nil, fmt.Errorf("dialogue: start line %d is outside 0 to %d", opts.Start, len(s.Lines)-1)
	}
	p := &Player{script: s, opts: opts, events: make([]Event, 0, 8)}
	var errs problems
	for i, sp := range s.Speakers {
		v, ok := voices[sp.Voice]
		if !ok {
			errs.add(fmt.Sprintf("speakers[%d].voice", i), "no voice for %q", sp.Voice)
			continue
		}
		c := *v
		if sp.Pan != nil {
			c.Pan = *sp.Pan
		}
		owner := voice.Owner(i + 1)
		p.pv = append(p.pv, voiceSpeaker{voice.NewSpeaker(sink, owner), &c})
		p.voices = append(p.voices, SpeakerVoice{owner, sp.ID, &c})
	}
	if len(errs.errs) > 0 {
		return nil, joinProblems(errs.errs)
	}
	p.beginLine(opts.Start)
	return p, nil
}

// Voices lists the owner and voice of every speaker, in script order. The
// caller gives them to the sink before the first Update.
func (p *Player) Voices() []SpeakerVoice { return p.voices }

// cur is the speaker of the current line.
func (p *Player) cur() *voiceSpeaker { return &p.pv[p.spk] }

func (p *Player) emit(k EventKind) { p.events = append(p.events, Event{k, p.idx, p.now}) }

// beginLine makes line i current: it waits for its delay, or starts at once.
func (p *Player) beginLine(i int) {
	p.idx = i
	l := p.script.Lines[i]
	p.spk = p.script.SpeakerIndex(l.Speaker)
	p.auto = p.opts.AutoAdvance || l.Advance == AdvanceAuto
	if l.Delay > 0 {
		p.phase, p.left = Waiting, seconds(l.Delay)
		return
	}
	p.say()
}

func (p *Player) say() {
	l := p.script.Lines[p.idx]
	vs := p.cur()
	var lang *voice.Language
	if p.opts.Languages != nil {
		lang = p.opts.Languages(vs.voice.Language)
	}
	vs.sp.Say(vs.voice, l.Text, voice.SayOptions{Language: lang, Speed: l.Speed, Instant: l.Instant})
	p.phase = Speaking
	p.emit(LineStarted)
}

// lineDone moves a finished line to its hold.
func (p *Player) lineDone() {
	p.phase = Holding
	p.left = 0
	if p.auto {
		p.left = seconds(p.script.Lines[p.idx].Hold)
	}
	p.emit(LineDone)
}

// advance goes on to the next line, or finishes after the last.
func (p *Player) advance() {
	if p.idx+1 >= len(p.script.Lines) {
		p.phase = Finished
		p.emit(DialogueFinished)
		return
	}
	p.beginLine(p.idx + 1)
}

func seconds(s float32) time.Duration { return time.Duration(float64(s) * float64(time.Second)) }

// Update advances the dialogue by dt: it passes the delay, moves the
// current line and passes the hold of an auto-advancing one, going on to the
// next line when due. Time left over when a phase ends goes to the next
// phase, except that a line that completes inside dt uses all of it. Events
// from the previous Update are dropped; Events has those of this Update and
// of calls since the previous one. Allocates nothing in steady state.
func (p *Player) Update(dt time.Duration) {
	n := copy(p.events, p.events[p.deliver:])
	p.events = p.events[:n]
	defer func() { p.deliver = len(p.events) }()
	if p.closed {
		return
	}
	// Speakers other than the speaking one only drain their events.
	for i := range p.pv {
		if p.phase != Speaking || i != p.spk {
			p.pv[i].sp.Update(0)
		}
	}
	if p.paused || dt <= 0 {
		if p.phase == Speaking {
			p.cur().sp.Update(0)
		}
		return
	}
	for {
		switch p.phase {
		case Waiting:
			if dt < p.left {
				p.left -= dt
				p.now += dt
				return
			}
			dt -= p.left
			p.now += p.left
			p.say()
		case Speaking:
			vs := p.cur()
			vs.sp.Update(dt)
			p.now += dt
			dt = 0
			if !vs.sp.Done() {
				return
			}
			p.lineDone()
		case Holding:
			if !p.auto {
				p.now += dt
				return
			}
			if dt < p.left {
				p.left -= dt
				p.now += dt
				return
			}
			dt -= p.left
			p.now += p.left
			p.advance()
		default: // Finished
			p.now += dt
			return
		}
	}
}

// Next is the player's input: a click or key press. While a line types out
// it shows the rest of it at once (the line is done); while a finished line
// holds it goes on to the next line (or finishes); during a delay and after
// the end it does nothing.
func (p *Player) Next() {
	if p.closed {
		return
	}
	switch p.phase {
	case Speaking:
		p.cur().sp.Skip()
		p.lineDone()
	case Holding:
		p.advance()
	}
}

// Start jumps to line i, stopping the current one, and plays it from its
// delay. It also resumes a paused or finished dialogue. It panics for an
// index outside the script.
func (p *Player) Start(i int) {
	if i < 0 || i >= len(p.script.Lines) {
		panic(fmt.Sprintf("dialogue: Start(%d): line index outside 0 to %d", i, len(p.script.Lines)-1))
	}
	if p.closed {
		return
	}
	p.stopSpeaking()
	p.paused = false
	p.beginLine(i)
}

func (p *Player) stopSpeaking() {
	if p.phase == Speaking {
		p.cur().sp.Stop()
	}
}

// Pause holds time everywhere; the sounding notes are released, as with
// [voice.Speaker.Pause]. Resume continues.
func (p *Player) Pause() {
	if p.closed || p.paused {
		return
	}
	p.paused = true
	if p.phase == Speaking {
		p.cur().sp.Pause()
	}
}

// Resume continues a paused dialogue.
func (p *Player) Resume() {
	if !p.paused {
		return
	}
	p.paused = false
	if p.phase == Speaking {
		p.cur().sp.Resume()
	}
}

// Stop ends the dialogue where it is: the notes are released, the phase is
// Finished, and no event is emitted. Start plays it again.
func (p *Player) Stop() {
	if p.closed {
		return
	}
	p.stopSpeaking()
	p.paused = false
	p.phase = Finished
}

// Close stops like Stop and makes every later call do nothing. The player
// stays readable.
func (p *Player) Close() {
	p.Stop()
	for i := range p.pv {
		p.pv[i].sp.Close()
	}
	p.closed = true
}

// Index is the index of the current line (the last one that played, once
// finished).
func (p *Player) Index() int { return p.idx }

// Line is the current line.
func (p *Player) Line() Line { return p.script.Lines[p.idx] }

// Speaker is the speaker of the current line.
func (p *Player) Speaker() Speaker { return p.script.Speakers[p.spk] }

// Phase is where the player is in the current line.
func (p *Player) Phase() Phase { return p.phase }

// Paused reports whether Pause was called and not Resume.
func (p *Player) Paused() bool { return p.paused }

// Revealed is how many bytes of Line().Text to show: none during the delay,
// the speaker's reveal while the line speaks, all of it afterwards.
func (p *Player) Revealed() int {
	switch p.phase {
	case Waiting:
		return 0
	case Speaking:
		return p.cur().sp.Revealed()
	}
	return len(p.Line().Text)
}

// Events returns what happened since the previous Update began: the events
// of Next and Start calls since then and those of the Update itself. The
// slice is reused: it is valid until the next Update.
func (p *Player) Events() []Event { return p.events }
