package synth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/weakpixel/voice"
)

// Format is the byte format [Stream.Read] produces.
type Format uint8

const (
	// FormatFloat32 is interleaved stereo float32 little endian, 8 bytes a
	// frame (what Ebiten's NewPlayerF32 reads).
	FormatFloat32 Format = iota
	// FormatInt16 is interleaved stereo int16 little endian, 4 bytes a frame.
	FormatInt16
)

// The limits of [StreamConfig].
const (
	MinSampleRate = 16000
	MaxSampleRate = 192000
	DefaultVoices = 24
	// BlockFrames is the render block. Sink calls are applied at the start
	// of a block, and glide and vibrato are updated once per block.
	BlockFrames = 64
)

// StreamConfig configures a [Stream].
type StreamConfig struct {
	SampleRate int    // MinSampleRate to MaxSampleRate, required
	Format     Format // byte format of Read; FormatFloat32 by default
	Voices     int    // size of the voice pool; 0 means DefaultVoices
}

// Stream is the synthesizer: an endless PCM source for an audio player and a
// voice.Sink for the notes.
//
// Threading: Read and Render belong to one goroutine at a time (the audio
// player's); they own the voices. NoteOn, NoteOff, Release, SetPatch and
// SetGain may be called from any goroutine: they append a command to a
// mutex-guarded buffer that the renderer swaps for its own at the start of
// every block of BlockFrames frames. Nothing is dropped, and once the
// buffers have grown to the busiest block they no longer allocate.
type Stream struct {
	rate       int
	format     Format
	frameBytes int

	mu      sync.Mutex
	pending []command // guarded by mu, filled by the Sink methods

	// Owned by the render goroutine.
	active  []command
	pool    pool
	patches map[voice.Owner]*[voice.InstrumentCount]*Patch
	gain    float32
	left    [BlockFrames]float32
	right   [BlockFrames]float32
	pos     int // next unread frame of left/right; BlockFrames when used up
}

type cmdKind uint8

const (
	cmdNoteOn cmdKind = iota
	cmdNoteOff
	cmdRelease
	cmdPatch
	cmdGain
)

type command struct {
	kind     cmdKind
	owner    voice.Owner
	inst     voice.Instrument
	note     float32
	velocity float32 // also the gain of cmdGain
	pan      float32
	patch    *Patch
}

