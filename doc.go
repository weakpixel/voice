// Package voice turns dialogue text into musical gibberish for games: text is
// split into phonemes of a fictional language, each phoneme plays a short
// blip, a quiet motif follows the punctuation, and the caller learns how much
// text to reveal at every moment.
//
// The package knows nothing about time sources or audio devices. Notes go to
// a [Sink]; the built-in synthesizer in package synth implements it. See
// doc/guide.md for the user guide and doc/design.md for how it is built.
package voice
