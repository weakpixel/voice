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
	DefaultVoices = 32
	// BlockFrames is the render block. Sink calls are applied at the start
	// of a block, and glide and vibrato are updated once per block.
	BlockFrames = 64
	// DefaultPending is the number of commands a stream queues between two
	// blocks before it compacts the queue (see [StreamConfig.Pending]).
	DefaultPending = 1024
)

// StreamConfig configures a [Stream].
type StreamConfig struct {
	SampleRate int    // MinSampleRate to MaxSampleRate, required
	Format     Format // byte format of Read; FormatFloat32 by default
	Voices     int    // size of the voice pool; 0 means DefaultVoices
	// Pending is how many commands (Sink and Set calls) the stream queues
	// before the next block; 0 means DefaultPending. A full queue is
	// compacted (see [Stream.DropNotes]), so a stream nobody reads keeps a
	// bounded queue.
	Pending int
}

// Stream is the synthesizer: an endless PCM source for an audio player and a
// voice.Sink for the notes.
//
// Threading: Read and Render belong to one goroutine at a time (the audio
// player's); they own the voices. NoteOn, NoteOff, Release, SetPatch and
// SetGain may be called from any goroutine: they append a command to a
// mutex-guarded buffer that the renderer swaps for its own at the start of
// every block of BlockFrames frames. The buffer holds StreamConfig.Pending
// commands. When it is full (nobody reads the stream, for example before a
// browser allows audio), it is compacted like [Stream.DropNotes] does:
// queued notes are dropped, settings and releases are kept, and the
// buffer does not grow.
type Stream struct {
	rate       int
	format     Format
	frameBytes int

	mu      sync.Mutex
	pending []command   // guarded by mu, filled by the Sink methods
	marks   []ownerMark // guarded by mu, scratch of compact

	// Owned by the render goroutine.
	active  []command
	pool    pool
	patches map[voice.Owner]*[voice.InstrumentCount]*Patch
	gain    float32
	left    [BlockFrames]float32
	right   [BlockFrames]float32
	bus     [BlockFrames]float32 // reverb sends of the block
	reverb  *reverb
	pos     int // next unread frame of left/right; BlockFrames when used up
}

type cmdKind uint8

const (
	cmdNoteOn cmdKind = iota
	cmdNoteOff
	cmdRelease
	cmdPatch
	cmdGain
	cmdReverb
)

type command struct {
	kind     cmdKind
	owner    voice.Owner
	inst     voice.Instrument
	note     float32
	velocity float32 // also the gain of cmdGain
	pan      float32
	patch    *Patch
	reverb   Reverb
}

// ownerMark is what compact has seen of one owner, walking the queue
// from its end.
type ownerMark struct {
	owner   voice.Owner
	release bool   // a Release or a dropped NoteOff: the owner gets one Release
	patched uint16 // bit i: the last patch of instrument i is already kept
}

// The patched bits of ownerMark hold one bit per instrument.
const _ = uint16(1 << (voice.InstrumentCount - 1))