// NewStream returns a silent stream with no patches. It reports every
// problem of cfg together.
func NewStream(cfg StreamConfig) (*Stream, error) {
	var errs []error
	if cfg.SampleRate < MinSampleRate || cfg.SampleRate > MaxSampleRate {
		errs = append(errs, fmt.Errorf("synth: sample rate %d is outside %d to %d", cfg.SampleRate, MinSampleRate, MaxSampleRate))
	}
	frameBytes := 0
	switch cfg.Format {
	case FormatFloat32:
		frameBytes = 8
	case FormatInt16:
		frameBytes = 4
	default:
		errs = append(errs, fmt.Errorf("synth: unknown format %d", cfg.Format))
	}
	voices := cfg.Voices
	if voices == 0 {
		voices = DefaultVoices
	}
	if voices < 0 {
		errs = append(errs, fmt.Errorf("synth: negative voice count %d", cfg.Voices))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return &Stream{
		rate:       cfg.SampleRate,
		format:     cfg.Format,
		frameBytes: frameBytes,
		pending:    make([]command, 0, 256),
		active:     make([]command, 0, 256),
		pool:       newPool(voices, cfg.SampleRate),
		patches:    make(map[voice.Owner]*[voice.InstrumentCount]*Patch),
		gain:       1,
		pos:        BlockFrames,
	}, nil
}

// SampleRate returns the stream's sample rate in hertz.
func (s *Stream) SampleRate() int { return s.rate }

// Format returns the byte format of Read.
func (s *Stream) Format() Format { return s.format }

// SetPatch sets the patch that instrument inst of owner plays. Notes already
// sounding keep their old patch. It returns the patch's validation errors
// (and sets nothing) when p is invalid, or an error for an out-of-range
// inst. The patch is copied; its Sample is shared and must not change.
// Safe from any goroutine; allocates.
func (s *Stream) SetPatch(owner voice.Owner, inst voice.Instrument, p Patch) error {
	if int(inst) >= voice.InstrumentCount {
		return fmt.Errorf("synth: unknown instrument %d", inst)
	}
	if err := p.Validate(); err != nil {
		return err
	}
	cp := new(Patch)
	*cp = p
	s.push(command{kind: cmdPatch, owner: owner, inst: inst, patch: cp})
	return nil
}

// SetGain sets the master gain (1 by default) applied before the output is
// clipped to -1 to 1. Safe from any goroutine.
func (s *Stream) SetGain(gain float32) {
	s.push(command{kind: cmdGain, velocity: gain})
}

// NoteOn implements voice.Sink. A note for an instrument without a patch is
// ignored. Velocity is clamped to 0 to 1, pan to -1 to 1.
func (s *Stream) NoteOn(owner voice.Owner, inst voice.Instrument, note, velocity, pan float32) {
	s.push(command{kind: cmdNoteOn, owner: owner, inst: inst, note: note, velocity: velocity, pan: pan})
}

// NoteOff implements voice.Sink.
func (s *Stream) NoteOff(owner voice.Owner, note float32) {
	s.push(command{kind: cmdNoteOff, owner: owner, note: note})
}

// Release implements voice.Sink.
func (s *Stream) Release(owner voice.Owner) {
	s.push(command{kind: cmdRelease, owner: owner})
}

func (s *Stream) push(c command) {
	s.mu.Lock()
	s.pending = append(s.pending, c)
	s.mu.Unlock()
}

// Read fills p with whole frames in the stream's format and returns the
// number of bytes written; it never ends and never returns an error. A p
// shorter than one frame reads nothing. Read and Render must not run
// concurrently. Steady state allocates nothing.
func (s *Stream) Read(p []byte) (int, error) {
	frames := len(p) / s.frameBytes
	n := 0
	for range frames {
		if s.pos == BlockFrames {
			s.renderBlock()
		}
		l, r := s.left[s.pos], s.right[s.pos]
		s.pos++
		switch s.format {
		case FormatFloat32:
			binary.LittleEndian.PutUint32(p[n:], math.Float32bits(l))
			binary.LittleEndian.PutUint32(p[n+4:], math.Float32bits(r))
		case FormatInt16:
			binary.LittleEndian.PutUint16(p[n:], uint16(toInt16(l)))
			binary.LittleEndian.PutUint16(p[n+2:], uint16(toInt16(r)))
		}
		n += s.frameBytes
	}
	return n, nil
}

func toInt16(x float32) int16 {
	return int16(math.Round(float64(x) * 32767))
}

// Render fills left and right (which must have the same length) with the
// next frames, -1 to 1, for offline rendering. It continues where Read left
// off. Steady state allocates nothing.
func (s *Stream) Render(left, right []float32) {
	if len(left) != len(right) {
		panic("synth: Render: left and right differ in length")
	}
	for i := range left {
		if s.pos == BlockFrames {
			s.renderBlock()
		}
		left[i], right[i] = s.left[s.pos], s.right[s.pos]
		s.pos++
	}
}

// renderBlock applies the commands that arrived since the last block and
// renders the next BlockFrames frames into s.left and s.right.
func (s *Stream) renderBlock() {
	// Double buffer: swap the filled pending slice for the empty active one,
	// so the lock is held for two slice headers, not for the commands.
	s.mu.Lock()
	s.pending, s.active = s.active, s.pending
	s.mu.Unlock()
	for i := range s.active {
		s.apply(&s.active[i])
	}
	clear(s.active) // drop patch pointers
	s.active = s.active[:0]

	clear(s.left[:])
	clear(s.right[:])
	s.pool.render(s.left[:], s.right[:])
	for i := range s.left {
		s.left[i] = min(max(s.left[i]*s.gain, -1), 1)
		s.right[i] = min(max(s.right[i]*s.gain, -1), 1)
	}
	s.pos = 0
}

func (s *Stream) apply(c *command) {
	switch c.kind {
	case cmdNoteOn:
		tbl := s.patches[c.owner]
		if tbl == nil || int(c.inst) >= voice.InstrumentCount || tbl[c.inst] == nil {
			return
		}
		s.pool.noteOn(noteOn{owner: c.owner, patch: tbl[c.inst], note: c.note, velocity: c.velocity, pan: c.pan})
	case cmdNoteOff:
		s.pool.noteOff(c.owner, c.note)
	case cmdRelease:
		s.pool.release(c.owner)
	case cmdPatch:
		tbl := s.patches[c.owner]
		if tbl == nil {
			tbl = new([voice.InstrumentCount]*Patch)
			s.patches[c.owner] = tbl
		}
		tbl[c.inst] = c.patch
	case cmdGain:
		s.gain = c.velocity
	}
}

var _ voice.Sink = (*Stream)(nil)
