// Command example is a dialogue box with two speakers: each line is spoken
// in its character's voice while the text types out. Space (or a click)
// skips to the end of a line, or goes on to the next one.
//
//	go run ./example            (from the ebiten module)
package main

import (
	"fmt"
	"image/color"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/weakpixel/voice"
	voiceebiten "github.com/weakpixel/voice/ebiten"
	"github.com/weakpixel/voice/synth"
)

const (
	sampleRate   = 48000
	screenWidth  = 480
	screenHeight = 270
)

type character struct {
	name    string
	voice   *voice.Voice
	speaker *voice.Speaker
}

type line struct {
	who  int
	text string
}

var script = []line{
	{0, "Hello there, traveler! Are you lost?"},
	{1, "Lost? No. Just... a little turned around."},
	{0, "The village is past the old mill. Follow the river and you can't miss it!"},
	{1, "Thank you. Is it far?"},
	{0, "Not at all. An hour, maybe two."},
	{1, "Then I had better hurry."},
}

type game struct {
	people []*character
	line   int
	player *voiceebiten.Player
}

func newGame() (*game, error) {
	stream, err := synth.NewStream(synth.StreamConfig{SampleRate: sampleRate})
	if err != nil {
		return nil, err
	}
	g := &game{}
	for i, name := range []string{"chirpy", "gruff"} {
		v, err := voice.Preset(name)
		if err != nil {
			return nil, err
		}
		v.Pan = float32(i)*0.6 - 0.3 // a little apart in stereo
		owner := voice.Owner(i + 1)
		if err := stream.SetVoice(owner, v, nil); err != nil {
			return nil, err
		}
		g.people = append(g.people, &character{name: []string{"Wren", "Bram"}[i], voice: v, speaker: voice.NewSpeaker(stream, owner)})
	}
	if g.player, err = voiceebiten.NewPlayer(audio.NewContext(sampleRate), stream, 0); err != nil {
		return nil, err
	}
	g.say()
	return g, nil
}

func (g *game) current() *character { return g.people[script[g.line].who] }

func (g *game) say() {
	g.current().speaker.Say(g.current().voice, script[g.line].text, voice.SayOptions{})
}

func (g *game) Update() error {
	sp := g.current().speaker
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if !sp.Done() {
			sp.Skip()
		} else {
			g.line = (g.line + 1) % len(script)
			g.say()
		}
	}
	dt := time.Second / time.Duration(ebiten.TPS())
	for _, c := range g.people {
		c.speaker.Update(dt)
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x1a, 0x1c, 0x2c, 0xff})
	vector.FillRect(screen, 16, 150, screenWidth-32, 104, color.RGBA{0x33, 0x3c, 0x57, 0xff}, false)
	c := g.current()
	text := script[g.line].text[:c.speaker.Revealed()]
	ebitenutil.DebugPrintAt(screen, c.name+":", 28, 160)
	ebitenutil.DebugPrintAt(screen, wrap(text, 70), 28, 180)
	hint := "Space: skip"
	if c.speaker.Done() {
		hint = "Space: next"
	}
	ebitenutil.DebugPrintAt(screen, hint, screenWidth-110, 236)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("voice: %s", c.voice.Name), 16, 8)
}

// wrap breaks text into lines of at most width bytes at spaces.
func wrap(text string, width int) string {
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

func (g *game) Layout(int, int) (int, int) { return screenWidth, screenHeight }

func main() {
	g, err := newGame()
	if err != nil {
		log.Fatal(err)
	}
	ebiten.SetWindowSize(screenWidth*2, screenHeight*2)
	ebiten.SetWindowTitle("voice: dialogue example")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
