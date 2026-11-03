package synth

import (
	"math"

	"github.com/weakpixel/voice"
)

// StealFade is how long a stolen voice fades out before the new note starts
// on it. Cutting a sounding voice to zero at once would click.
const StealFade = MinEnvelopeTime

// noteOn is a note waiting to start or playing on a voice.
type noteOn struct {
	owner          voice.Owner
	patch          *Patch
	note           float32 // the played note: NoteOff and Release match it
	layer          float32 // semitones this voice sounds away from note
	velocity       float32
	pan            float32
	releaseOnStart bool // a note off arrived before the note could start
}

// voiceSlot is one voice of the pool: the state of one sounding note.
type voiceSlot struct {
	active   bool
	released bool   // note off received, envelope releasing
	started  uint64 // pool clock at the start, for stealing the oldest
	ended    bool   // the sound is over; the slot frees after this block
	on       noteOn
	gainL    float32 // velocity, patch gain and pan combined
	gainR    float32
	send     float32 // velocity, patch gain and reverb send combined
	elapsed  int     // frames since the start, for glide and vibrato
	env      envelope
	// uni are the unison copies of the main oscillator (uni[:unison]),
	// detuned by uniRatio; osc2 is the Detune oscillator.
	uni      [MaxUnison]oscillator
	uniRatio [MaxUnison]float64
	unison   int
	uniGain  float32 // 1/sqrt(unison): copies add up incoherently
	osc2     oscillator
	detuned  bool
	sample   samplePlayer
	filt     svf
	// fade counts down the frames of a steal fade; fadeLen > 0 marks a
	// voice being stolen.
	fade, fadeLen int
	pending       noteOn // the note that starts when the steal fade ends
	hasPending    bool
}

// pool is the fixed set of voices. It is owned by the render goroutine.
type pool struct {
	voices     []voiceSlot
	rate       int
	clock      uint64
	seed       uint32 // xorshift state handing out noise seeds; deterministic
	fadeFrames int
}

func newPool(n, rate int) pool {
	return pool{
		voices:     make([]voiceSlot, n),
		rate:       rate,
		seed:       0x9e3779b9,
		fadeFrames: framesOf(StealFade, rate, 1),
	}
}

// noteOn starts n with every layer of its patch.
func (p *pool) noteOn(n noteOn) {
	layers := n.patch.Layers
	if len(layers) == 0 {
		p.startVoice(n)
		return
	}
	n.velocity /= float32(math.Sqrt(float64(len(layers))))
	for _, l := range layers {
		n.layer = l
		p.startVoice(n)
	}
}

// startVoice starts n on a free voice, or steals one: the oldest releasing
// voice, else the oldest sounding one. The stolen voice fades out over
// StealFade and the note starts after it.
func (p *pool) startVoice(n noteOn) {
	victim := -1
	victimRank := 0
	for i := range p.voices {
		v := &p.voices[i]
		if !v.active && !v.hasPending {
			p.start(v, n)
			return
		}
		// Lower rank is stolen first: releasing, then sounding, then voices
		// already fading out (whose waiting note is replaced).
		rank := 1
		switch {
		case v.fadeLen > 0:
			rank = 2
		case v.released:
			rank = 0
		}
		if victim < 0 || rank < victimRank || rank == victimRank && v.started < p.voices[victim].started {
			victim, victimRank = i, rank
		}
	}
	v := &p.voices[victim]
	if v.fadeLen == 0 {
		v.fade, v.fadeLen = p.fadeFrames, p.fadeFrames
	}
	v.pending, v.hasPending = n, true
}

func (p *pool) start(v *voiceSlot, n noteOn) {
	p.clock++
	pt := n.patch
	*v = voiceSlot{active: true, started: p.clock, on: n}
	// Equal-power pan: cos/sin of a quarter turn keeps the loudness constant
	// across the stereo field.
	a := (float64(min(max(n.pan, -1), 1)) + 1) * math.Pi / 4
	g := min(max(n.velocity, 0), 1) * pt.gain()
	v.gainL, v.gainR = g*float32(math.Cos(a)), g*float32(math.Sin(a))
	v.send = g * pt.Reverb
	v.env.start(pt.Env, p.rate)
	if pt.Sample != nil {
		v.sample.init(pt.Sample, p.fadeFrames)
	} else {
		v.unison = pt.unison()
		v.uniGain = 1 / float32(math.Sqrt(float64(v.unison)))
		for k := range v.unison {
			v.uni[k].init(pt.Wave, pt.pulseWidth(), p.nextSeed())
			v.uniRatio[k] = 1
			if v.unison > 1 {
				// Spread the copies evenly across the spread; random start
				// phases (from the seeded generator) keep them from starting
				// as one loud spike.
				off := float64(pt.Spread) * (float64(k)/float64(v.unison-1) - 0.5)
				v.uniRatio[k] = math.Exp2(off / 12)
				v.uni[k].phase = float64(p.nextSeed()) / (1 << 32)
			}
		}
		if pt.Detune != 0 {
			v.osc2.init(pt.Wave, pt.pulseWidth(), p.nextSeed())
			v.detuned = true
		}
	}
	v.filt.setup(pt.Filter, p.rate)
	if n.releaseOnStart {
		v.noteOff()
	}
}

