// Package session is the part of the Voice Editor that runs in Go: it loads
// voice and language documents (reporting every problem with its JSON
// path), plays test lines on a synth.Stream through a voice.Speaker exactly
// as a game does, meters what the audio player reads, and renders lines
// offline for the waveform and the WAV download.
//
// It knows nothing about browsers or audio devices: the js/wasm bridge
// (editor/wasm) hands Session.Reader to an oto player and calls the session
// from the page, and the tests drive it natively.
package session
