// Package dialoguebox is an Ebiten game that shows a dialogue: a dark
// screen with a box at the bottom, the speaker's name, the text typing out
// in the speaker's voice, and a hint. Space, Enter or a click skips the
// typing or goes on to the next line. It plays a [dialogue.Script] through a
// [dialogue.Player].
//
// The box knows nothing about the browser or the audio device: the caller
// makes the synth.Stream and plays it (for example with the parent
// package's Player), and runs the box with ebiten.RunGame.
package dialoguebox

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/dialogue"
	"github.com/weakpixel/voice/synth"
)

// The size of the box's screen; Layout always returns it, and Ebiten scales
// it to the window.
const (
	ScreenWidth  = 480
	ScreenHeight = 270
)

// Options tune a [Box].
type Options struct {
	// Languages resolves a voice's language name, see
	// [dialogue.Options].Languages.
	Languages func(name string) *voice.Language
	// AutoAdvance plays hands-free, see [dialogue.Options].AutoAdvance.
	AutoAdvance bool
	// Samples loads the sample files that voices name; nil means none, and a
	// voice with a sample instrument is then rejected by SetScript.
	Samples synth.SampleLoader
}

// Box is an ebiten.Game around a dialogue player. Use it from Ebiten's
// goroutine only (Update, Draw, SetScript).
type Box struct {
	stream *synth.Stream
	opts   Options
	script *dialogue.Script
	player *dialogue.Player
	voices map[string]*voice.Voice
}

// New returns a box that speaks on stream. It shows nothing until
// [Box.SetScript].
func New(stream *synth.Stream, opts Options) *Box {
	return &Box{stream: stream, opts: opts}
}

// SetScript swaps the dialogue: the running player is closed (its notes
// released), the speakers' voices are set on the stream, and line start is
// played. It returns an error for a missing voice, an invalid start line or
// a voice the stream rejects, and then keeps the old dialogue.
func (b *Box) SetScript(s *dialogue.Script, voices map[string]*voice.Voice, start int) error {
	p, err := dialogue.NewPlayer(b.stream, s, voices, dialogue.Options{
		Languages: b.opts.Languages, AutoAdvance: b.opts.AutoAdvance, Start: start,
	})
	if err != nil {
		return err
	}
	for _, sv := range p.Voices() {
		if err := b.stream.SetVoice(sv.Owner, sv.Voice, b.opts.Samples); err != nil {
			p.Close()
			return fmt.Errorf("dialoguebox: voice of %s: %w", sv.Speaker, err)
		}
	}
	if b.player != nil {
		b.player.Close()
	}
	b.script, b.voices, b.player = s, voices, p
	return nil
}

// SetAutoAdvance changes hands-free play for the next SetScript.
func (b *Box) SetAutoAdvance(on bool) { b.opts.AutoAdvance = on }

// SetLanguages changes the language resolver for the next SetScript.
func (b *Box) SetLanguages(f func(name string) *voice.Language) { b.opts.Languages = f }

// SetSamples changes the sample loader for the next SetScript.
func (b *Box) SetSamples(l synth.SampleLoader) { b.opts.Samples = l }

// Script is the current script, nil before SetScript.
func (b *Box) Script() *dialogue.Script { return b.script }

// Player is the current player, nil before SetScript.
func (b *Box) Player() *dialogue.Player { return b.player }

// Step advances the dialogue by dt without reading input: Update calls it
// every tick, and tests drive the box with it.
func (b *Box) Step(dt time.Duration) {
	if b.player != nil {
		b.player.Update(dt)
	}
}

// Update implements ebiten.Game: input, then one tick of time.
func (b *Box) Update() error {
	if b.player != nil && (inpututil.IsKeyJustPressed(ebiten.KeySpace) ||
		inpututil.IsKeyJustPressed(ebiten.KeyEnter) ||
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)) {
		if b.player.Phase() == dialogue.Finished {
			b.player.Start(0)
		} else {
			b.player.Next()
		}
	}
	b.Step(time.Second / time.Duration(ebiten.TPS()))
	return nil
}

// Draw implements ebiten.Game.
func (b *Box) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x1a, 0x1c, 0x2c, 0xff})
	if b.player == nil {
		return
	}
	p := b.player
	vector.FillRect(screen, 16, 150, ScreenWidth-32, 104, color.RGBA{0x33, 0x3c, 0x57, 0xff}, false)
	if p.Phase() == dialogue.Finished {
		ebitenutil.DebugPrintAt(screen, "The end. Click to start again", 28, 160)
		return
	}
	text := p.Line().Text[:p.Revealed()]
	ebitenutil.DebugPrintAt(screen, p.Speaker().Name+":", 28, 160)
	ebitenutil.DebugPrintAt(screen, Wrap(text, 70), 28, 180)
	hint := "Space: skip"
	if p.Phase() == dialogue.Holding {
		hint = "Space: next"
	}
	ebitenutil.DebugPrintAt(screen, hint, ScreenWidth-110, 236)
	ebitenutil.DebugPrintAt(screen, "voice: "+b.voiceName(), 16, 8)
}

// voiceName is the name of the current speaker's voice.
func (b *Box) voiceName() string {
	ref := b.player.Speaker().Voice
	if v := b.voices[ref]; v != nil && v.Name != "" {
		return v.Name
	}
	return ref
}

// Layout implements ebiten.Game.
func (b *Box) Layout(int, int) (int, int) { return ScreenWidth, ScreenHeight }

// Wrap breaks text into lines of at most width bytes at spaces; a longer
// word stays whole on its own line.
func Wrap(text string, width int) string {
	var b strings.Builder
	n := 0
	for i, w := range strings.Split(text, " ") {
		if i > 0 {
			if n+1+len(w) > width {
				b.WriteByte('\n')
				n = 0
			} else {
				b.WriteByte(' ')
				n++
			}
		}
		b.WriteString(w)
		n += len(w)
	}
	return b.String()
}
