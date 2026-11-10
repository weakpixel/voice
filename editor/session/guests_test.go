package session

import (
	"strings"
	"testing"
	"time"
)

func TestSayWithGuests(t *testing.T) {
	s, err := New(rate)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetVoice(preset(t, "soft")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetGuest("gruff", preset(t, "gruff")); err != nil {
		t.Fatal(err)
	}
	if err := s.Say("He said {gruff}stay back{/} and {nobody}left{/}.", Options{}); err != nil {
		t.Fatal(err)
	}
	if s.Text() != "He said stay back and left." {
		t.Errorf("text %q", s.Text())
	}
	if p := s.Problems(); len(p) != 1 || !strings.Contains(p[0].Message, "{nobody}") {
		t.Errorf("problems %v", p)
	}
	d := Data(s.Voice(), s.Timeline())
	spanned := 0
	for _, n := range d.Notes {
		if n.Span == 1 {
			spanned++
		}
	}
	if spanned == 0 {
		t.Error("no note of the gruff span")
	}
	for !s.State().Done {
		s.Update(10 * time.Millisecond)
	}

	// The offline render plays the guest too, and differs from the plain line.
	r, err := s.StartRender("He said {gruff}stay back{/}.", Options{})
	if err != nil {
		t.Fatal(err)
	}
	for !r.Step(1 << 20) {
	}
	l1, _, tl := r.Result()
	plain, _, _, _ := RenderLine(s.Voice(), nil, "He said stay back.", Options{}, rate)
	if tl.TextLen != len("He said stay back.") || len(l1) == 0 || len(l1) == len(plain) && l1[len(l1)/3] == plain[len(plain)/3] {
		t.Errorf("render with a span: %d frames, text %d", len(l1), tl.TextLen)
	}

	s.ClearGuests()
	_ = s.Say("{gruff}x{/}", Options{})
	if len(s.Problems()) != 1 {
		t.Errorf("after ClearGuests: %v", s.Problems())
	}
	if err := s.Say("plain", Options{}); err != nil || s.Problems() != nil {
		t.Errorf("plain line: %v %v", err, s.Problems())
	}
}
