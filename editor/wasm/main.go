//go:build js && wasm

// Command wasm is the Voice Editor's Go side in the browser: it puts a
// voiceEditor object on the page's global scope that loads documents,
// plays test lines through the session and reports what to show. The
// page (editor/web) calls it; all logic lives in package session.
package main

import (
	"encoding/json"
	"syscall/js"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/editor/session"
)

// sampleRate is the stream's and the AudioContext's rate; browsers resample
// to the device.
const sampleRate = 48000

type bridge struct {
	s     *session.Session
	audio *audio
	// loaded is whether the last voice document loaded; lines need one.
	loaded bool
}

func main() {
	s, err := session.New(sampleRate)
	if err != nil {
		panic(err) // the rate is a constant: cannot happen
	}
	b := &bridge{s: s}
	b.audio = startAudio(s.Reader(), sampleRate)
	api := map[string]any{
		"loadVoice":    js.FuncOf(b.loadVoice),
		"loadLanguage": js.FuncOf(b.loadLanguage),
		"say":          js.FuncOf(b.say),
		"update":       js.FuncOf(b.update),
		"skip":         js.FuncOf(func(js.Value, []js.Value) any { s.Skip(); return nil }),
		"stop":         js.FuncOf(func(js.Value, []js.Value) any { s.Stop(); return nil }),
		"pause":        js.FuncOf(func(js.Value, []js.Value) any { s.Pause(); return nil }),
		"resume":       js.FuncOf(func(js.Value, []js.Value) any { s.Resume(); return nil }),
		"setVolume":    js.FuncOf(func(_ js.Value, a []js.Value) any { s.SetVolume(float32(a[0].Float())); return nil }),
		"peak":         js.FuncOf(func(js.Value, []js.Value) any { return float64(s.Peak()) }),
		"audioReady":   js.FuncOf(func(js.Value, []js.Value) any { return b.audio.isReady() }),
		"audioError":   js.FuncOf(func(js.Value, []js.Value) any { return b.audio.errText() }),
	}
	js.Global().Set("voiceEditor", js.ValueOf(api))
	// Tell the page the API is there (it waits for this event).
	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("voiceeditorready"))
	select {}
}

// toJS turns a Go value into a JS value through JSON, which keeps the
// bridge free of hand-built JS objects. Not for the per-frame call.
func toJS(v any) js.Value {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err) // only plain data is passed
	}
	return js.Global().Get("JSON").Call("parse", string(data))
}

// loadVoice(text) loads a voice document and, if it loads, makes it the
// session's voice. Returns {ok, name, problems}.
func (b *bridge) loadVoice(_ js.Value, a []js.Value) any {
	type result struct {
		OK       bool              `json:"ok"`
		Name     string            `json:"name"`
		Rhythm   bool              `json:"rhythm"`
		Problems []session.Problem `json:"problems"`
	}
	v, problems := session.LoadVoice(a[0].String())
	if v != nil {
		if err := b.s.SetVoice(v); err != nil {
			v, problems = nil, []session.Problem{{Message: err.Error()}}
		}
	}
	b.loaded = v != nil
	r := result{OK: v != nil, Problems: problems}
	if v != nil {
		r.Name, r.Rhythm = v.Name, v.Rhythm != nil
	}
	return toJS(r)
}

// loadLanguage(text) loads a language document ("" is the default) and, if
// it loads, uses it for the next lines. Returns {ok, problems}.
func (b *bridge) loadLanguage(_ js.Value, a []js.Value) any {
	l, problems := session.LoadLanguage(a[0].String())
	if l != nil {
		b.s.SetLanguage(l)
	}
	return toJS(map[string]any{"ok": l != nil, "problems": problems})
}

// sayOptions is the JSON form of session.Options from the page.
type sayOptions struct {
	Speed    float32  `json:"speed"`
	Instant  bool     `json:"instant"`
	BPM      float32  `json:"bpm"`   // with Phase: a clock, 0 means none
	Phase    float64  `json:"phase"` // seconds until the music's next bar
	MuteBeat bool     `json:"muteBeat"`
}

// say(text, options) starts a line with the current voice and returns its
// timeline (session.TimelineData), or null when no voice is loaded.
func (b *bridge) say(_ js.Value, a []js.Value) any {
	if !b.loaded {
		return js.Null()
	}
	var o sayOptions
	if len(a) > 1 && a[1].Truthy() {
		_ = json.Unmarshal([]byte(js.Global().Get("JSON").Call("stringify", a[1]).String()), &o)
	}
	opts := session.Options{Speed: o.Speed, Instant: o.Instant, MuteBeat: o.MuteBeat}
	if o.BPM > 0 {
		opts.Clock = &voice.Clock{BPM: o.BPM, Phase: time.Duration(o.Phase * float64(time.Second))}
	}
	if err := b.s.Say(a[0].String(), opts); err != nil {
		return js.Null()
	}
	return toJS(session.Data(b.s.Voice(), b.s.Timeline()))
}

// update(dtMs) advances the line and returns {revealed, done, paused, time,
// events}; time in seconds, events as names ("started", "completed", ...).
func (b *bridge) update(_ js.Value, a []js.Value) any {
	b.s.Update(time.Duration(a[0].Float() * float64(time.Millisecond)))
	st := b.s.State()
	events := make([]any, len(st.Events))
	for i, e := range st.Events {
		events[i] = e.Kind.String()
	}
	return js.ValueOf(map[string]any{
		"revealed": st.Revealed,
		"done":     st.Done,
		"paused":   st.Paused,
		"time":     st.Time.Seconds(),
		"events":   events,
	})
}
