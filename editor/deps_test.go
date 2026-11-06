package editor_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ebiten is linked only into game.wasm: the Voice Editor's own program
// (voice.wasm: session and wasm) must not import it, so the editor's page
// does not grow by Ebiten's size.
func TestVoiceWasmHasNoEbiten(t *testing.T) {
	for _, dir := range []string{"session", "wasm"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("%s: %v (%d files)", dir, err, len(files))
		}
		fset := token.NewFileSet()
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parser.ParseFile(fset, f, src, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range parsed.Imports {
				if strings.Contains(imp.Path.Value, "hajimehoshi/ebiten") || strings.Contains(imp.Path.Value, "voice/ebiten") {
					t.Errorf("%s imports %s", f, imp.Path.Value)
				}
			}
		}
	}
}
