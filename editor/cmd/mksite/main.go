// Command mksite assembles the Voice Editor's static site: the page from
// editor/web, the starter voices and the default language from the
// repository's presets (with presets/index.json listing the starters in
// voice.PresetNames order), and wasm_exec.js from the Go installation. It
// stamps the version into index.html for cache busting. voice.wasm is built
// separately (make editor).
//
//	go run ./cmd/mksite -out ../build/editor -version abc1234
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/weakpixel/voice"
)

func main() {
	out := flag.String("out", "../build/editor", "output directory")
	web := flag.String("web", "web", "the page's source directory")
	presets := flag.String("presets", "../presets", "the repository's presets directory")
	version := flag.String("version", "dev", "version stamped into index.html")
	flag.Parse()
	if err := build(*out, *web, *presets, *version, filepath.Join(runtime.GOROOT(), "lib", "wasm", "wasm_exec.js")); err != nil {
		fmt.Fprintln(os.Stderr, "mksite:", err)
		os.Exit(1)
	}
}

// build writes the site into out (voice.wasm aside).
func build(out, web, presets, version, wasmExec string) error {
	if err := copyTree(web, out, func(rel string) bool {
		return !strings.HasPrefix(rel, "test"+string(filepath.Separator)) && rel != "test"
	}); err != nil {
		return err
	}
	index := filepath.Join(out, "index.html")
	html, err := os.ReadFile(index)
	if err != nil {
		return err
	}
	if err := os.WriteFile(index, []byte(strings.ReplaceAll(string(html), "__VERSION__", version)), 0o644); err != nil {
		return err
	}
	dst := filepath.Join(out, "presets")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	var errs []error
	for _, name := range voice.PresetNames() {
		errs = append(errs, copyFile(filepath.Join(presets, name+".voice.json"), filepath.Join(dst, name+".voice.json")))
	}
	errs = append(errs, copyFile(filepath.Join(presets, "default.lang.json"), filepath.Join(dst, "default.lang.json")))
	names, _ := json.Marshal(voice.PresetNames())
	errs = append(errs, os.WriteFile(filepath.Join(dst, "index.json"), append(names, '\n'), 0o644))
	errs = append(errs, copyFile(wasmExec, filepath.Join(out, "wasm_exec.js")))
	return errors.Join(errs...)
}

// copyTree copies the files of src below dst, those keep accepts (by path
// relative to src).
func copyTree(src, dst string, keep func(rel string) bool) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || !keep(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyFile(path, filepath.Join(dst, rel))
	})
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = io.Copy(out, in)
	return err
}