// compactMarks is how many owners compact handles without allocating.
const compactMarks = 32

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
	pending := cfg.Pending
	if pending == 0 {
		pending = DefaultPending
	}
	if pending < 0 {
		errs = append(errs, fmt.Errorf("synth: negative pending count %d", cfg.Pending))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return &Stream{
		rate:       cfg.SampleRate,
		format:     cfg.Format,
		frameBytes: frameBytes,
		pending:    make([]command, 0, pending),
		active:     make([]command, 0, pending),
		marks:      make([]ownerMark, 0, compactMarks),
		pool:       newPool(voices, cfg.SampleRate),
		patches:    make(map[voice.Owner]*[voice.InstrumentCount]*Patch),
		gain:       1,
		reverb:     newReverb(cfg.SampleRate),
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
	cp.Layers = append([]float32(nil), p.Layers...) // the caller may reuse its slice
	s.push(command{kind: cmdPatch, owner: owner, inst: inst, patch: cp})
	return nil
}

// SetGain sets the master gain (1 by default) applied before the output is
// clipped to -1 to 1. Safe from any goroutine.
func (s *Stream) SetGain(gain float32) {
	s.push(command{kind: cmdGain, velocity: gain})
}

// SetReverb sets the stream's reverb (DefaultReverb at first). Patches
// send into it with Patch.Reverb. It returns an error, and changes
// nothing, for a field outside 0 to 1. Safe from any goroutine.
func (s *Stream) SetReverb(r Reverb) error {
	if err := r.validate(); err != nil {
		return err
	}
	s.push(command{kind: cmdReverb, reverb: r})
	return nil
}

// NoteOn implements voice.Sink. A note for an instrument without a patch is
// ignored. Velocity is clamped to 0 to 1, pan to -1 to 1.
func (s *Stream) NoteOn(owner voice.Owner, inst voice.Instrument, note, velocity, pan float32) {
	s.push(command{kind: cmdNoteOn, owner: owner, inst: inst, note: note, velocity: velocity, pan: pan})
}

// NoteOff implements voice.Sink.
func (s *Stream) NoteOff(owner voice.Owner, inst voice.Instrument, note float32) {
	s.push(command{kind: cmdNoteOff, owner: owner, inst: inst, note: note})
}

// Release implements voice.Sink.
func (s *Stream) Release(owner voice.Owner) {
	s.push(command{kind: cmdRelease, owner: owner})
}

// DropNotes drops the notes queued since the last block, for a caller
// that knows nobody hears the stream: call it while the audio player does
// not read (before a browser allows audio, while a window is paused), and
// reading starts with the notes sent from then on instead of a burst of
// old ones. It keeps every Release, the last patch of each owner and
// instrument, the last gain and the last reverb. Each owner whose NoteOff
// it drops gets a Release instead, so a note that sounded before the
// queue filled up still ends (and with it the owner's other notes). The
// stream does the same by itself when its queue is full. Safe from any
// goroutine; allocates nothing for up to 32 owners in the queue.
func (s *Stream) DropNotes() {
	s.mu.Lock()
	s.compact()
	s.mu.Unlock()
}

func (s *Stream) push(c command) {
	s.mu.Lock()
	// Compact at the capacity instead of growing. When compaction keeps
	// everything (more owners with settings than the capacity), append
	// grows the slice and the next compaction waits for the new capacity,
	// so compaction never runs on every push.
	if len(s.pending) == cap(s.pending) {
		s.compact()
	}
	s.pending = append(s.pending, c)
	s.mu.Unlock()
}

// compact drops the queued notes in place (see DropNotes). The caller
// holds mu.
//
// It walks the queue from its end, so the first patch, gain and reverb it
// meets are the last ones sent, and moves what it keeps to the end of the
// slice in the original order. The Releases then go to the front: they act
// on voices only, settings on the patch table, gain and reverb only, so
// putting them first changes nothing but makes sure they come before any
// note queued after the compaction. There are at most as many Releases as
// dropped NoteOffs and Releases, so the front never overruns the kept part.
func (s *Stream) compact() {
	q := s.pending
	s.marks = s.marks[:0]
	keep := len(q) // q[keep:] is kept, in order
	seenGain, seenReverb := false, false
	for i := len(q) - 1; i >= 0; i-- {
		c := &q[i]
		kept := false
		switch c.kind {
		case cmdNoteOff, cmdRelease:
			s.mark(c.owner).release = true
		case cmdPatch:
			m, bit := s.mark(c.owner), uint16(1)<<c.inst
			if m.patched&bit == 0 {
				m.patched |= bit
				kept = true
			}
		case cmdGain:
			kept, seenGain = !seenGain, true
		case cmdReverb:
			kept, seenReverb = !seenReverb, true
		}
		if kept {
			keep--
			q[keep] = *c
		}
	}
	n := 0
	for _, m := range s.marks {
		if m.release {
			n++
		}
	}
	end := n + copy(q[n:], q[keep:])
	n = 0
	for _, m := range s.marks {
		if m.release {
			q[n] = command{kind: cmdRelease, owner: m.owner}
			n++
		}
	}
	clear(q[end:]) // drop patch pointers
	s.pending = q[:end]
}

// mark returns the mark of owner, adding one.
func (s *Stream) mark(owner voice.Owner) *ownerMark {
	for i := range s.marks {
		if s.marks[i].owner == owner {
			return &s.marks[i]
		}
	}
	s.marks = append(s.marks, ownerMark{owner: owner}) // allocates beyond compactMarks owners
	return &s.marks[len(s.marks)-1]
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
	clear(s.bus[:])
	s.pool.render(s.left[:], s.right[:], s.bus[:])
	s.reverb.process(s.bus[:], s.left[:], s.right[:])
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
		s.pool.noteOn(noteOn{owner: c.owner, inst: c.inst, patch: tbl[c.inst], note: c.note, velocity: c.velocity, pan: c.pan})
	case cmdNoteOff:
		s.pool.noteOff(c.owner, c.inst, c.note)
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
	case cmdReverb:
		s.reverb.set(c.reverb)
	}
}

var _ voice.Sink = (*Stream)(nil)
