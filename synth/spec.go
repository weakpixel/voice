package synth

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/weakpixel/voice"
)

// SampleLoader resolves the sample name of a voice file's instrument (for
// example a path relative to the voice file) to a sample. It may return the
// same *Sample for the same name; samples are never modified.
type SampleLoader func(name string) (*Sample, error)

// PatchFromSpec turns an instrument of a voice file into a patch. A spec
// with a Sample needs load (nil returns an error for it). The result is
// validated.
func PatchFromSpec(spec voice.InstrumentSpec, load SampleLoader) (Patch, error) {
	ms := func(v float32) time.Duration { return time.Duration(float64(v) * float64(time.Millisecond)) }
	p := Patch{
		PulseWidth: spec.PulseWidth,
		Detune:     spec.Detune,
		Env:        Envelope{Attack: ms(spec.AttackMs), Decay: ms(spec.DecayMs), Sustain: spec.Sustain, Release: ms(spec.ReleaseMs)},
		Glide:      Glide{From: spec.GlideFrom, Time: ms(spec.GlideMs)},
		Vibrato:    Vibrato{Rate: spec.VibratoRate, Depth: spec.VibratoDepth, Delay: ms(spec.VibratoDelayMs)},
		Filter:     Filter{Cutoff: spec.Cutoff, Resonance: spec.Resonance},
		Gain:       spec.Gain,
		Reverb:     spec.Reverb,
		Layers:     spec.Layers,
		Unison:     spec.Unison,
		Spread:     spec.Spread,
	}
	if spec.Wave != "" {
		i := slices.Index(waveformNames[:], spec.Wave)
		if i < 0 {
			return Patch{}, fmt.Errorf("synth: unknown wave %q", spec.Wave)
		}
		p.Wave = Waveform(i)
	}
	if spec.Sample != "" {
		if load == nil {
			return Patch{}, fmt.Errorf("synth: sample %q: no sample loader", spec.Sample)
		}
		s, err := load(spec.Sample)
		if err != nil {
			return Patch{}, fmt.Errorf("synth: sample %q: %w", spec.Sample, err)
		}
		cp := *s // root and loop come from the voice file
		cp.Root = spec.Root
		if cp.Root == 0 {
			cp.Root = 60
		}
		cp.Loop, cp.LoopStart, cp.LoopEnd = spec.Loop, spec.LoopStart, spec.LoopEnd
		p.Sample = &cp
	}
	if err := p.Validate(); err != nil {
		return Patch{}, err
	}
	return p, nil
}

// SetVoice sets the patches of all instruments of owner from v (see
// [PatchFromSpec] and [Stream.SetPatch]). It reports the errors of every
// instrument together and sets nothing if any fails. Safe from any
// goroutine; allocates.
func (s *Stream) SetVoice(owner voice.Owner, v *voice.Voice, load SampleLoader) error {
	var patches [voice.InstrumentCount]Patch
	var errs []error
	for i, spec := range v.Instruments {
		p, err := PatchFromSpec(spec, load)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", voice.Instrument(i), err))
		}
		patches[i] = p
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	for i, p := range patches {
		if err := s.SetPatch(owner, voice.Instrument(i), p); err != nil {
			return err // cannot happen: validated above
		}
	}
	return nil
}
