package wav

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

// pcm16 builds a 16-bit PCM WAV with an extra chunk before the data.
func pcm16(rate, channels int, frames [][]int16, extensible bool) []byte {
	le := binary.LittleEndian
	var fmtChunk []byte
	format := uint16(1)
	if extensible {
		format = 0xFFFE
	}
	fmtChunk = le.AppendUint16(fmtChunk, format)
	fmtChunk = le.AppendUint16(fmtChunk, uint16(channels))
	fmtChunk = le.AppendUint32(fmtChunk, uint32(rate))
	fmtChunk = le.AppendUint32(fmtChunk, uint32(rate*channels*2))
	fmtChunk = le.AppendUint16(fmtChunk, uint16(channels*2))
	fmtChunk = le.AppendUint16(fmtChunk, 16)
	if extensible {
		fmtChunk = le.AppendUint16(fmtChunk, 22)
		fmtChunk = le.AppendUint16(fmtChunk, 16)
		fmtChunk = le.AppendUint32(fmtChunk, 3)
		fmtChunk = le.AppendUint16(fmtChunk, 1) // PCM subformat
		fmtChunk = append(fmtChunk, make([]byte, 14)...)
	}
	var data []byte
	for _, f := range frames {
		for _, v := range f {
			data = le.AppendUint16(data, uint16(v))
		}
	}
	var b []byte
	b = append(b, "RIFF\x00\x00\x00\x00WAVE"...)
	b = append(b, "fmt "...)
	b = le.AppendUint32(b, uint32(len(fmtChunk)))
	b = append(b, fmtChunk...)
	b = append(b, "LIST\x03\x00\x00\x00abc\x00"...) // odd size, padded
	b = append(b, "data"...)
	b = le.AppendUint32(b, uint32(len(data)))
	b = append(b, data...)
	le.PutUint32(b[4:], uint32(len(b)-8))
	return b
}

func TestRead(t *testing.T) {
	tests := []struct {
		name       string
		channels   int
		frames     [][]int16
		extensible bool
		want       []float32
	}{
		{"mono", 1, [][]int16{{0}, {16384}, {-32768}}, false, []float32{0, 0.5, -1}},
		{"stereo mixed to mono", 2, [][]int16{{16384, -16384}, {16384, 16384}}, false, []float32{0, 0.5}},
		{"extensible", 1, [][]int16{{-16384}}, true, []float32{-0.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rate, err := Read(bytes.NewReader(pcm16(22050, tt.channels, tt.frames, tt.extensible)))
			if err != nil {
				t.Fatal(err)
			}
			if rate != 22050 {
				t.Errorf("rate %d", rate)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("sample %d = %g, want %g", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestReadErrors(t *testing.T) {
	good := pcm16(8000, 1, [][]int16{{1}}, false)
	bits8 := bytes.Clone(good)
	binary.LittleEndian.PutUint16(bits8[34:], 8)
	float := bytes.Clone(good)
	binary.LittleEndian.PutUint16(float[20:], 3)
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "reading the header"},
		{"not wave", []byte("RIFF\x00\x00\x00\x00AVI "), "not a RIFF WAVE"},
		{"8 bit", bits8, "8 bits per sample"},
		{"float", float, "format 3 is not PCM"},
		{"no data", good[:36], "no data chunk"},
		{"truncated", good[:len(good)-1], "reading the data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Read(bytes.NewReader(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %v, want %q", err, tt.want)
			}
		})
	}
}

func TestWriteStereo(t *testing.T) {
	left := []float32{0, 0.5, -1}
	right := []float32{1, -0.25, 0}
	var buf bytes.Buffer
	if err := WriteStereo(&buf, 44100, left, right); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	le := binary.LittleEndian
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" || int(le.Uint32(b[4:])) != len(b)-8 {
		t.Fatalf("bad RIFF header % x", b[:12])
	}
	if f, ch, rate, bits := le.Uint16(b[20:]), le.Uint16(b[22:]), le.Uint32(b[24:]), le.Uint16(b[34:]); f != 3 || ch != 2 || rate != 44100 || bits != 32 {
		t.Errorf("fmt: format %d channels %d rate %d bits %d", f, ch, rate, bits)
	}
	data := b[len(b)-24:]
	if string(b[len(b)-32:len(b)-28]) != "data" {
		t.Fatalf("no data chunk before the samples")
	}
	for i := range left {
		l := math.Float32frombits(le.Uint32(data[8*i:]))
		r := math.Float32frombits(le.Uint32(data[8*i+4:]))
		if l != left[i] || r != right[i] {
			t.Errorf("frame %d = %g %g", i, l, r)
		}
	}
	if err := WriteStereo(&buf, 44100, left, right[:1]); err == nil {
		t.Error("length mismatch accepted")
	}
}
