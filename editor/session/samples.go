package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/weakpixel/voice/synth"
	"github.com/weakpixel/voice/wav"
)

// samples holds the sample files the user added, by the name a voice file
// uses for them (the file name). The page keeps the files and adds them
// again after a reload; the session only holds them decoded.
type samples map[string]*synth.Sample

// AddSample decodes a 16-bit PCM WAV file and makes it the sample called
// name, replacing one of that name. Instruments of the current voice keep
// their sound until the voice is set again.
func (s *Session) AddSample(name string, data []byte) error {
	d, rate, err := wav.Read(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if s.samples == nil {
		s.samples = samples{}
	}
	s.samples[name] = &synth.Sample{Data: d, Rate: rate}
	return nil
}

// RemoveSample forgets the sample called name.
func (s *Session) RemoveSample(name string) { delete(s.samples, name) }

// Samples returns the names of the added samples, sorted.
func (s *Session) Samples() []string { return slices.Sorted(maps.Keys(s.samples)) }

// load is the session's synth.SampleLoader.
func (s *Session) load(name string) (*synth.Sample, error) {
	if smp, ok := s.samples[name]; ok {
		return smp, nil
	}
	return nil, fmt.Errorf("no sample file %q added", name)
}

// MissingSamples returns a problem at the "sample" field of every
// instrument of the voice document that names a sample the session does
// not hold. It reads the document itself, not the loaded voice, so an
// instrument that inherits the vowel's sound is not reported twice.
func (s *Session) MissingSamples(text string) []Problem {
	var doc struct {
		Instruments map[string]struct{ Sample string } `json:"instruments"`
		Percussion  map[string]struct{ Sample string } `json:"percussion"`
	}
	if json.Unmarshal([]byte(text), &doc) != nil {
		return nil // LoadVoice reports the syntax error
	}
	var out []Problem
	check := func(section string, m map[string]struct{ Sample string }) {
		for _, name := range slices.Sorted(maps.Keys(m)) {
			if smp := m[name].Sample; smp != "" && s.samples[smp] == nil {
				out = append(out, Problem{
					Path:    section + "." + name + ".sample",
					Message: fmt.Sprintf("no sample file %q: drop the WAV file on the page to add it", smp),
				})
			}
		}
	}
	check("instruments", doc.Instruments)
	check("percussion", doc.Percussion)
	return out
}
