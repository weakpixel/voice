// Package ebiten plays a synth.Stream of the voice library on an Ebiten
// audio context. It is a nested module, so programs that do not use Ebiten
// never depend on it.
package ebiten

import (
	"errors"
	"fmt"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/weakpixel/voice/synth"
)

// DefaultBufferSize is a player buffer that keeps the typewriter and the
// sound in step (notes start within about this much of their Sink call)
// without underruns on desktop or in browsers.
const DefaultBufferSize = 50 * time.Millisecond

// Player plays a synth.Stream on an Ebiten audio context. The stream must
// be in FormatFloat32 at the context's sample rate.
//
// The Ebiten player owns the stream's Read from its own goroutine; keep
// calling only the stream's Sink methods (through a voice.Speaker) from the
// game. Browsers start audio only after a user gesture; Ebiten resumes the
// context on the first input, so sound starts then. Keep the Player
// referenced while it plays: the audio library closes a player that is
// garbage collected.
type Player struct {
	p *audio.Player
}

// NewPlayer starts playing s on ctx with the given buffer size (0 means
// DefaultBufferSize). It returns an error, and starts nothing, when the
// stream's format or sample rate does not fit the context.
func NewPlayer(ctx *audio.Context, s *synth.Stream, bufferSize time.Duration) (*Player, error) {
	if ctx == nil || s == nil {
		return nil, errors.New("ebiten: NewPlayer needs a context and a stream")
	}
	var errs []error
	if s.Format() != synth.FormatFloat32 {
		errs = append(errs, errors.New("ebiten: the stream must use synth.FormatFloat32"))
	}
	if s.SampleRate() != ctx.SampleRate() {
		errs = append(errs, fmt.Errorf("ebiten: the stream runs at %d Hz, the audio context at %d Hz", s.SampleRate(), ctx.SampleRate()))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	p, err := ctx.NewPlayerF32(s)
	if err != nil {
		return nil, fmt.Errorf("ebiten: %w", err)
	}
	if bufferSize <= 0 {
		bufferSize = DefaultBufferSize
	}
	p.SetBufferSize(bufferSize)
	p.Play()
	return &Player{p: p}, nil
}

// SetVolume sets the playback volume, 0 to 1.
func (p *Player) SetVolume(v float64) { p.p.SetVolume(v) }

// Volume returns the playback volume.
func (p *Player) Volume() float64 { return p.p.Volume() }

// Close stops playback and releases the Ebiten player. The stream is left
// as it is.
func (p *Player) Close() error { return p.p.Close() }
