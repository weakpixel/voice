// Package synth is the built-in synthesizer of the voice library: oscillator
// and sample patches, a fixed voice pool, and a [Stream] that renders PCM and
// implements voice.Sink.
//
// A [Stream] is an endless io.Reader for an audio player; [Stream.Render]
// serves offline rendering. Notes arrive through the voice.Sink methods from
// any goroutine and start at the next render block of [BlockFrames] frames.
package synth
