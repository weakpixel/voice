package voicetest

import "testing"

func TestRecordingSink(t *testing.T) {
	s := NewRecordingSink(4)
	s.NoteOn(1, 0, 60, 1, 0)
	s.NoteOn(1, 0, 62, 1, 0)
	s.NoteOn(1, 0, 60, 1, 0)
	s.NoteOn(2, 0, 60, 1, 0)
	s.NoteOff(1, 0, 60)
	if got := s.Sounding(1); len(got) != 1 || got[0].Note != 62 {
		t.Errorf("owner 1 sounding %+v, want 62", got)
	}
	s.Release(2)
	if got := s.Sounding(2); len(got) != 0 {
		t.Errorf("owner 2 sounding %+v after Release", got)
	}
	if n := len(s.Calls()); n != 6 {
		t.Errorf("%d calls", n)
	}
	s.Reset()
	if len(s.Calls()) != 0 || len(s.Sounding(1)) != 0 {
		t.Error("Reset kept calls")
	}
}
