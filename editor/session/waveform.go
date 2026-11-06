package session

import (
	"bytes"

	"github.com/weakpixel/voice/wav"
)

// StartRender starts an offline render of text with the session's voice
// and language (see LineRender), for the piano roll, the waveform and the
// WAV download. It does not touch the live stream.
func (s *Session) StartRender(text string, o Options) (*LineRender, error) {
	return NewLineRender(s.v, s.lang, text, o, s.SampleRate())
}

// Peaks reduces a stereo render to n columns for a waveform: for each
// column the lowest and the highest sample of the two channels mixed to
// mono, as min0, max0, min1, max1, ... An empty render gives n flat columns.
func Peaks(left, right []float32, n int) []float32 {
	out := make([]float32, 2*max(n, 0))
	if n <= 0 || len(left) == 0 {
		return out
	}
	for c := range n {
		from, to := c*len(left)/n, (c+1)*len(left)/n
		lo, hi := float32(0), float32(0)
		for i := from; i < max(to, from+1) && i < len(left); i++ {
			m := (left[i] + right[i]) / 2
			lo, hi = min(lo, m), max(hi, m)
		}
		out[2*c], out[2*c+1] = lo, hi
	}
	return out
}

// WAV encodes a stereo render as a float32 WAV file.
func WAV(left, right []float32, rate int) ([]byte, error) {
	var b bytes.Buffer
	if err := wav.WriteStereo(&b, rate, left, right); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