func (p *pool) nextSeed() uint32 {
	p.seed ^= p.seed << 13
	p.seed ^= p.seed >> 17
	p.seed ^= p.seed << 5
	return p.seed
}

func (v *voiceSlot) noteOff() {
	v.released = true
	v.env.noteOff()
}

// noteOff releases the notes of owner at pitch note, including one waiting
// for a steal fade.
func (p *pool) noteOff(owner voice.Owner, note float32) {
	for i := range p.voices {
		v := &p.voices[i]
		if v.active && v.fadeLen == 0 && !v.released && v.on.owner == owner && v.on.note == note {
			v.noteOff()
		}
		if v.hasPending && v.pending.owner == owner && v.pending.note == note {
			v.pending.releaseOnStart = true
		}
	}
}

// release releases every note of owner.
func (p *pool) release(owner voice.Owner) {
	for i := range p.voices {
		v := &p.voices[i]
		if v.active && !v.released && v.on.owner == owner {
			v.noteOff()
		}
		if v.hasPending && v.pending.owner == owner {
			v.pending.releaseOnStart = true
		}
	}
}

// sounding returns the number of voices that are playing or waiting to.
func (p *pool) sounding() int {
	n := 0
	for i := range p.voices {
		if p.voices[i].active || p.voices[i].hasPending {
			n++
		}
	}
	return n
}

// render adds one block of every voice to left and right, and their
// reverb sends to bus.
func (p *pool) render(left, right, bus []float32) {
	for i := range p.voices {
		v := &p.voices[i]
		if v.active {
			p.renderVoice(v, left, right, bus)
		}
		if v.ended || !v.active && v.hasPending {
			v.active = false
			if v.hasPending {
				p.start(v, v.pending)
			}
		}
	}
}

// pitchOffset is the glide and vibrato offset in semitones at the voice's
// current time. It is evaluated once per block (control rate, 750 Hz at
// 48 kHz), which is smooth for glides of a few milliseconds and vibratos of
// a few hertz and saves an exp2 per frame.
func (p *pool) pitchOffset(v *voiceSlot) float32 {
	pt := v.on.patch
	t := float64(v.elapsed) / float64(p.rate)
	var off float64
	if gt := pt.Glide.Time.Seconds(); gt > 0 && t < gt {
		off += float64(pt.Glide.From) * (1 - t/gt)
	}
	if vib := pt.Vibrato; vib.Rate > 0 && vib.Depth != 0 {
		if d := vib.Delay.Seconds(); t >= d {
			off += float64(vib.Depth) * math.Sin(2*math.Pi*float64(vib.Rate)*(t-d))
		}
	}
	return float32(off)
}

func (p *pool) renderVoice(v *voiceSlot, left, right, bus []float32) {
	pt := v.on.patch
	pitch := float64(v.on.note + v.on.layer + p.pitchOffset(v))
	if pt.Sample != nil {
		v.sample.step = float64(pt.Sample.Rate) / float64(p.rate) * math.Exp2((pitch-float64(pt.Sample.Root))/12)
	} else {
		hz := 440 * math.Exp2((pitch-69)/12)
		for k := range v.unison {
			v.uni[k].setFreq(hz*v.uniRatio[k], p.rate)
		}
		if v.detuned {
			v.osc2.setFreq(hz*math.Exp2(float64(pt.Detune)/12), p.rate)
		}
	}
	for f := range left {
		var x float32
		switch {
		case pt.Sample != nil:
			x = v.sample.next()
		default:
			x = v.uni[0].next()
			for k := 1; k < v.unison; k++ {
				x += v.uni[k].next()
			}
			if v.unison > 1 {
				x *= v.uniGain
			}
			if v.detuned {
				x = 0.5 * (x + v.osc2.next())
			}
		}
		x = v.filt.process(x) * v.env.next()
		if v.fadeLen > 0 {
			x *= float32(v.fade) / float32(v.fadeLen)
			v.fade--
		}
		left[f] += x * v.gainL
		right[f] += x * v.gainR
		bus[f] += x * v.send
		if v.env.done() || v.sample.ended || v.fadeLen > 0 && v.fade <= 0 {
			v.ended = true
			break
		}
	}
	v.elapsed += len(left)
}
