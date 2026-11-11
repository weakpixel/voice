package synth

import (
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/voice"
)

func TestPendingErrors(t *testing.T) {
	_, err := NewStream(StreamConfig{SampleRate: testRate, Pending: -1})
	if err == nil || !strings.Contains(err.Error(), "negative pending count -1") {
		t.Errorf("err = %v, want negative pending count", err)
	}
}

// pushNotes queues n note pairs for owners 1 to 3, as speakers do while
// nobody reads.
func pushNotes(s *Stream, n int) {
	for i := range n {
		owner := voice.Owner(1 + i%3)
		s.NoteOn(owner, voice.InstVowel, float32(50+i%20), 1, 0)
		s.NoteOff(owner, voice.InstVowel, float32(50+i%20))
	}
}

func TestPendingBounded(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pending int
		want    int
	}{
		{"default", 0, DefaultPending},
		{"small", 64, 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewStream(StreamConfig{SampleRate: testRate, Pending: tc.pending})
			if err != nil {
				t.Fatal(err)
			}
			setPatch(t, s, 1, voice.InstVowel, heldPatch)
			pushNotes(s, 50_000)
			if c := cap(s.pending); c != tc.want {
				t.Errorf("queue capacity %d after 100 000 commands, want %d", c, tc.want)
			}
			if n := testing.AllocsPerRun(20, func() { pushNotes(s, 1000) }); n != 0 {
				t.Errorf("pushes across the capacity: %g allocs per run, want 0", n)
			}
		})
	}
}

func TestCompactionKeepsConfig(t *testing.T) {
	// a gets the settings and is read; b gets the same settings mixed into
	// more notes than its queue holds. Afterwards both render a note the
	// same way: same patches, gain and reverb.
	config := func(s *Stream) {
		setPatch(t, s, 1, voice.InstVowel, Patch{Wave: Square, Env: Envelope{Sustain: 1}})
		setPatch(t, s, 1, voice.InstVowel, Patch{Wave: Saw, Reverb: 0.5, Env: Envelope{Sustain: 1}}) // the last wins
		s.SetGain(0.3)
		if err := s.SetReverb(Reverb{Size: 0.2, Damping: 0.9, Width: 0.1}); err != nil {
			t.Fatal(err)
		}
	}
	late := func(s *Stream) {
		setPatch(t, s, 2, voice.InstMotif, Patch{Wave: Triangle, Env: Envelope{Sustain: 1}})
		s.SetGain(0.6)
	}
	a, b := newTestStream(t, 8), newTestStream(t, 8)
	config(a)
	late(a)
	// Owner 9 has no patches: its notes are silent whether dropped or not.
	notes := func() {
		for i := range DefaultPending {
			b.NoteOn(9, voice.InstVowel, float32(50+i%20), 1, 0)
			b.NoteOff(9, voice.InstVowel, float32(50+i%20))
		}
	}
	config(b)
	notes()
	late(b)
	notes()
	if len(b.pending) >= 2*DefaultPending {
		t.Fatal("the queue never compacted")
	}
	render(a, time.Millisecond)
	render(b, time.Millisecond)
	for _, s := range []*Stream{a, b} {
		s.NoteOn(1, voice.InstVowel, 60, 1, 0)
		s.NoteOn(2, voice.InstMotif, 67, 1, 0)
	}
	x, y := render(a, 100*time.Millisecond), render(b, 100*time.Millisecond)
	if peak(x) == 0 {
		t.Fatal("reference is silent")
	}
	for i := range x {
		if x[i] != y[i] {
			t.Fatalf("frame %d differs: %g vs %g", i, x[i], y[i])
		}
	}
}

func TestCompactionReleasesHeldNotes(t *testing.T) {
	s := newTestStream(t, 8)
	setPatch(t, s, 1, voice.InstVowel, heldPatch)
	setPatch(t, s, 4, voice.InstVowel, heldPatch)
	s.NoteOn(1, voice.InstVowel, 69, 1, 0)
	s.NoteOn(4, voice.InstVowel, 72, 1, 0)
	render(s, 10*time.Millisecond)
	s.NoteOff(1, voice.InstVowel, 69) // lost in the compaction below
	s.Release(4)
	pushNotes(s, DefaultPending)
	render(s, 50*time.Millisecond)
	if n := s.pool.sounding(); n != 0 {
		t.Errorf("%d voices sounding after the stall", n)
	}
}

func TestCompactionManyOwners(t *testing.T) {
	s, err := NewStream(StreamConfig{SampleRate: testRate, Pending: 64})
	if err != nil {
		t.Fatal(err)
	}
	for owner := range voice.Owner(100) {
		setPatch(t, s, owner, voice.InstVowel, heldPatch)
		s.NoteOn(owner, voice.InstVowel, 60, 1, 0)
	}
	render(s, time.Millisecond)
	if n := len(s.patches); n != 100 {
		t.Errorf("%d owners with patches, want 100", n)
	}
}

// speak runs a speaker on s at 60 Hz for d; read says whether the stream
// is read meanwhile, drop whether DropNotes follows every update.
func speak(sp *voice.Speaker, s *Stream, d time.Duration, read, drop bool) {
	const dt = time.Second / 60
	l, r := make([]float32, frames(dt)), make([]float32, frames(dt))
	for t := time.Duration(0); t < d; t += dt {
		sp.Update(dt)
		if drop {
			s.DropNotes()
		}
		if read {
			s.Render(l, r)
		}
	}
}

func TestDropNotesNoBurst(t *testing.T) {
	v, err := voice.Preset("chirpy")
	if err != nil {
		t.Fatal(err)
	}
	const line = "Hello there, traveler! The road north is long, and the river runs cold."
	stalled := func(drop bool) (*Stream, *voice.Speaker) {
		s := newTestStream(t, DefaultVoices)
		if err := s.SetVoice(1, v, nil); err != nil {
			t.Fatal(err)
		}
		sp := voice.NewSpeaker(s, 1)
		sp.Say(v, line, voice.SayOptions{})
		speak(sp, s, 2*time.Second, false, drop)
		render(s, time.Millisecond) // reading starts
		return s, sp
	}
	burst, _ := stalled(false)
	s, sp := stalled(true)
	if n := burst.pool.sounding(); n < 8 {
		t.Fatalf("without DropNotes only %d voices start at once: the test shows no burst", n)
	}
	if n := s.pool.sounding(); n > 4 {
		t.Errorf("%d voices start at once after DropNotes, want at most the notes of one frame", n)
	}
	for !sp.Done() {
		speak(sp, s, time.Second/60, true, false)
	}
	render(s, time.Second)
	if n := s.pool.sounding(); n != 0 {
		t.Errorf("%d voices sounding after the line", n)
	}
	sp.Say(v, "And the next line.", voice.SayOptions{})
	speak(sp, s, 200*time.Millisecond, true, false)
	l := render(s, 300*time.Millisecond)
	if peak(l) == 0 {
		t.Error("the next line is silent")
	}
}
