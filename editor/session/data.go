package session

import (
	"encoding/json"
	"time"

	"github.com/weakpixel/voice"
)

// NoteData is a timeline note for the page: times in seconds, the
// instrument by name ("vowel", ..., "motif", or the percussion sound's
// name, such as "kick").
type NoteData struct {
	On         float64 `json:"on"`
	Off        float64 `json:"off"`
	Instrument string  `json:"instrument"`
	Inst       int     `json:"inst"`
	Note       float32 `json:"note"`
	Velocity   float32 `json:"velocity"`
	// Span is 0 for the line's voice, i for the line's i-th voice span.
	Span int `json:"span,omitempty"`
}

// RevealData is a timeline reveal for the page, its time in seconds.
type RevealData struct {
	At     float64 `json:"at"`
	Offset int     `json:"offset"`
}

// TimelineData is a voice.Timeline as the page uses it (JSON).
type TimelineData struct {
	Notes    []NoteData   `json:"notes"`
	Reveals  []RevealData `json:"reveals"`
	Duration float64      `json:"duration"`
	TextLen  int          `json:"textLen"`
	// Text and Problems are set for a line said with voice markup: the
	// text without the markup, and the markup's problems.
	Text     string    `json:"text,omitempty"`
	Problems []Problem `json:"problems,omitempty"`
}

// Data converts tl, a line of v, for the page.
func Data(v *voice.Voice, tl voice.Timeline) TimelineData {
	d := TimelineData{
		Notes:    make([]NoteData, len(tl.Notes)),
		Reveals:  make([]RevealData, len(tl.Reveals)),
		Duration: sec(tl.Duration),
		TextLen:  tl.TextLen,
	}
	for i, n := range tl.Notes {
		d.Notes[i] = NoteData{On: sec(n.On), Off: sec(n.Off), Instrument: instName(v, n.Instrument), Inst: int(n.Instrument), Note: n.Note, Velocity: n.Velocity, Span: n.Span}
	}
	for i, r := range tl.Reveals {
		d.Reveals[i] = RevealData{At: sec(r.At), Offset: r.Offset}
	}
	return d
}

func sec(d time.Duration) float64 { return d.Seconds() }

func instName(v *voice.Voice, i voice.Instrument) string {
	if p := int(i) - int(voice.InstPercussion); p >= 0 && v != nil && p < len(v.Percussion) {
		return v.Percussion[p].Name
	}
	return i.String()
}

// PercussionDefaults returns each built-in percussion sound as the fields of
// a voice file's percussion entry ("note" and the instrument fields), so the
// page can show what an entry that starts from it inherits.
func PercussionDefaults() map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, name := range voice.PercussionNames() {
		p, _ := voice.BuiltinPercussion(name)
		data, _ := json.Marshal(p.Spec) // plain struct: cannot fail
		fields := map[string]any{}
		_ = json.Unmarshal(data, &fields)
		fields["note"] = p.Note
		out[name] = fields
	}
	return out
}
