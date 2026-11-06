//go:build js && wasm

// Command game is the Voice Editor's dialogue game in the browser: the
// Ebiten dialogue box (ebiten/dialoguebox) playing a dialogue the page
// sends, in an iframe of its own (editor/web/game.html). It cannot share
// voice.wasm: Ebiten owns its canvas, its loop and its audio context.
//
// The page talks to game.html with postMessage; game.html calls the
// voiceGame object this program puts on the iframe's global scope, and the
// program calls voiceGameEvent(json) for what happens in the game:
//
//	page to game: load {dialogue, voices, language, start, autoAdvance}
//	              (and samples, see game.html), start {line}, pause, resume,
//	              volume {value}
//	game to page: ready, line {index}, finished, problems {list},
//	              audio {running}
//
// The logic that does not need syscall/js is package load.
package main

import (
	"encoding/json"
	"log"
	"sync"
	"syscall/js"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/dialogue"
	voiceebiten "github.com/weakpixel/voice/ebiten"
	"github.com/weakpixel/voice/ebiten/dialoguebox"
	"github.com/weakpixel/voice/editor/game/load"
	"github.com/weakpixel/voice/synth"
)

const sampleRate = 48000

// game wraps the box. The JS callbacks run on another goroutine than
// Ebiten's loop, so they only queue commands; Update runs them.
type game struct {
	box    *dialoguebox.Box
	player *voiceebiten.Player
	ctx    *audio.Context

	mu       sync.Mutex
	commands []func()

	audioRunning bool
	audioSent    bool
}

func (g *game) queue(f func()) {
	g.mu.Lock()
	g.commands = append(g.commands, f)
	g.mu.Unlock()
}

func (g *game) Update() error {
	g.mu.Lock()
	cmds := g.commands
	g.commands = nil
	g.mu.Unlock()
	for _, f := range cmds {
		f()
	}
	if err := g.box.Update(); err != nil {
		return err
	}
	if p := g.box.Player(); p != nil {
		for _, e := range p.Events() {
			switch e.Kind {
			case dialogue.LineStarted:
				send(map[string]any{"type": "line", "index": e.Line})
			case dialogue.DialogueFinished:
				send(map[string]any{"type": "finished"})
			}
		}
	}
	if ready := g.ctx.IsReady(); !g.audioSent || ready != g.audioRunning {
		g.audioRunning, g.audioSent = ready, true
		send(map[string]any{"type": "audio", "running": ready})
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image)  { g.box.Draw(screen) }
func (g *game) Layout(w, h int) (int, int) { return g.box.Layout(w, h) }

// send posts an event to the page through game.html.
func send(msg map[string]any) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	if f := js.Global().Get("voiceGameEvent"); f.Type() == js.TypeFunction {
		f.Invoke(string(data))
	}
}

// load runs on Ebiten's goroutine: it decodes the request and swaps the
// dialogue, or tells the page what is wrong and keeps the old one.
func (g *game) load(req load.Request) {
	loaded, problems := load.Decode(req)
	if len(problems) == 0 {
		g.box.SetAutoAdvance(req.AutoAdvance)
		g.box.SetLanguages(languageFor(loaded))
		g.box.SetSamples(loaded.Samples)
		if err := g.box.SetScript(loaded.Script, loaded.Voices, req.Start); err != nil {
			problems = []load.Problem{{File: "dialogue", Message: err.Error()}}
		}
	}
	if problems == nil {
		problems = []load.Problem{}
	}
	send(map[string]any{"type": "problems", "list": problems})
}

func main() {
	stream, err := synth.NewStream(synth.StreamConfig{SampleRate: sampleRate})
	if err != nil {
		log.Fatal(err)
	}
	ctx := audio.NewContext(sampleRate)
	player, err := voiceebiten.NewPlayer(ctx, stream, 0)
	if err != nil {
		log.Fatal(err)
	}
	g := &game{box: dialoguebox.New(stream, dialoguebox.Options{}), player: player, ctx: ctx}

	js.Global().Set("voiceGame", map[string]any{
		"load": js.FuncOf(func(_ js.Value, a []js.Value) any {
			var req load.Request
			if err := json.Unmarshal([]byte(a[0].String()), &req); err != nil {
				send(map[string]any{"type": "problems", "list": []load.Problem{{File: "dialogue", Message: err.Error()}}})
				return nil
			}
			if len(a) > 1 { // sample files: an object of name to Uint8Array
				req.Samples = samplesFromJS(a[1])
			}
			g.queue(func() { g.load(req) })
			return nil
		}),
		"start": js.FuncOf(func(_ js.Value, a []js.Value) any {
			line := a[0].Int()
			g.queue(func() {
				if p := g.box.Player(); p != nil && line >= 0 && line < len(g.box.Script().Lines) {
					p.Start(line)
				}
			})
			return nil
		}),
		"pause": js.FuncOf(func(js.Value, []js.Value) any {
			g.queue(func() {
				if p := g.box.Player(); p != nil {
					p.Pause()
				}
			})
			return nil
		}),
		"resume": js.FuncOf(func(js.Value, []js.Value) any {
			g.queue(func() {
				if p := g.box.Player(); p != nil {
					p.Resume()
				}
			})
			return nil
		}),
		"volume": js.FuncOf(func(_ js.Value, a []js.Value) any {
			player.SetVolume(a[0].Float())
			return nil
		}),
	})
	ebiten.SetRunnableOnUnfocused(true)
	send(map[string]any{"type": "ready"})
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}

// samplesFromJS reads an object of sample name to Uint8Array.
func samplesFromJS(v js.Value) map[string][]byte {
	out := map[string][]byte{}
	if v.IsUndefined() || v.IsNull() {
		return out
	}
	keys := js.Global().Get("Object").Call("keys", v)
	for i := range keys.Length() {
		name := keys.Index(i).String()
		arr := v.Get(name)
		buf := make([]byte, arr.Length())
		js.CopyBytesToGo(buf, arr)
		out[name] = buf
	}
	return out
}

// languageFor returns the resolver for the request's language: the one
// language document serves every voice (nil means the built-in language).
func languageFor(l *load.Loaded) func(string) *voice.Language {
	if l.Language == nil {
		return nil
	}
	return func(string) *voice.Language { return l.Language }
}
