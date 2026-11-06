package dialoguebox_test

import (
	"os"
	"testing"
	"time"

	"github.com/weakpixel/voice/dialogue"
	"github.com/weakpixel/voice/ebiten/dialoguebox"
	"github.com/weakpixel/voice/synth"
)

func newBox(t *testing.T, opts dialoguebox.Options) (*dialoguebox.Box, *dialogue.Script) {
	t.Helper()
	stream, err := synth.NewStream(synth.StreamConfig{SampleRate: 48000})
	if err != nil {
		t.Fatal(err)
	}
	fsys := os.DirFS("../example")
	s, err := dialogue.LoadFS(fsys, "village.dialogue.json")
	if err != nil {
		t.Fatal(err)
	}
	voices, err := dialogue.LoadVoices(fsys, s)
	if err != nil {
		t.Fatal(err)
	}
	b := dialoguebox.New(stream, opts)
	if err := b.SetScript(s, voices, 0); err != nil {
		t.Fatal(err)
	}
	return b, s
}

// The village dialogue plays hands-free from the first line to the end.
func TestBoxAutoAdvance(t *testing.T) {
	b, s := newBox(t, dialoguebox.Options{AutoAdvance: true})
	started := 0
	for range 60 * 120 {
		b.Step(time.Second / 60)
		for _, e := range b.Player().Events() {
			if e.Kind == dialogue.LineStarted {
				if e.Line != started {
					t.Fatalf("line %d started, want %d", e.Line, started)
				}
				started++
			}
		}
		if b.Player().Phase() == dialogue.Finished {
			break
		}
	}
	if b.Player().Phase() != dialogue.Finished || started != len(s.Lines) {
		t.Fatalf("phase %v, %d of %d lines started", b.Player().Phase(), started, len(s.Lines))
	}
}

func TestBoxSetScriptSwaps(t *testing.T) {
	b, s := newBox(t, dialoguebox.Options{})
	old := b.Player()
	b.Step(time.Second / 60)
	if err := b.SetScript(s, nil, 0); err == nil {
		t.Error("a missing voice was accepted")
	}
	if b.Player() != old {
		t.Error("a failed SetScript replaced the player")
	}
}

func TestWrap(t *testing.T) {
	if got := dialoguebox.Wrap("aa bb cc", 5); got != "aa bb\ncc" {
		t.Errorf("got %q", got)
	}
}
