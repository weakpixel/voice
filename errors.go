package voice

import "fmt"

// FieldError is one validation error of a language or voice file: the JSON
// path of the value it is about (for example "instruments.vowel.sustain" or
// "phonemes[2].symbol") and what is wrong with it. LoadLanguage, LoadVoice
// and Voice.Validate join every FieldError they find with errors.Join, so a
// tool that shows errors next to the field unwraps the joined error (its
// Unwrap() []error method) or uses errors.As for the first one.
type FieldError struct {
	// File is "language" or "voice".
	File string
	// Path is the JSON path of the value, as in the file.
	Path string
	// Message says what is wrong, for example "2 is outside 0 to 1".
	Message string
}

// Error returns "voice: <file>: <path>: <message>".
func (e *FieldError) Error() string {
	return "voice: " + e.File + ": " + e.Path + ": " + e.Message
}

// fieldErrors collects validation errors, each with the JSON path of the
// value it is about, so a file with several mistakes reports all of them.
type fieldErrors struct {
	file string // "language" or "voice"
	errs []error
}

func (e *fieldErrors) add(path, format string, args ...any) {
	e.errs = append(e.errs, &FieldError{File: e.file, Path: path, Message: fmt.Sprintf(format, args...)})
}

// finite reports whether v is a finite number and records an error if not.
func (e *fieldErrors) finite(path string, v float32) bool {
	if v != v || v > maxFloat || v < -maxFloat {
		e.add(path, "not a finite number")
		return false
	}
	return true
}

// inRange records an error when v is outside lo to hi.
func (e *fieldErrors) inRange(path string, v, lo, hi float32) {
	if e.finite(path, v) && (v < lo || v > hi) {
		e.add(path, "%g is outside %g to %g", v, lo, hi)
	}
}

const maxFloat = 3.4028234663852886e38
