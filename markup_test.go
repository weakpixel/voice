package voice

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseMarkup(t *testing.T) {
	tests := []struct {
		name, src, text string
		spans           []MarkupSpan
	}{
		{"plain", "Hello there!", "Hello there!", nil},
		{"one span", "Grandpa said {bram}keep dry{/}.", "Grandpa said keep dry.", []MarkupSpan{{13, 21, "bram", false}}},
		{"named close", "a {x}b{/x} c", "a b c", []MarkupSpan{{2, 3, "x", false}}},
		{"two spans", "{a}one{/} two {b.voice.json}three{/}", "one two three", []MarkupSpan{{0, 3, "a", false}, {8, 13, "b.voice.json", false}}},
		{"impression", "he went {bram~}grr{/}", "he went grr", []MarkupSpan{{8, 11, "bram", true}}},
		{"mid word", "sooo{ghost}ooo{/}!", "soooooo!", []MarkupSpan{{4, 7, "ghost", false}}},
		{"escaped brace", "a {{b} c", "a {b} c", nil},
		{"lone close brace", "a } b", "a } b", nil},
		{"utf-8 offsets", "Grüß {x}dich{/}", "Grüß dich", []MarkupSpan{{7, 11, "x", false}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, spans, err := ParseMarkup(tt.src)
			if err != nil {
				t.Fatal(err)
			}
			if text != tt.text || !reflect.DeepEqual(spans, tt.spans) {
				t.Errorf("got %q %v, want %q %v", text, spans, tt.text, tt.spans)
			}
		})
	}
}

func TestParseMarkupErrors(t *testing.T) {
	tests := []struct {
		name, src string
		want      []string // one substring per problem
		offsets   []int
	}{
		{"unclosed tag", "a {bram", []string{"not closed with }"}, []int{2}},
		{"unclosed span", "a {bram}b", []string{"span {bram} is not closed"}, []int{2}},
		{"stray close", "a{/} b", []string{"without an open span"}, []int{1}},
		{"wrong close", "{a}x{/b}", []string{"{/b} closes the span {a}"}, []int{4}},
		{"nested", "{a}x{b}y{/}", []string{"spans do not nest"}, []int{4}},
		{"empty name", "a {} b", []string{"empty voice name"}, []int{2}},
		{"empty impression name", "a {~}b{/}", []string{"empty voice name", "without an open span"}, []int{2, 6}},
		{"reserved", "{#wave}x{/}", []string{"tags starting with # are reserved", "without an open span"}, []int{0, 8}},
		{"bad name", "{Bram}x{/}", []string{"may only use", "without an open span"}, []int{0, 7}},
		{"empty span", "a{x}{/}b", []string{"span {x} is empty"}, []int{1}},
		{"all together", "{a}{/} {B} {/}", []string{"is empty", "may only use", "without an open span"}, []int{0, 7, 11}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseMarkup(tt.src)
			if err == nil {
				t.Fatal("no error")
			}
			errs := err.(interface{ Unwrap() []error }).Unwrap()
			if len(errs) != len(tt.want) {
				t.Fatalf("got %d problems, want %d: %v", len(errs), len(tt.want), err)
			}
			for i, e := range errs {
				var me *MarkupError
				if !errors.As(e, &me) {
					t.Fatalf("problem %d is %T", i, e)
				}
				if !strings.Contains(me.Message, tt.want[i]) || me.Offset != tt.offsets[i] {
					t.Errorf("problem %d = %d %q, want %d %q", i, me.Offset, me.Message, tt.offsets[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseMarkupKeepsTextOnError(t *testing.T) {
	text, spans, err := ParseMarkup("{a}one{/} {b}two")
	if err == nil || text != "one two" || len(spans) != 1 {
		t.Errorf("got %q %v %v", text, spans, err)
	}
}

func FuzzParseMarkup(f *testing.F) {
	for _, s := range []string{"a {b}c{/} d", "{{x}", "{a~}b{/a}", "{", "}{/}", "Grüß {x}dich{/}"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		text, spans, err := ParseMarkup(src)
		if len(text) > len(src) {
			t.Fatalf("text %q longer than %q", text, src)
		}
		prev := 0
		for _, s := range spans {
			if s.Start < prev || s.End <= s.Start || s.End > len(text) || s.Name == "" {
				t.Fatalf("bad span %v in %q (text %q)", s, src, text)
			}
			prev = s.End
		}
		if err == nil && strings.Contains(src, "{") && !strings.Contains(src, "{{") && strings.Contains(text, "{") {
			t.Fatalf("tag left in %q from %q", text, src)
		}
	})
}
