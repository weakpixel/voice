package synth

import "fmt"

// Sample is a mono recording played by a [Patch]. It plays at its own pitch
// when the note equals Root and is resampled with linear interpolation for
// other notes. A one-shot sample ends the note at its end (with a fade over
// the last MinEnvelopeTime, so a sample that stops abruptly does not click);
// a looped one repeats LoopStart to LoopEnd until the envelope ends.
type Sample struct {
	Data []float32 // mono samples, -1 to 1
	Rate int       // sample rate of Data in hertz
	Root float32   // the note (MIDI numbering) Data sounds at
	Loop bool
	// LoopStart and LoopEnd are frame indexes; LoopEnd 0 means len(Data).
	LoopStart, LoopEnd int
}

func (s *Sample) validate() []error {
	var errs []error
	bad := func(field, format string, args ...any) {
		errs = append(errs, fmt.Errorf("synth: patch: sample.%s: %s", field, fmt.Sprintf(format, args...)))
	}
	if len(s.Data) == 0 {
		bad("data", "empty")
	}
	if s.Rate <= 0 {
		bad("rate", "%d is not positive", s.Rate)
	}
	if s.Loop {
		end := s.loopEnd()
		if s.LoopStart < 0 || s.LoopEnd < 0 || end > len(s.Data) || s.LoopStart >= end {
			bad("loop", "loop %d to %d does not fit %d frames", s.LoopStart, end, len(s.Data))
		}
	}
	return errs
}

func (s *Sample) loopEnd() int {
	if s.LoopEnd == 0 {
		return len(s.Data)
	}
	return s.LoopEnd
}

// samplePlayer is the per-voice playback position in a [Sample].
type samplePlayer struct {
	s     *Sample
	pos   float64 // frame position in s.Data
	step  float64 // frames of s.Data per output frame, set once per block
	fade  float64 // output frames of the end fade of a one-shot sample
	ended bool
}

func (p *samplePlayer) init(s *Sample, fadeFrames int) {
	*p = samplePlayer{s: s, fade: float64(fadeFrames)}
}

// next returns the interpolated value at the position and advances.
func (p *samplePlayer) next() float32 {
	if p.ended {
		return 0
	}
	d := p.s.Data
	i := int(p.pos)
	frac := float32(p.pos - float64(i))
	a := d[i]
	var b float32
	switch {
	case i+1 < len(d) && !(p.s.Loop && i+1 >= p.s.loopEnd()):
		b = d[i+1]
	case p.s.Loop:
		b = d[p.s.LoopStart]
	}
	y := a + (b-a)*frac
	if !p.s.Loop {
		// Fade out over the last frames: remaining output frames until the end.
		if left := (float64(len(d)) - p.pos) / p.step; left < p.fade {
			y *= float32(left / p.fade)
		}
	}
	p.pos += p.step
	if p.s.Loop {
		end := float64(p.s.loopEnd())
		for p.pos >= end {
			p.pos -= end - float64(p.s.LoopStart)
		}
	} else if p.pos >= float64(len(d)) {
		p.ended = true
	}
	return y
}
