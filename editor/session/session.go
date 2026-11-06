package session

import (
	"errors"
	"io"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/synth"
)

// owner is the one speaker of the editor on its stream.
const owner voice.Owner = 1

// Options tune one test line; they are voice.SayOptions without the
// language, which the session holds.
type Options struct {
	Speed    float32      // playback speed, 0 means 1
	Instant  bool         // show all text at once, play only the motif
	Clock    *voice.Clock // time a rhythmic voice to music
	MuteBeat bool         // leave out the rhythm's percussion
}

func (o Options) say(lang *voice.Language) voice.SayOptions {
	return voice.SayOptions{Language: lang, Speed: o.Speed, Instant: o.Instant, Clock: o.Clock, MuteBeat: o.MuteBeat}
}

// speed is the factor the speaker applies to time (Speed is ignored while
// a Clock is set, as in voice.SayOptions).
func (o Options) speed() float64 {
	if o.Speed <= 0 || o.Clock != nil {
		return 1
	}
	return float64(o.Speed)
}

// State is what the page shows after an Update.
type State struct {
	Revealed int           // bytes of the text to show
	Done     bool          // no line is playing
	Paused   bool          // the line is paused
	Audition bool          // an audition is playing (Done is true meanwhile)
	Time     time.Duration // position in the line's timeline (scaled by the speed)
	Events   []voice.Event // since the previous Update; reused, valid until the next one
}

// Session plays test lines of one voice on one synth.Stream through a
// voice.Speaker, the way a game does. Use it from one goroutine (the page's
// main loop); only Reader is read by the audio player.
type Session struct {
	stream *synth.Stream
	meter  meter
	sp     *voice.Speaker
	v      *voice.Voice
	lang   *voice.Language

	text  string
	speed float64
	t     time.Duration
	aud   audition

	samples samples
}

// New returns a session whose stream runs at rate (float32 stereo).
func New(rate int) (*Session, error) {
	st, err := synth.NewStream(synth.StreamConfig{SampleRate: rate})
	if err != nil {
		return nil, err
	}
	s := &Session{stream: st, sp: voice.NewSpeaker(st, owner), lang: voice.DefaultLanguage(), speed: 1}
	s.meter.r = st
	return s, nil
}

// Reader is the PCM the audio player reads: the stream, metered (see Peak).
// Interleaved float32 stereo, little endian, at the session's rate.
func (s *Session) Reader() io.Reader { return &s.meter }

// SampleRate returns the stream's sample rate.
func (s *Session) SampleRate() int { return s.stream.SampleRate() }

// SetVoice makes v the voice of the next lines and sets its instruments on
// the stream at once: notes already sounding keep their sound, the next ones
// use the new one. Sample instruments use the samples added with
// AddSample. On error (for example a sample that was not added) nothing
// changes.
func (s *Session) SetVoice(v *voice.Voice) error {
	if v == nil {
		return errors.New("session: no voice")
	}
	if err := s.stream.SetVoice(owner, v, s.load); err != nil {
		return err
	}
	s.v = v
	return nil
}

// Voice returns the current voice, nil before the first SetVoice.
func (s *Session) Voice() *voice.Voice { return s.v }

// SetLanguage sets the language of the next lines; nil is the default.
func (s *Session) SetLanguage(l *voice.Language) {
	if l == nil {
		l = voice.DefaultLanguage()
	}
	s.lang = l
}

// Say starts text with the current voice, replacing a running line.
func (s *Session) Say(text string, o Options) error {
	if s.v == nil {
		return errors.New("session: no voice")
	}
	s.stopAudition()
	s.text, s.speed, s.t = text, o.speed(), 0
	s.sp.Say(s.v, text, o.say(s.lang))
	return nil
}

// Update advances the line by the frame time dt. Allocates nothing.
func (s *Session) Update(dt time.Duration) {
	if s.aud.playing() {
		s.advanceAudition(max(dt, 0))
		s.t = s.aud.t
	}
	if !s.sp.Done() && !s.sp.Paused() && dt > 0 {
		s.t = min(s.t+time.Duration(float64(dt)*s.speed), s.sp.Timeline().Duration)
	}
	s.sp.Update(dt)
}

// Skip shows the whole text and releases the notes.
func (s *Session) Skip() { s.stopAudition(); s.sp.Skip() }

// Stop releases the notes and leaves the text as it is.
func (s *Session) Stop() { s.stopAudition(); s.sp.Stop() }

// Pause stops time and releases the sounding notes.
func (s *Session) Pause() { s.sp.Pause() }

// Resume continues a paused line.
func (s *Session) Resume() { s.sp.Resume() }

// State returns the speaker's state after the last Update.
func (s *Session) State() State {
	return State{Revealed: s.sp.Revealed(), Done: s.sp.Done(), Paused: s.sp.Paused(), Audition: s.aud.playing(), Time: s.t, Events: s.sp.Events()}
}

// Text returns the current line's text.
func (s *Session) Text() string { return s.text }

// Timeline returns the current line's timeline.
func (s *Session) Timeline() voice.Timeline { return s.sp.Timeline() }

// SetVolume sets the stream's master gain, 0 to 1.
func (s *Session) SetVolume(g float32) { s.stream.SetGain(min(max(g, 0), 1)) }

// FramesRead returns how many frames the audio player has read in all. The
// page watches it to tell a stalled audio output from a quiet line.
func (s *Session) FramesRead() int64 { return s.meter.read.Load() / 8 }

// Peak returns the loudest sample the audio player read since the previous
// call, 0 to 1.
func (s *Session) Peak() float32 { return s.meter.Peak() }
