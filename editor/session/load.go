package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/weakpixel/voice"
)

// Problem is one thing wrong with a document. Path is the JSON path of the
// value it is about (as in voice.FieldError), or "" when the problem is not
// about one value (malformed JSON, an unknown field).
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// LoadVoice loads a *.voice.json document. On failure it returns nil and
// every problem.
func LoadVoice(text string) (*voice.Voice, []Problem) {
	v, err := voice.LoadVoice(strings.NewReader(text))
	if err != nil {
		return nil, problems(err, text, "voice: voice: ")
	}
	return v, nil
}

// LoadLanguage loads a *.lang.json document. An empty or blank document is
// the default language. On failure it returns nil and every problem.
func LoadLanguage(text string) (*voice.Language, []Problem) {
	if strings.TrimSpace(text) == "" {
		return voice.DefaultLanguage(), nil
	}
	l, err := voice.LoadLanguage(strings.NewReader(text))
	if err != nil {
		return nil, problems(err, text, "voice: language: ")
	}
	return l, nil
}

// problems splits a load error into its field errors. Errors that are not
// field errors keep their text without the package prefix; a JSON syntax
// error gets its line and column in text.
func problems(err error, text, prefix string) []Problem {
	var out []Problem
	list := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		list = joined.Unwrap()
	}
	for _, e := range list {
		var fe *voice.FieldError
		var se *json.SyntaxError
		switch {
		case errors.As(e, &fe):
			out = append(out, Problem{Path: fe.Path, Message: fe.Message})
		case errors.As(e, &se):
			// Offset counts the offending byte itself.
			line, col := lineCol(text, se.Offset-1)
			out = append(out, Problem{Message: fmt.Sprintf("line %d, column %d: %s", line, col, se.Error())})
		default:
			out = append(out, Problem{Message: strings.TrimPrefix(e.Error(), prefix)})
		}
	}
	return out
}

// lineCol turns a byte offset into a 1-based line and column (in bytes).
func lineCol(text string, offset int64) (line, col int) {
	offset = min(max(offset, 0), int64(len(text)))
	before := text[:offset]
	line = strings.Count(before, "\n") + 1
	col = len(before) - strings.LastIndexByte(before, '\n')
	return line, col
}
