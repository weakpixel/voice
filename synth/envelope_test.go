package synth

import (
	"testing"
	"time"
)

func TestEnvelopeSegments(t *testing.T) {
	env := Envelope{Attack: 10 * time.Millisecond, Decay: 20 * time.Millisecond, Sustain: 0.5, Release: 30 * time.Millisecond}
	var e envelope
	e.start(env, testRate)
	at := func(n int) float32 {
		var l float32
		for range n {
			l = e.next()
		}
		return l
	}
	if l := at(frames(10 * time.Millisecond)); l != 1 {
		t.Errorf("after attack level %g, want 1", l)
	}
	if l := at(frames(10 * time.Millisecond)); l < 0.74 || l > 0.76 {
		t.Errorf("mid decay level %g, want 0.75", l)
	}
	if l := at(frames(10*time.Millisecond) + 1); l != 0.5 {
		t.Errorf("sustain level %g, want 0.5", l)
	}
	if l := at(frames(time.Second)); l != 0.5 || e.done() {
		t.Errorf("held level %g done %v, want 0.5 and not done", l, e.done())
	}
	e.noteOff()
	if l := at(frames(15 * time.Millisecond)); l < 0.24 || l > 0.26 {
		t.Errorf("mid release level %g, want 0.25", l)
	}
	at(frames(15 * time.Millisecond))
	if !e.done() {
		t.Errorf("not done after release")
	}
}

func TestEnvelopeMinimumTimes(t *testing.T) {
	// Zero times are raised to MinEnvelopeTime: no jump from 0 to 1.
	var e envelope
	e.start(Envelope{Sustain: 0}, testRate)
	minFrames := frames(MinEnvelopeTime)
	var prev, maxJump float32
	n := 0
	for !e.done() {
		l := e.next()
		maxJump = max(maxJump, l-prev, prev-l)
		prev = l
		n++
	}
	if want := 1 / float32(minFrames); maxJump > want*1.01 {
		t.Errorf("max jump %g, want at most %g", maxJump, want)
	}
	if n < 2*minFrames {
		t.Errorf("blip of %d frames, want at least %d (attack + decay)", n, 2*minFrames)
	}
}

func TestEnvelopeNoteOffDuringAttack(t *testing.T) {
	var e envelope
	e.start(Envelope{Attack: 100 * time.Millisecond, Sustain: 1, Release: 10 * time.Millisecond}, testRate)
	var l float32
	for range frames(10 * time.Millisecond) {
		l = e.next()
	}
	e.noteOff()
	// The attack completes first (about 90 ms more), then the release starts.
	n := 0
	for l < 1 {
		l = e.next()
		n++
	}
	if want := frames(90 * time.Millisecond); n < want-2 || n > want+2 {
		t.Errorf("attack completed after %d more frames, want %d", n, want)
	}
	if l := e.next(); l >= 1 || l < 0.99 {
		t.Errorf("release from the peak went to %g, want a small step down", l)
	}
}

func TestNoClicks(t *testing.T) {
	// Start and end of a held note and of a one-shot blip: the steps
	// between frames stay at what the waveform itself produces.
	patches := map[string]Patch{
		"held":  {Wave: Square, Env: Envelope{Sustain: 1}},
		"blip":  {Wave: Saw, Env: Envelope{Decay: 30 * time.Millisecond}},
		"sine0": {Env: Envelope{Sustain: 1}},
	}
	for name, p := range patches {
		t.Run(name, func(t *testing.T) {
			s := newTestStream(t, 4)
			setPatch(t, s, 1, 0, p)
			s.NoteOn(1, 0, 57, 1, 0)
			x := render(s, 100*time.Millisecond)
			s.NoteOff(1, 0, 57)
			x = append(x, render(s, 100*time.Millisecond)...)
			// A 220 Hz waveform steps at most ~2πf/rate per frame for the
			// sine; band-limited square and saw jumps are what PolyBLEP
			// leaves. Measure the first and last 2 ms only.
			edge := frames(MinEnvelopeTime)
			if s := maxStep(x[:edge]); s > 0.1 {
				t.Errorf("start step %g", s)
			}
			end := len(x) - frames(100*time.Millisecond)
			if s := maxStep(x[end-edge : end+edge]); s > 1.05 {
				t.Errorf("end step %g", s)
			}
			if p := peak(x[len(x)-frames(10*time.Millisecond):]); p != 0 {
				t.Errorf("still sounding %g after release", p)
			}
		})
	}
}
