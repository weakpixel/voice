package voice

import "testing"

func TestInstrumentString(t *testing.T) {
	tests := []struct {
		inst Instrument
		want string
	}{
		{InstVowel, "vowel"},
		{InstSonorant, "sonorant"},
		{InstFricative, "fricative"},
		{InstPlosive, "plosive"},
		{InstMotif, "motif"},
		{InstPercussion, "percussion(0)"},
		{InstPercussion + 7, "percussion(7)"},
		{Instrument(13), "instrument(13)"},
		{Instrument(200), "instrument(200)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.inst.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
