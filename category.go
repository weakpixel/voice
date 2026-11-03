package voice

import "strconv"

// Category is the sound class of a phoneme. Each category plays on its own
// instrument of the voice: Category(c).Instrument() is InstVowel,
// InstSonorant, InstFricative or InstPlosive.
type Category uint8

// The phoneme categories, in the order of the matching instruments.
const (
	Vowel     Category = Category(InstVowel)
	Sonorant  Category = Category(InstSonorant)
	Fricative Category = Category(InstFricative)
	Plosive   Category = Category(InstPlosive)
)

const categoryCount = 4

// Instrument returns the instrument that plays phonemes of category c.
func (c Category) Instrument() Instrument { return Instrument(c) }

// String returns the category's name as used in language files ("vowel",
// "sonorant", "fricative", "plosive").
func (c Category) String() string {
	if c < categoryCount {
		return Instrument(c).String()
	}
	return "category(" + strconv.Itoa(int(c)) + ")"
}

func parseCategory(s string) (Category, bool) {
	for c := range Category(categoryCount) {
		if c.String() == s {
			return c, true
		}
	}
	return 0, false
}

// Contour is what the end of a clause does to the melody.
type Contour uint8

const (
	ContourNone   Contour = iota // no change
	ContourFall                  // the last notes fall (a statement)
	ContourRise                  // the last notes rise (a question)
	ContourAccent                // the clause is louder and a little higher (an exclamation)
)

var contourNames = [...]string{"", "fall", "rise", "accent"}

// String returns the contour's name as used in language files ("" for
// ContourNone).
func (c Contour) String() string {
	if int(c) < len(contourNames) {
		return contourNames[c]
	}
	return "contour(" + strconv.Itoa(int(c)) + ")"
}

func parseContour(s string) (Contour, bool) {
	for i, n := range contourNames {
		if n == s {
			return Contour(i), true
		}
	}
	return 0, false
}
