// Package wav reads 16-bit PCM WAV files into mono float32 samples and
// writes float32 stereo WAV files. It uses the standard library only.
package wav

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	formatPCM        = 1
	formatFloat      = 3
	formatExtensible = 0xFFFE
)

// Read decodes a 16-bit PCM WAV file (format 1, or WAVE_FORMAT_EXTENSIBLE
// with the PCM subformat) of any channel count. It returns the channels
// mixed to mono as samples from -1 to 1, and the sample rate. Chunks other
// than "fmt " and "data" are skipped. Errors start with "wav: ".
func Read(r io.Reader) (samples []float32, rate int, err error) {
	br := bufio.NewReader(r)
	var hdr [12]byte
	if _, err := io.ReadFull(br, hdr[:]); err != nil {
		return nil, 0, fmt.Errorf("wav: reading the header: %w", err)
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return nil, 0, errors.New("wav: not a RIFF WAVE file")
	}
	var channels, bits, format int
	haveFmt := false
	for {
		var ch [8]byte
		if _, err := io.ReadFull(br, ch[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return nil, 0, errors.New("wav: no data chunk")
			}
			return nil, 0, fmt.Errorf("wav: reading a chunk header: %w", err)
		}
		id, size := string(ch[0:4]), int64(binary.LittleEndian.Uint32(ch[4:8]))
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, 0, fmt.Errorf("wav: fmt chunk of %d bytes, want at least 16", size)
			}
			b := make([]byte, size)
			if _, err := io.ReadFull(br, b); err != nil {
				return nil, 0, fmt.Errorf("wav: reading the fmt chunk: %w", err)
			}
			format = int(binary.LittleEndian.Uint16(b[0:2]))
			channels = int(binary.LittleEndian.Uint16(b[2:4]))
			rate = int(binary.LittleEndian.Uint32(b[4:8]))
			bits = int(binary.LittleEndian.Uint16(b[14:16]))
			if format == formatExtensible && size >= 26 {
				format = int(binary.LittleEndian.Uint16(b[24:26])) // first two bytes of the subformat GUID
			}
			var errs []error
			if format != formatPCM {
				errs = append(errs, fmt.Errorf("wav: format %d is not PCM", format))
			}
			if bits != 16 {
				errs = append(errs, fmt.Errorf("wav: %d bits per sample, want 16", bits))
			}
			if channels < 1 {
				errs = append(errs, errors.New("wav: no channels"))
			}
			if rate <= 0 {
				errs = append(errs, errors.New("wav: sample rate 0"))
			}
			if err := errors.Join(errs...); err != nil {
				return nil, 0, err
			}
			haveFmt = true
		case "data":
			if !haveFmt {
				return nil, 0, errors.New("wav: data chunk before the fmt chunk")
			}
			frames := size / int64(2*channels)
			samples = make([]float32, frames)
			frame := make([]byte, 2*channels)
			for i := range samples {
				if _, err := io.ReadFull(br, frame); err != nil {
					return nil, 0, fmt.Errorf("wav: reading the data: %w", err)
				}
				var sum float32
				for c := range channels {
					sum += float32(int16(binary.LittleEndian.Uint16(frame[2*c:]))) / 32768
				}
				samples[i] = sum / float32(channels)
			}
			return samples, rate, nil
		default:
			if _, err := br.Discard(int(size + size&1)); err != nil { // chunks are padded to even sizes
				return nil, 0, fmt.Errorf("wav: skipping chunk %q: %w", id, err)
			}
		}
	}
}

// WriteStereo writes left and right (same length, samples -1 to 1) as an
// interleaved 32-bit float stereo WAV file at rate.
func WriteStereo(w io.Writer, rate int, left, right []float32) error {
	if len(left) != len(right) {
		return fmt.Errorf("wav: left has %d samples, right %d", len(left), len(right))
	}
	if rate <= 0 {
		return fmt.Errorf("wav: sample rate %d is not positive", rate)
	}
	const channels, bytesPer = 2, 4
	dataSize := len(left) * channels * bytesPer
	if dataSize > math.MaxUint32-58 {
		return errors.New("wav: too long for a WAV file")
	}
	bw := bufio.NewWriter(w)
	le := binary.LittleEndian
	var h []byte
	h = append(h, "RIFF"...)
	h = le.AppendUint32(h, uint32(4+(8+18)+(8+4)+8+dataSize))
	h = append(h, "WAVE"...)
	// fmt chunk of 18 bytes (cbSize 0), as non-PCM formats require.
	h = append(h, "fmt "...)
	h = le.AppendUint32(h, 18)
	h = le.AppendUint16(h, formatFloat)
	h = le.AppendUint16(h, channels)
	h = le.AppendUint32(h, uint32(rate))
	h = le.AppendUint32(h, uint32(rate*channels*bytesPer))
	h = le.AppendUint16(h, channels*bytesPer)
	h = le.AppendUint16(h, 8*bytesPer)
	h = le.AppendUint16(h, 0)
	h = append(h, "fact"...)
	h = le.AppendUint32(h, 4)
	h = le.AppendUint32(h, uint32(len(left)))
	h = append(h, "data"...)
	h = le.AppendUint32(h, uint32(dataSize))
	bw.Write(h)
	var frame [8]byte
	for i := range left {
		le.PutUint32(frame[0:], math.Float32bits(left[i]))
		le.PutUint32(frame[4:], math.Float32bits(right[i]))
		bw.Write(frame[:])
	}
	return bw.Flush()
}
