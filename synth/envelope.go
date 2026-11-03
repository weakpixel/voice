package synth

type envStage uint8

const (
	envAttack envStage = iota
	envDecay
	envSustain
	envRelease
	envDone
)

// envelope is the per-voice state of an [Envelope]. All segments are linear
// and step once per frame. Attack and release are at least MinEnvelopeTime
// long, and so is the decay when it has to fall (sustain below 1): a jump in
// level is a click.
type envelope struct {
	stage       envStage
	level       float32
	sustain     float32
	attackStep  float32
	decayStep   float32
	releaseStep float32
	release     int  // frames of the release segment
	releaseDue  bool // note off during the attack, applied at its end
}

func (e *envelope) start(env Envelope, rate int) {
	minFrames := framesOf(MinEnvelopeTime, rate, 1)
	*e = envelope{sustain: min(max(env.Sustain, 0), 1)}
	e.attackStep = 1 / float32(framesOf(env.Attack, rate, minFrames))
	if e.sustain < 1 {
		e.decayStep = (1 - e.sustain) / float32(framesOf(env.Decay, rate, minFrames))
	}
	e.release = framesOf(env.Release, rate, minFrames)
}

// noteOff starts the release from the current level, so it never jumps. A
// note off during the attack takes effect when the attack ends: every note
// reaches its peak, so even a note on and off in the same block sounds.
func (e *envelope) noteOff() {
	if e.stage >= envRelease {
		return
	}
	if e.stage == envAttack {
		e.releaseDue = true
		return
	}
	e.stage = envRelease
	e.releaseStep = e.level / float32(e.release)
	if e.level <= 0 {
		e.stage = envDone
	}
}

// next advances one frame and returns the level of that frame.
func (e *envelope) next() float32 {
	switch e.stage {
	case envAttack:
		e.level += e.attackStep
		if e.level >= 1 {
			e.level = 1
			e.stage = envDecay
			if e.sustain >= 1 {
				e.stage = envSustain
			}
			if e.releaseDue {
				e.noteOff()
			}
		}
	case envDecay:
		e.level -= e.decayStep
		if e.level <= e.sustain {
			e.level = e.sustain
			e.stage = envSustain
			if e.sustain <= 0 {
				e.stage = envDone
			}
		}
	case envRelease:
		e.level -= e.releaseStep
		if e.level <= 0 {
			e.level = 0
			e.stage = envDone
		}
	case envDone:
		e.level = 0
	}
	return e.level
}

func (e *envelope) done() bool { return e.stage == envDone }
