package voice

import (
	"errors"
	"fmt"
	"strings"
)

// MarkupSpan is a span of a line that another voice speaks, found by
// [ParseMarkup]: the bytes Start to End of the shown text, and the name
// written in its tag. What the name means (a starter voice, a voice file,
// a dialogue speaker) is up to the caller.
type MarkupSpan struct {
	Start, End int
	Name       string
	// Impression is true for a span written {name~}: the line's voice
	// imitates the named voice (see [VoiceSpan]).
	Impression bool
}

// MarkupError is one problem of a line's markup, at byte Offset of the
// source text.
type MarkupError struct {
	Offset  int
	Message string
}

// Error returns "voice: markup at byte <offset>: <message>".
func (e *MarkupError) Error() string {
	return fmt.Sprintf("voice: markup at byte %d: %s", e.Offset, e.Message)
}

// ParseMarkup splits a line written with voice markup into the text to
// show and speak and the spans of other voices:
//
//	Grandpa said {bram}keep your boots dry{/}, and left.
//
// {name} opens a span, {/} or {/name} (which must match) closes it, and
// {name~} opens an impression. {{ is a literal "{"; a "}" outside a tag is
// literal. A name is made of a-z, 0-9, "_", "." and "-"; names starting
// with "#" are reserved for other tags. Spans do not nest, and a span must
// not be empty.
//
// Every problem is returned together (errors.Join of *MarkupError, with
// offsets into src). The text and the well-formed spans are returned also
// on error: tags are always removed from the text. Spans are sorted and do
// not overlap.
func ParseMarkup(src string) (text string, spans []MarkupSpan, err error) {
	var b strings.Builder
	var errs []error
	fail := func(at int, format string, args ...any) {
		errs = append(errs, &MarkupError{Offset: at, Message: fmt.Sprintf(format, args...)})
	}
	open, openAt := MarkupSpan{}, -1 // the open span and its tag's offset
	for i := 0; i < len(src); {
		c := src[i]
		if c != '{' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 < len(src) && src[i+1] == '{' {
			b.WriteByte('{')
			i += 2
			continue
		}
		j := strings.IndexByte(src[i+1:], '}')
		if j < 0 {
			fail(i, "tag is not closed with }")
			b.WriteString(src[i:])
			break
		}
		tag := src[i+1 : i+1+j]
		at := i
		i += j + 2
		if name, closing := strings.CutPrefix(tag, "/"); closing {
			switch {
			case openAt < 0:
				fail(at, "{%s} without an open span", tag)
			case name != "" && name != open.Name:
				fail(at, "{%s} closes the span {%s}", tag, open.Name)
				openAt = -1
			case b.Len() == open.Start:
				fail(openAt, "span {%s} is empty", open.Name)
				openAt = -1
			default:
				open.End = b.Len()
				spans = append(spans, open)
				openAt = -1
			}
			continue
		}
		name, impression := strings.CutSuffix(tag, "~")
		if msg := checkMarkupName(name); msg != "" {
			fail(at, "%s", msg)
			continue
		}
		if openAt >= 0 {
			fail(at, "{%s} inside the span {%s}: spans do not nest, close it first", tag, open.Name)
			continue
		}
		open, openAt = MarkupSpan{Start: b.Len(), Name: name, Impression: impression}, at
	}
	if openAt >= 0 {
		fail(openAt, "span {%s} is not closed with {/}", open.Name)
	}
	return b.String(), spans, errors.Join(errs...)
}

// checkMarkupName returns what is wrong with a tag's name, or "".
func checkMarkupName(name string) string {
	switch {
	case name == "":
		return "empty voice name in {}"
	case name[0] == '#':
		return fmt.Sprintf("{%s} is not a voice: tags starting with # are reserved", name)
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-') {
			return fmt.Sprintf("voice name %q may only use a-z, 0-9, _, . and -", name)
		}
	}
	return ""
}
