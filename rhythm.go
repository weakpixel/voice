package voice

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Rhythm makes a voice speak on a beat grid, like a rapper: every syllable
// starts on a grid step, strong steps are accented, and an optional
// percussion pattern plays under the line. A bar has 4 beats of Division
// steps each. See the guide's rhythm section for how Compose uses it.
type Rhythm struct {
	BPM      float32 // beats per minute, 40 to 240
	Division int     // steps per beat: 2 (eighths), 3 (triplets) or 4 (sixteenths)
	// Swing delays every odd step by this share of a step, 0 to 0.5.
	Swing float32
	// Accents is the accent strength, 0 to 1, of each step of a bar
	// (4*Division values). Empty means beats 1 and 3 strong, 2 and 4
	// medium, the steps between them none.
	Accents []float32
	// AccentPitch is how many semitones a step of accent 1 raises its
	// syllable, 0 to 12 (default 2). Falling accents across a bar with a
	// large AccentPitch give a line that steps down, like a laugh.
	AccentPitch float32
	// PauseRest is where the voice starts again after punctuation without
	// a contour (a comma), EndRest after punctuation with one (a period,
	// question or exclamation mark).
	PauseRest, EndRest Rest
	// CountIn is the number of bars of beat before the voice starts, 0 to 2.
	CountIn int
	// BeatVolume is the velocity of a percussion hit, 0 to 1.
	BeatVolume float32
	// Beat holds one pattern per percussion sound, sorted by sound name.
	Beat []BeatTrack
}

// BeatTrack is the pattern of one percussion sound: one character per step
// of a bar, 'x' a hit, '-' a ghost hit (half velocity), '.' a rest. It
// repeats every bar.
type BeatTrack struct {
	Sound   string // a name in Voice.Percussion
	Pattern string
}

// Rest says where a rhythmic voice starts again after punctuation. Every
// rest but RestNone leaves at least one step of silence.
type Rest uint8

const (
	RestNone Rest = iota // go on with the next step
	RestStep             // the punctuation's pause rounded to whole steps
	RestBeat             // the next beat
	RestBar              // the next bar
)

var restNames = [...]string{"none", "step", "beat", "bar"}

// String returns the rest's name as used in voice files.
func (r Rest) String() string {
	if int(r) < len(restNames) {
		return restNames[r]
	}
	return fmt.Sprintf("rest(%d)", r)
}

// The defaults of the optional rhythm fields.
const (
	DefaultBPM         = 90
	DefaultDivision    = 4
	DefaultBeatVolume  = 0.8
	DefaultAccentPitch = 2
	DefaultPauseRest   = RestBeat
	DefaultEndRest     = RestBar
)

// Clock ties a rhythmic line to music the game plays: the line takes the
// music's tempo and its first syllable lands on a bar line of the music.
type Clock struct {
	BPM float32 // the music's tempo, 40 to 240
	// Phase is the time from the start of the line until the music's next
	// bar begins. It is taken modulo a bar, so the time since the music
	// started, negated, works too.
	Phase time.Duration
}

// rhythmFile is the JSON form of a voice file's "rhythm" object.
type rhythmFile struct {
	BPM         *float32  `json:"bpm"`
	Division    *int      `json:"division"`
	Swing       float32   `json:"swing"`
	Accents     []float32 `json:"accents"`
	AccentPitch *float32  `json:"accentPitch"`
	Rest        struct {
		Pause string `json:"pause"`
		End   string `json:"end"`
	} `json:"rest"`
	CountIn    int               `json:"countIn"`
	BeatVolume *float32          `json:"beatVolume"`
	Beat       map[string]string `json:"beat"`
}

func (f *rhythmFile) build(errs *fieldErrors) *Rhythm {
	r := &Rhythm{
		BPM:         orDefault(f.BPM, DefaultBPM),
		Division:    DefaultDivision,
		Swing:       f.Swing,
		Accents:     f.Accents,
		AccentPitch: orDefault(f.AccentPitch, DefaultAccentPitch),
		PauseRest:   parseRest(errs, "rhythm.rest.pause", f.Rest.Pause, DefaultPauseRest),
		EndRest:     parseRest(errs, "rhythm.rest.end", f.Rest.End, DefaultEndRest),
		CountIn:     f.CountIn,
		BeatVolume:  orDefault(f.BeatVolume, DefaultBeatVolume),
	}
	if f.Division != nil {
		r.Division = *f.Division
	}
	for sound, pattern := range f.Beat {
		r.Beat = append(r.Beat, BeatTrack{Sound: sound, Pattern: pattern})
	}
	slices.SortFunc(r.Beat, func(a, b BeatTrack) int { return strings.Compare(a.Sound, b.Sound) })
	return r
}

func parseRest(errs *fieldErrors, path, s string, def Rest) Rest {
	if s == "" {
		return def
	}
	i := slices.Index(restNames[:], s)
	if i < 0 {
		errs.add(path, "%q is not one of none, step, beat, bar", s)
		return def
	}
	return Rest(i)
}

// barSteps returns the number of grid steps of a bar.
func (r *Rhythm) barSteps() int { return 4 * r.Division }

// validate checks r; v supplies the percussion sounds the beat may name.
func (r *Rhythm) validate(errs *fieldErrors, v *Voice) {
	errs.inRange("rhythm.bpm", r.BPM, 40, 240)
	if r.Division < 2 || r.Division > 4 {
		errs.add("rhythm.division", "%d is not 2, 3 or 4", r.Division)
		return // the lengths below depend on it
	}
	errs.inRange("rhythm.swing", r.Swing, 0, 0.5)
	if n := len(r.Accents); n != 0 && n != r.barSteps() {
		errs.add("rhythm.accents", "%d values, want %d (one per step of a bar) or none", n, r.barSteps())
	}
	for i, a := range r.Accents {
		errs.inRange(fmt.Sprintf("rhythm.accents[%d]", i), a, 0, 1)
	}
	errs.inRange("rhythm.accentPitch", r.AccentPitch, 0, 12)
	if r.PauseRest > RestBar {
		errs.add("rhythm.rest.pause", "unknown rest %d", r.PauseRest)
	}
	if r.EndRest > RestBar {
		errs.add("rhythm.rest.end", "unknown rest %d", r.EndRest)
	}
	if r.CountIn < 0 || r.CountIn > 2 {
		errs.add("rhythm.countIn", "%d is outside 0 to 2", r.CountIn)
	}
	errs.inRange("rhythm.beatVolume", r.BeatVolume, 0, 1)
	for _, t := range r.Beat {
		path := "rhythm.beat." + t.Sound
		if v.percussionIndex(t.Sound) < 0 {
			errs.add(path, "unknown sound (define it under percussion or use a built-in: %s)", strings.Join(PercussionNames(), ", "))
		}
		if len(t.Pattern) != r.barSteps() {
			errs.add(path, "%d steps, want %d (one per step of a bar)", len(t.Pattern), r.barSteps())
		}
		if i := strings.IndexFunc(t.Pattern, func(c rune) bool { return c != 'x' && c != '-' && c != '.' }); i >= 0 {
			c, _ := utf8.DecodeRuneInString(t.Pattern[i:])
			errs.add(path, "step %d is %q, want x (hit), - (ghost) or . (rest)", i+1, c)
		}
	}
}
