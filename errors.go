package voice

import "fmt"

// fieldErrors collects validation errors, each with the JSON path of the
// value it is about, so a file with several mistakes reports all of them.
type fieldErrors struct {
	prefix string // "voice: language" or "voice: voice"
	errs   []error
}

func (e *fieldErrors) add(path, format string, args ...any) {
	e.errs = append(e.errs, fmt.Errorf("%s: %s: %s", e.prefix, path, fmt.Sprintf(format, args...)))
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
