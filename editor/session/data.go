package session

import (
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
		d.Notes[i] = NoteData{On: sec(n.On), Off: sec(n.Off), Instrument: instName(v, n.Instrument), Inst: int(n.Instrument), Note: n.Note, Velocity: n.Velocity}
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
