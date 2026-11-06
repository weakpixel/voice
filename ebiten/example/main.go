// Command example plays a dialogue in an Ebiten window: each line is spoken
// in its character's voice while the text types out. Space, Enter or a click
// skips to the end of a line, or goes on to the next one.
//
// Without arguments it plays the embedded village.dialogue.json. With a file
// it plays that dialogue; voice files it names are read from the file's
// folder.
//
//	go run ./example [file.dialogue.json]     (from the ebiten module)
package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/weakpixel/voice/dialogue"
	voiceebiten "github.com/weakpixel/voice/ebiten"
	"github.com/weakpixel/voice/ebiten/dialoguebox"
	"github.com/weakpixel/voice/synth"
)

const sampleRate = 48000

//go:embed village.dialogue.json
var village embed.FS

func main() {
	var fsys fs.FS = village
	name := "village.dialogue.json"
	if len(os.Args) > 1 {
		dir, file := filepath.Split(os.Args[1])
		if dir == "" {
			dir = "."
		}
		fsys, name = os.DirFS(dir), file
	}
	script, err := dialogue.LoadFS(fsys, name)
	if err != nil {
		log.Fatal(err)
	}
	voices, err := dialogue.LoadVoices(fsys, script)
	if err != nil {
		log.Fatal(err)
	}
	stream, err := synth.NewStream(synth.StreamConfig{SampleRate: sampleRate})
	if err != nil {
		log.Fatal(err)
	}
	box := dialoguebox.New(stream, dialoguebox.Options{})
	if err := box.SetScript(script, voices, 0); err != nil {
		log.Fatal(err)
	}
	player, err := voiceebiten.NewPlayer(audio.NewContext(sampleRate), stream, 0)
	if err != nil {
		log.Fatal(err)
	}
	defer player.Close()
	ebiten.SetWindowSize(dialoguebox.ScreenWidth*2, dialoguebox.ScreenHeight*2)
	ebiten.SetWindowTitle("voice: dialogue example")
	if err := ebiten.RunGame(box); err != nil {
		log.Fatal(err)
	}
}
