//go:build js && wasm

package main

import (
	"io"
	"sync/atomic"
	"time"

	"github.com/ebitengine/oto/v3"
)

// bufferSize is oto's device buffer and the player's read-ahead. The stream
// applies Sink calls at the next block it renders, so the read-ahead is how
// far a note can start late; 50 ms is what the Ebiten player and the say
// command use (with oto's default of 0.5 s, the blips of half a second
// would start at once).
const bufferSize = 50 * time.Millisecond

// audio plays the session's stream through oto, the library under Ebiten's
// audio. Ebiten's own audio context opens the device only from a running
// game loop; the editor has none. oto creates the AudioContext at once and
// resumes it on the page's first click, key or touch (browsers start audio
// only after a gesture), then the player starts pulling samples.
//
// audio holds on to the context and the player: oto closes a player that
// is no longer referenced (a runtime cleanup), so with a local variable the
// sound stopped at some garbage collection, while the page went on playing.
type audio struct {
	ready  atomic.Bool
	err    atomic.Value // string
	ctx    *oto.Context
	player *oto.Player
}

func startAudio(r io.Reader, rate int) *audio {
	a := &audio{}
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   rate,
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE, // synth.FormatFloat32: interleaved little-endian stereo
		BufferSize:   bufferSize,
	})
	if err != nil {
		a.err.Store(err.Error())
		return a
	}
	a.ctx = ctx
	go func() {
		<-ready
		a.player = ctx.NewPlayer(r)
		a.player.SetBufferSize(int(int64(rate)*int64(bufferSize)/int64(time.Second)) * 8)
		a.player.Play()
		a.ready.Store(true)
	}()
	return a
}

// isReady reports whether the device runs and the player plays.
func (a *audio) isReady() bool { return a.ready.Load() }

// errText is why audio could not start, "" if it did or still may.
func (a *audio) errText() string {
	s, _ := a.err.Load().(string)
	return s
}
