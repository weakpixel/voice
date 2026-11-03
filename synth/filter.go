package synth

import "math"

// svf is a state-variable low-pass in the trapezoidal (TPT) form by Andrew
// Simper (Cytomic). Unlike the classic Chamberlin form it stays stable up to
// high cutoffs and with resonance near 1, and needs only the two integrator
// states ic1 and ic2. The coefficients are computed once per note.
type svf struct {
	on         bool
	a1, a2, a3 float32
	ic1, ic2   float32
}

func (f *svf) setup(flt Filter, rate int) {
	*f = svf{}
	if flt.Cutoff <= 0 {
		return
	}
	fc := min(float64(flt.Cutoff), 0.45*float64(rate))
	g := math.Tan(math.Pi * fc / float64(rate))
	k := 2 - 2*min(max(float64(flt.Resonance), 0), 0.98) // damping: 2 = none, near 0 = self-oscillation
	a1 := 1 / (1 + g*(g+k))
	a2 := g * a1
	*f = svf{on: true, a1: float32(a1), a2: float32(a2), a3: float32(g * a2)}
}

func (f *svf) process(x float32) float32 {
	if !f.on {
		return x
	}
	v3 := x - f.ic2
	v1 := f.a1*f.ic1 + f.a2*v3
	v2 := f.ic2 + f.a2*f.ic1 + f.a3*v3
	f.ic1 = 2*v1 - f.ic1
	f.ic2 = 2*v2 - f.ic2
	return v2
}
