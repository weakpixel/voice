package session

import (
	"encoding/binary"
	"io"
	"math"
	"sync/atomic"
)

// meter passes float32 stereo PCM through and remembers the loudest sample
// since the last Peak call. The audio player calls Read from its own
// goroutine (natively; in a browser everything shares one thread), the page
// calls Peak, so the peak lives in an atomic.
type meter struct {
	r    io.Reader
	peak atomic.Uint32 // math.Float32bits of the peak; positive floats order like their bits
	read atomic.Int64  // bytes read in all
}

// Read reads from the stream and updates the peak. Allocates nothing.
func (m *meter) Read(p []byte) (int, error) {
	n, err := m.r.Read(p)
	m.read.Add(int64(n))
	var peak float32
	for i := 0; i+4 <= n; i += 4 {
		x := math.Float32frombits(binary.LittleEndian.Uint32(p[i:]))
		peak = max(peak, abs32(x))
	}
	bits := math.Float32bits(peak)
	for {
		old := m.peak.Load()
		if bits <= old || m.peak.CompareAndSwap(old, bits) {
			break
		}
	}
	return n, err
}

// Peak returns the loudest sample since the previous call and resets it.
func (m *meter) Peak() float32 { return math.Float32frombits(m.peak.Swap(0)) }

func abs32(x float32) float32 { return math.Float32frombits(math.Float32bits(x) &^ (1 << 31)) }
