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
	// render is the offline render in progress, nil if none.
	render *session.LineRender
	// left and right are the last finished render, for wav().
	left, right []float32
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
		"audition":     js.FuncOf(b.audition),
		"addSample":    js.FuncOf(b.addSample),
		"removeSample": js.FuncOf(func(_ js.Value, a []js.Value) any { s.RemoveSample(a[0].String()); return nil }),
		"samples":      js.FuncOf(func(js.Value, []js.Value) any { return toJS(s.Samples()) }),
		"renderStart":  js.FuncOf(b.renderStart),
		"renderStep":   js.FuncOf(b.renderStep),
		"wav":          js.FuncOf(b.wav),
		"skip":         js.FuncOf(func(js.Value, []js.Value) any { s.Skip(); return nil }),
		"stop":         js.FuncOf(func(js.Value, []js.Value) any { s.Stop(); return nil }),
		"pause":        js.FuncOf(func(js.Value, []js.Value) any { s.Pause(); return nil }),
		"resume":       js.FuncOf(func(js.Value, []js.Value) any { s.Resume(); return nil }),
		"setVolume":    js.FuncOf(func(_ js.Value, a []js.Value) any { s.SetVolume(float32(a[0].Float())); return nil }),
		"peak":         js.FuncOf(func(js.Value, []js.Value) any { return float64(s.Peak()) }),
		"framesRead":   js.FuncOf(func(js.Value, []js.Value) any { return float64(s.FramesRead()) }),
		"audioReady":   js.FuncOf(func(js.Value, []js.Value) any { return b.audio.isReady() }),
		"audioError":   js.FuncOf(func(js.Value, []js.Value) any { return b.audio.errText() }),
		"percussionDefaults": js.FuncOf(func(js.Value, []js.Value) any {
			return toJS(session.PercussionDefaults())
		}),
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
	if missing := b.s.MissingSamples(a[0].String()); v != nil && len(missing) > 0 {
		v, problems = nil, missing
	}
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
	Speed    float32 `json:"speed"`
	Instant  bool    `json:"instant"`
	BPM      float32 `json:"bpm"`   // with Phase: a clock, 0 means none
	Phase    float64 `json:"phase"` // seconds until the music's next bar
	MuteBeat bool    `json:"muteBeat"`
}

// options reads the session options from the second argument, if any.
func options(a []js.Value) session.Options {
	var o sayOptions
	if len(a) > 1 && a[1].Truthy() {
		_ = json.Unmarshal([]byte(js.Global().Get("JSON").Call("stringify", a[1]).String()), &o)
	}
	opts := session.Options{Speed: o.Speed, Instant: o.Instant, MuteBeat: o.MuteBeat}
	if o.BPM > 0 {
		opts.Clock = &voice.Clock{BPM: o.BPM, Phase: time.Duration(o.Phase * float64(time.Second))}
	}
	return opts
}

// addSample(name, bytes) adds a 16-bit PCM WAV file (a Uint8Array) as the
// sample called name. Returns "" or why it cannot. Load the voice again
// afterwards so its instruments use it.
func (b *bridge) addSample(_ js.Value, a []js.Value) any {
	data := make([]byte, a[1].Get("length").Int())
	js.CopyBytesToGo(data, a[1])
	if err := b.s.AddSample(a[0].String(), data); err != nil {
		return err.Error()
	}
	return ""
}

// audition(name) plays a short phrase on one instrument or percussion
// sound of the current voice. Returns "" or why it cannot.
func (b *bridge) audition(_ js.Value, a []js.Value) any {
	if !b.loaded {
		return "no voice"
	}
	if err := b.s.Audition(a[0].String()); err != nil {
		return err.Error()
	}
	return ""
}

// renderStart(text, options) starts an offline render of the line; a
// running one is dropped. False when no voice is loaded.
func (b *bridge) renderStart(_ js.Value, a []js.Value) any {
	b.render = nil
	if !b.loaded {
		return false
	}
	r, err := b.s.StartRender(a[0].String(), options(a))
	if err != nil {
		return false
	}
	b.render = r
	return true
}

// renderStep(blocks, columns) renders up to blocks blocks of the started
// render. Until it is complete it returns null; then {peaks, duration,
// timeline}: peaks holds min and max per column (session.Peaks), duration
// is the render's length in seconds (the line and its tail), timeline is
// what say would return. The finished render is kept for wav().
func (b *bridge) renderStep(_ js.Value, a []js.Value) any {
	if b.render == nil || !b.render.Step(a[0].Int()) {
		return js.Null()
	}
	l, r, tl := b.render.Result()
	b.render = nil
	b.left, b.right = l, r
	peaks := session.Peaks(l, r, a[1].Int())
	arr := js.Global().Get("Float32Array").New(len(peaks))
	for i, p := range peaks {
		arr.SetIndex(i, p)
	}
	return js.ValueOf(map[string]any{
		"peaks":    arr,
		"duration": float64(len(l)) / float64(b.s.SampleRate()),
		"timeline": toJS(session.Data(b.s.Voice(), tl)),
	})
}

// wav() returns the last render as WAV bytes (a Uint8Array), or null.
func (b *bridge) wav(js.Value, []js.Value) any {
	if b.left == nil {
		return js.Null()
	}
	data, err := session.WAV(b.left, b.right, b.s.SampleRate())
	if err != nil {
		return js.Null()
	}
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	return arr
}

// say(text, options) starts a line with the current voice and returns its
// timeline (session.TimelineData), or null when no voice is loaded.
func (b *bridge) say(_ js.Value, a []js.Value) any {
	if !b.loaded {
		return js.Null()
	}
	if err := b.s.Say(a[0].String(), options(a)); err != nil {
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
		"audition": st.Audition,
		"paused":   st.Paused,
		"time":     st.Time.Seconds(),
		"events":   events,
	})
}
