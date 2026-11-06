package dialogue

import (
	"errors"
	"io/fs"
	"path"

	"github.com/weakpixel/voice"
)

// LoadVoices resolves the voice of every speaker of s: a starter name
// through [voice.Preset], a file name through [voice.LoadVoiceFS] relative to
// the dialogue's folder ([Script.Dir]) in fsys. The result is keyed by the
// speaker's voice reference ([Speaker].Voice), so speakers that share a
// reference share the loaded voice; [NewPlayer] copies it per speaker.
//
// A voice that cannot be loaded is a [voice.FieldError] at
// "speakers[i].voice"; every problem is returned together (errors.Join).
func LoadVoices(fsys fs.FS, s *Script) (map[string]*voice.Voice, error) {
	voices := map[string]*voice.Voice{}
	failed := map[string]error{}
	var p problems
	for i, sp := range s.Speakers {
		if _, ok := voices[sp.Voice]; ok {
			continue
		}
		err, bad := failed[sp.Voice]
		if !bad {
			var v *voice.Voice
			if isStarter(sp.Voice) {
				v, err = voice.Preset(sp.Voice)
			} else {
				v, err = voice.LoadVoiceFS(fsys, path.Join(s.Dir, sp.Voice))
			}
			if err == nil {
				voices[sp.Voice] = v
				continue
			}
			failed[sp.Voice] = err
		}
		p.add("speakers["+itoa(i)+"].voice", "%v", err)
	}
	if err := errors.Join(p.errs...); err != nil {
		return nil, err
	}
	return voices, nil
}
