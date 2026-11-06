package dialogue_test

import (
	"encoding/json"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/dialogue"
	"github.com/weakpixel/voice/voicetest"
)

var update = flag.Bool("update", false, "rewrite the golden events in testdata/")

const frame = time.Second / 60

func newPlayer(t testing.TB, sink voice.Sink, opts dialogue.Options) *dialogue.Player {
	t.Helper()
	s, voices := scene(t)
	p, err := dialogue.NewPlayer(sink, s, voices, opts)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// runUntil updates p until stop reports true, failing after 60 seconds.
func runUntil(t testing.TB, p *dialogue.Player, stop func() bool) {
	t.Helper()
	for range 60 * 60 {
		if stop() {
			return
		}
		p.Update(frame)
	}
	t.Fatalf("not reached; phase %v line %d", p.Phase(), p.Index())
}

func TestPlayerVoices(t *testing.T) {
	p := newPlayer(t, voicetest.NewRecordingSink(0), dialogue.Options{})
	vs := p.Voices()
	if len(vs) != 2 || vs[0].Owner != 1 || vs[1].Owner != 2 || vs[0].Speaker != "wren" || vs[1].Speaker != "bram" {
		t.Fatalf("voices %+v", vs)
	}
	if vs[0].Voice.Pan != -0.3 || vs[1].Voice.Pan != 0.3 {
		t.Errorf("pans %v %v", vs[0].Voice.Pan, vs[1].Voice.Pan)
	}
	// The speaker's pan is applied to a copy: the shared voice is not touched.
	s, voices := scene(t)
	before := voices["chirpy"].Pan
	if _, err := dialogue.NewPlayer(voicetest.NewRecordingSink(0), s, voices, dialogue.Options{}); err != nil {
		t.Fatal(err)
	}
	if voices["chirpy"].Pan != before {
		t.Error("the loaded voice was modified")
	}
}

func TestNewPlayerErrors(t *testing.T) {
	s, voices := scene(t)
	sink := voicetest.NewRecordingSink(0)
	if _, err := dialogue.NewPlayer(sink, s, map[string]*voice.Voice{"chirpy": voices["chirpy"]}, dialogue.Options{}); err == nil {
		t.Error("a missing voice was accepted")
	}
	for _, start := range []int{-1, len(s.Lines)} {
		if _, err := dialogue.NewPlayer(sink, s, voices, dialogue.Options{Start: start}); err == nil {
			t.Errorf("start %d accepted", start)
		}
	}
	p, err := dialogue.NewPlayer(sink, s, voices, dialogue.Options{Start: 2})
	if err != nil || p.Index() != 2 {
		t.Errorf("start 2: %v, index %d", err, p.Index())
	}
}

func TestPlayerPhasesWithInput(t *testing.T) {
	sink := voicetest.NewRecordingSink(0)
	p := newPlayer(t, sink, dialogue.Options{})
	if p.Phase() != dialogue.Speaking || p.Index() != 0 || p.Speaker().Name != "Wren" {
		t.Fatalf("start: %v %d %v", p.Phase(), p.Index(), p.Speaker())
	}
	if p.Revealed() == 0 {
		t.Error("nothing revealed at once")
	}
	runUntil(t, p, func() bool { return p.Phase() == dialogue.Holding })
	if p.Revealed() != len(p.Line().Text) {
		t.Error("a finished line is not fully shown")
	}
	// Input mode waits for Next, however long.
	for range 600 {
		p.Update(frame)
	}
	if p.Phase() != dialogue.Holding || p.Index() != 0 {
		t.Fatalf("input line moved on: %v %d", p.Phase(), p.Index())
	}
	p.Next()
	// Line 1 has a delay: Waiting, nothing shown, Next ignored.
	if p.Phase() != dialogue.Waiting || p.Index() != 1 || p.Revealed() != 0 {
		t.Fatalf("delay: %v %d %d", p.Phase(), p.Index(), p.Revealed())
	}
	p.Next()
	if p.Phase() != dialogue.Waiting {
		t.Error("Next during a delay did something")
	}
	runUntil(t, p, func() bool { return p.Phase() == dialogue.Speaking })
	// Next while speaking skips the rest of the line.
	p.Next()
	if p.Phase() != dialogue.Holding || p.Revealed() != len(p.Line().Text) {
		t.Fatalf("skip: %v %d", p.Phase(), p.Revealed())
	}
	assertSilent(t, sink, p)
}

func TestPlayerFinishes(t *testing.T) {
	sink := voicetest.NewRecordingSink(0)
	p := newPlayer(t, sink, dialogue.Options{Start: 3})
	runUntil(t, p, func() bool { return p.Phase() == dialogue.Holding })
	p.Next()
	if p.Phase() != dialogue.Finished || p.Index() != 3 {
		t.Fatalf("%v %d", p.Phase(), p.Index())
	}
	last := p.Events()[len(p.Events())-1]
	if last.Kind != dialogue.DialogueFinished || last.Line != 3 {
		t.Errorf("last event %+v", last)
	}
	p.Next()
	p.Update(frame)
	if p.Phase() != dialogue.Finished {
		t.Error("Next after the end did something")
	}
	p.Start(0) // plays again
	if p.Phase() != dialogue.Speaking || p.Index() != 0 {
		t.Errorf("restart: %v %d", p.Phase(), p.Index())
	}
}

func TestPlayerAutoAdvance(t *testing.T) {
	p := newPlayer(t, voicetest.NewRecordingSink(0), dialogue.Options{AutoAdvance: true})
	runUntil(t, p, func() bool { return p.Phase() == dialogue.Finished })
	if p.Index() != 3 {
		t.Errorf("ended on line %d", p.Index())
	}

	// The script's own auto line goes on without the option: start on it.
	p = newPlayer(t, voicetest.NewRecordingSink(0), dialogue.Options{Start: 2})
	runUntil(t, p, func() bool { return p.Index() == 3 })
}

func TestPlayerNextSkipsAHold(t *testing.T) {
	p := newPlayer(t, voicetest.NewRecordingSink(0), dialogue.Options{AutoAdvance: true})
	runUntil(t, p, func() bool { return p.Phase() == dialogue.Holding })
	p.Next()
	if p.Index() != 1 {
		t.Errorf("index %d", p.Index())
	}
}

func TestPlayerStartPauseStop(t *testing.T) {
	sink := voicetest.NewRecordingSink(0)
	p := newPlayer(t, sink, dialogue.Options{})
	for range 30 {
		p.Update(frame)
	}
	p.Start(1)
	assertSilent(t, sink, p) // line 0 was cut: its notes are released
	if p.Phase() != dialogue.Waiting {
		t.Fatalf("Start into a delay: %v", p.Phase())
	}
	p.Start(2)
	if p.Index() != 2 || p.Phase() != dialogue.Speaking {
		t.Fatalf("%v %d", p.Phase(), p.Index())
	}
	for range 30 {
		p.Update(frame)
	}
	shown := p.Revealed()
	p.Pause()
	assertSilent(t, sink, p)
	for range 120 {
		p.Update(frame)
	}
	if !p.Paused() || p.Revealed() != shown {
		t.Errorf("time moved while paused: %d, then %d", shown, p.Revealed())
	}
	p.Resume()
	runUntil(t, p, func() bool { return p.Phase() != dialogue.Speaking })
	p.Start(0)
	p.Update(frame)
	p.Stop()
	assertSilent(t, sink, p)
	if p.Phase() != dialogue.Finished {
		t.Errorf("after Stop: %v", p.Phase())
	}
	mustPanic(t, "line index", func() { p.Start(9) })
}

func TestPlayerClose(t *testing.T) {
	sink := voicetest.NewRecordingSink(0)
	p := newPlayer(t, sink, dialogue.Options{})
	p.Update(frame)
	p.Close()
	assertSilent(t, sink, p)
	n := len(sink.Calls())
	p.Start(0)
	p.Next()
	p.Update(frame)
	if len(sink.Calls()) != n || p.Phase() != dialogue.Finished {
		t.Error("a closed player did something")
	}
}

// Every note a line starts ends, whichever way the line was cut.
func TestPlayerReleasesNotes(t *testing.T) {
	cuts := map[string]func(*dialogue.Player){
		"next":  func(p *dialogue.Player) { p.Next() },
		"start": func(p *dialogue.Player) { p.Start(1) },
		"stop":  func(p *dialogue.Player) { p.Stop() },
		"close": func(p *dialogue.Player) { p.Close() },
		"pause": func(p *dialogue.Player) { p.Pause() },
	}
	for name, cut := range cuts {
		t.Run(name, func(t *testing.T) {
			sink := voicetest.NewRecordingSink(0)
			p := newPlayer(t, sink, dialogue.Options{})
			for range 20 {
				p.Update(frame)
			}
			if len(sink.Sounding(1)) == 0 {
				t.Fatal("nothing sounds before the cut")
			}
			cut(p)
			assertSilent(t, sink, p)
		})
	}
}

func assertSilent(t testing.TB, sink *voicetest.RecordingSink, p *dialogue.Player) {
	t.Helper()
	for _, v := range p.Voices() {
		if on := sink.Sounding(v.Owner); len(on) > 0 {
			t.Fatalf("owner %d (%s) still sounds %d notes", v.Owner, v.Speaker, len(on))
		}
	}
}

func TestPlayerLanguages(t *testing.T) {
	var asked []string
	p := newPlayer(t, voicetest.NewRecordingSink(0), dialogue.Options{Languages: func(name string) *voice.Language {
		asked = append(asked, name)
		return nil
	}})
	p.Update(frame)
	if len(asked) != 1 {
		t.Errorf("asked %v", asked)
	}
}

func TestPlayerUpdateAllocatesNothing(t *testing.T) {
	sink := voicetest.NewRecordingSink(1 << 20)
	p := newPlayer(t, sink, dialogue.Options{AutoAdvance: true})
	for range 10 {
		p.Update(frame)
	}
	// Steady state: a line speaking. (Starting a line allocates.)
	if p.Phase() != dialogue.Speaking {
		t.Fatal(p.Phase())
	}
	allocs := testing.AllocsPerRun(20, func() {
		p.Update(time.Millisecond)
	})
	if allocs != 0 {
		t.Errorf("Update allocates %v", allocs)
	}
}

// goldenEvent is the JSON form of an event in testdata/scene.events.json.
type goldenEvent struct {
	Kind string `json:"kind"`
	Line int    `json:"line"`
	AtMs int    `json:"atMs"`
}

func TestGoldenEvents(t *testing.T) {
	p := newPlayer(t, voicetest.NewRecordingSink(0), dialogue.Options{AutoAdvance: true})
	var got []goldenEvent
	collect := func() {
		for _, e := range p.Events() {
			got = append(got, goldenEvent{e.Kind.String(), e.Line, int(e.At.Round(time.Millisecond) / time.Millisecond)})
		}
	}
	collect()
	runUntil(t, p, func() bool { collect(); return p.Phase() == dialogue.Finished })
	collect()
	got = dedupe(got)
	const file = "testdata/scene.events.json"
	if *update {
		data, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(file, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var want []goldenEvent
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("events differ from %s (go test -update rewrites it):\n got %v\nwant %v", file, got, want)
	}
	// Same script, same events: the player is deterministic.
	p2 := newPlayer(t, voicetest.NewRecordingSink(0), dialogue.Options{AutoAdvance: true})
	var again []goldenEvent
	for _, e := range p2.Events() {
		again = append(again, goldenEvent{e.Kind.String(), e.Line, int(e.At.Round(time.Millisecond) / time.Millisecond)})
	}
	for p2.Phase() != dialogue.Finished {
		p2.Update(frame)
		for _, e := range p2.Events() {
			again = append(again, goldenEvent{e.Kind.String(), e.Line, int(e.At.Round(time.Millisecond) / time.Millisecond)})
		}
	}
	if !slices.Equal(dedupe(again), got) {
		t.Error("two runs differ")
	}
}

// dedupe drops repeats: Events keeps the events of a call until the next
// Update, so collecting after every step may see one twice.
func dedupe(evs []goldenEvent) []goldenEvent {
	return slices.CompactFunc(evs, func(a, b goldenEvent) bool { return a == b })
}

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if s, ok := r.(string); !ok || !strings.Contains(s, want) {
			t.Errorf("panic %v, want one containing %q", r, want)
		}
	}()
	f()
}
