package session

import (
	"errors"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/synth"
)

// Offline rendering: the tail after the line runs until the output stays
// below silenceLevel (-80 dB) for silenceTime, at most maxTail, so the
// waveform shows the reverb ringing out.
const (
	silenceLevel = 1e-4
	silenceTime  = 50 * time.Millisecond
	maxTail      = 2 * time.Second
)

// RenderLine renders text spoken by v in lang (nil is the default) at rate:
// the line from its start to its end, then the tail. It drives a
// voice.Speaker on a new stream block by block, so it sounds like playback
// (notes start at the block after they are due). It returns the two
// channels and the timeline.
func RenderLine(v *voice.Voice, lang *voice.Language, text string, o Options, rate int) (left, right []float32, tl voice.Timeline, err error) {
	if v == nil {
		return nil, nil, tl, errors.New("session: no voice")
	}
	st, err := synth.NewStream(synth.StreamConfig{SampleRate: rate})
	if err != nil {
		return nil, nil, tl, err
	}
	if err := st.SetVoice(owner, v, nil); err != nil {
		return nil, nil, tl, err
	}
	if lang == nil {
		lang = voice.DefaultLanguage()
	}
	sp := voice.NewSpeaker(st, owner)
	sp.Say(v, text, o.say(lang))
	tl = sp.Timeline()

	r := renderer{st: st, rate: rate}
	sp.Update(0)
	for !sp.Done() {
		r.block()
		sp.Update(r.step())
	}
	quiet, tail := 0, 0
	need, limit := frames(silenceTime, rate), frames(maxTail, rate)
	for quiet < need && tail < limit {
		n := r.block()
		tail += n
		if r.peakOfLast(n) < silenceLevel {
			quiet += n
		} else {
			quiet = 0
		}
	}
	return r.left, r.right, tl, nil
}

func frames(d time.Duration, rate int) int { return int(d.Seconds() * float64(rate)) }

// renderer appends blocks of synth.BlockFrames to two channels and reports
// the time they cover, counted from the total frames so it does not drift.
type renderer struct {
	st          *synth.Stream
	rate        int
	left, right []float32
	stepped     time.Duration
}

func (r *renderer) block() int {
	n := synth.BlockFrames
	at := len(r.left)
	r.left = append(r.left, make([]float32, n)...)
	r.right = append(r.right, make([]float32, n)...)
	r.st.Render(r.left[at:], r.right[at:])
	return n
}

// step returns the time rendered since the previous step.
func (r *renderer) step() time.Duration {
	now := time.Duration(float64(len(r.left)) / float64(r.rate) * float64(time.Second))
	d := now - r.stepped
	r.stepped = now
	return d
}

func (r *renderer) peakOfLast(n int) float32 {
	var p float32
	for i := len(r.left) - n; i < len(r.left); i++ {
		p = max(p, abs32(r.left[i]), abs32(r.right[i]))
	}
	return p
}
