package voice

// InstrumentSpec describes the sound of one instrument of a voice, as written
// in a voice file: an oscillator patch, or a sample reference when Sample is
// set. The synthesizer turns it into a patch (synth.PatchFromSpec); its
// guide section lists what each field does. Times are in milliseconds.
//
// Zero values mean defaults: Wave "" is "sine", PulseWidth 0 is 0.5, Gain 0
// is 1, Root 0 is 60.
type InstrumentSpec struct {
	Wave           string  `json:"wave,omitempty"` // sine, triangle, square, saw, noise
	PulseWidth     float32 `json:"pulseWidth,omitempty"`
	Detune         float32 `json:"detune,omitempty"`
	AttackMs       float32 `json:"attackMs,omitempty"`
	DecayMs        float32 `json:"decayMs,omitempty"`
	Sustain        float32 `json:"sustain,omitempty"`
	ReleaseMs      float32 `json:"releaseMs,omitempty"`
	GlideFrom      float32 `json:"glideFrom,omitempty"`
	GlideMs        float32 `json:"glideMs,omitempty"`
	VibratoRate    float32 `json:"vibratoRate,omitempty"`
	VibratoDepth   float32 `json:"vibratoDepth,omitempty"`
	VibratoDelayMs float32 `json:"vibratoDelayMs,omitempty"`
	Cutoff         float32 `json:"cutoff,omitempty"`
	Resonance      float32 `json:"resonance,omitempty"`
	Gain           float32 `json:"gain,omitempty"`

	// Sample names a sound file the caller resolves (for example a path in
	// an fs.FS next to the voice file). Root is the note it sounds at.
	Sample    string  `json:"sample,omitempty"`
	Root      float32 `json:"root,omitempty"`
	Loop      bool    `json:"loop,omitempty"`
	LoopStart int     `json:"loopStart,omitempty"`
	LoopEnd   int     `json:"loopEnd,omitempty"`
}

// Waves lists the valid values of InstrumentSpec.Wave.
var Waves = []string{"sine", "triangle", "square", "saw", "noise"}

func (s *InstrumentSpec) validate(errs *fieldErrors, path string) {
	if s.Wave != "" {
		known := false
		for _, w := range Waves {
			known = known || w == s.Wave
		}
		if !known {
			errs.add(path+".wave", "%q is not one of sine, triangle, square, saw, noise", s.Wave)
		}
		if s.Sample != "" {
			errs.add(path+".wave", "an instrument is a wave or a sample, not both")
		}
	}
	if errs.finite(path+".pulseWidth", s.PulseWidth) && s.PulseWidth != 0 && (s.PulseWidth < 0.01 || s.PulseWidth > 0.99) {
		errs.add(path+".pulseWidth", "%g is outside 0.01 to 0.99", s.PulseWidth)
	}
	errs.inRange(path+".detune", s.Detune, -24, 24)
	for _, ms := range []struct {
		name string
		v    float32
	}{{"attackMs", s.AttackMs}, {"decayMs", s.DecayMs}, {"releaseMs", s.ReleaseMs}, {"glideMs", s.GlideMs}, {"vibratoDelayMs", s.VibratoDelayMs}} {
		errs.inRange(path+"."+ms.name, ms.v, 0, 10000)
	}
	errs.inRange(path+".sustain", s.Sustain, 0, 1)
	errs.inRange(path+".glideFrom", s.GlideFrom, -24, 24)
	errs.inRange(path+".vibratoRate", s.VibratoRate, 0, 50)
	errs.inRange(path+".vibratoDepth", s.VibratoDepth, -12, 12)
	errs.inRange(path+".cutoff", s.Cutoff, 0, 100000)
	if errs.finite(path+".resonance", s.Resonance) && (s.Resonance < 0 || s.Resonance >= 1) {
		errs.add(path+".resonance", "%g is outside 0 to below 1", s.Resonance)
	}
	errs.inRange(path+".gain", s.Gain, 0, 4)
	errs.inRange(path+".root", s.Root, 0, 127)
	if s.LoopStart < 0 || s.LoopEnd < 0 || s.LoopEnd != 0 && s.LoopEnd <= s.LoopStart {
		errs.add(path+".loopStart", "loop %d to %d is not a range", s.LoopStart, s.LoopEnd)
	}
	if s.Sample == "" && (s.Root != 0 || s.Loop || s.LoopStart != 0 || s.LoopEnd != 0) {
		errs.add(path+".sample", "root and loop need a sample")
	}
}
