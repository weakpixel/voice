package voice

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// TokenKind is what a [Phoneme] of a parsed line is.
type TokenKind uint8

const (
	// TokenPhoneme sounds: a note on the instrument of its category.
	TokenPhoneme TokenKind = iota
	// TokenPause is silence: whitespace, punctuation, or an unknown letter
	// under UnknownPause.
	TokenPause
	// TokenSkip is revealed at once without sound or time: characters the
	// language does not know.
	TokenSkip
)

// Phoneme is one token of a parsed line. The spans of a line's tokens tile
// the text: each starts where the previous ended, the first at 0 and the
// last ending at len(text).
type Phoneme struct {
	Kind       TokenKind
	Symbol     string   // the language's symbol; for whitespace " ", for skips the text itself
	Category   Category // TokenPhoneme only
	Pitch      float32  // TokenPhoneme: semitones from the language
	Length     float32  // TokenPhoneme: slot in beats; TokenPause: pause in beats
	Velocity   float32  // TokenPhoneme: relative loudness
	Contour    Contour  // TokenPause from punctuation: its clause end rule
	Clause     bool     // TokenPause from punctuation: ends a clause
	Start, End int      // byte span in the text
}

// Parse splits text into the phonemes of lang. See [AppendParse].
func Parse(lang *Language, text string) []Phoneme {
	return AppendParse(nil, lang, text)
}

// AppendParse appends the tokens of text to dst and returns the result.
//
// Symbols are matched longest first and case-insensitively (Unicode simple
// case folding, no normalization: "é" written as e plus a combining accent
// is two characters). A run of whitespace is one pause. Letters and digits
// the language does not know follow its UnknownPolicy; any other unknown
// character, and invalid UTF-8, is skipped. Parse is pure and allocates
// only to grow dst.
func AppendParse(dst []Phoneme, lang *Language, text string) []Phoneme {
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if unicode.IsSpace(r) {
			j := i + size
			for j < len(text) {
				r, s := utf8.DecodeRuneInString(text[j:])
				if !unicode.IsSpace(r) {
					break
				}
				j += s
			}
			dst = append(dst, Phoneme{Kind: TokenPause, Symbol: " ", Length: lang.whitespacePause, Start: i, End: j})
			i = j
			continue
		}
		if n, end := lang.trie.longest(text, i); n != nil {
			dst = append(dst, lang.token(n.phoneme, n.punct, i, end))
			i = end
			continue
		}
		end := i + size
		switch {
		case r == utf8.RuneError && size == 1, !unicode.IsLetter(r) && !unicode.IsDigit(r), lang.unknown == UnknownSkip:
			dst = append(dst, Phoneme{Kind: TokenSkip, Symbol: text[i:end], Start: i, End: end})
		case lang.unknown == UnknownPause:
			dst = append(dst, Phoneme{Kind: TokenPause, Symbol: " ", Length: lang.whitespacePause, Start: i, End: end})
		default: // UnknownFallback
			dst = append(dst, lang.token(lang.fallback, -1, i, end))
		}
		i = end
	}
	return dst
}

// token builds the token for phoneme index ph or punctuation index pu (the
// other is -1).
func (l *Language) token(ph, pu, start, end int) Phoneme {
	if ph >= 0 {
		p := &l.phonemes[ph]
		return Phoneme{Kind: TokenPhoneme, Symbol: p.Symbol, Category: p.Category, Pitch: p.Pitch,
			Length: p.Length, Velocity: p.Velocity, Start: start, End: end}
	}
	p := &l.punctuation[pu]
	return Phoneme{Kind: TokenPause, Symbol: p.Symbol, Length: p.Pause, Contour: p.Contour, Clause: true, Start: start, End: end}
}

// trieNode is a node of the prefix tree of folded symbols. A node that ends
// a symbol has phoneme or punct set (the other is -1).
type trieNode struct {
	next    map[rune]*trieNode
	phoneme int
	punct   int
}

func newTrieNode() *trieNode { return &trieNode{phoneme: -1, punct: -1} }

func buildTrie(l *Language) *trieNode {
	root := newTrieNode()
	insert := func(sym string) *trieNode {
		n := root
		for _, r := range sym {
			r = foldRune(r)
			c := n.next[r]
			if c == nil {
				if n.next == nil {
					n.next = map[rune]*trieNode{}
				}
				c = newTrieNode()
				n.next[r] = c
			}
			n = c
		}
		return n
	}
	for i, p := range l.phonemes {
		insert(p.Symbol).phoneme = i
	}
	for i, p := range l.punctuation {
		insert(p.Symbol).punct = i
	}
	return root
}

// longest returns the node of the longest symbol starting at text[i:] and
// the end of the match, or nil.
func (t *trieNode) longest(text string, i int) (*trieNode, int) {
	var best *trieNode
	bestEnd := i
	n := t
	for j := i; j < len(text); {
		r, s := utf8.DecodeRuneInString(text[j:])
		n = n.next[foldRune(r)]
		if n == nil {
			break
		}
		j += s
		if n.phoneme >= 0 || n.punct >= 0 {
			best, bestEnd = n, j
		}
	}
	return best, bestEnd
}

// foldRune maps r to the smallest rune of its simple case folding orbit, so
// all case variants of a letter (K, k and the Kelvin sign) meet in one key.
func foldRune(r rune) rune {
	m := r
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		m = min(m, f)
	}
	return m
}

func foldString(s string) string {
	return strings.Map(foldRune, s)
}
