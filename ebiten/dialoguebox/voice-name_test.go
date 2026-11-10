package dialoguebox

import (
	"os"
	"testing"
	"time"

	"github.com/weakpixel/voice/dialogue"
	"github.com/weakpixel/voice/synth"
)

// The top bar names the span's voice while the span is spoken.
func TestVoiceNameOfSpan(t *testing.T) {
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
	line := firstSpanLine(s)
	b := New(stream, Options{})
	if err := b.SetScript(s, voices, line); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 60 * 20 {
		if b.player.Phase() != dialogue.Speaking {
			break
		}
		seen[b.voiceName()] = true
		b.Step(time.Second / 60)
	}
	if !seen["verse"] || !seen["verse + chirpy"] {
		t.Errorf("voice names %v, want verse and verse + chirpy", seen)
	}
}

// firstSpanLine returns the index of the first line with a span.
func firstSpanLine(s *dialogue.Script) int {
	for i, l := range s.Lines {
		if len(l.Spans) > 0 {
			return i
		}
	}
	return 0
}
