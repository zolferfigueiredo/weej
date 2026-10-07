package core

import (
	"math"
	"math/cmplx"
	"sync"
)

// Spectrum keeps the last fftSize samples of each side of the sound playing, for the EQ
// patterns, which each read it through an Analyzer.
type Spectrum struct {
	mu   sync.Mutex
	rate float64
	ring [2][fftSize]float64
	pos  int
}

// 4096 samples tell a kick (30 to 80 Hz) from a bass line; at 48 kHz they span 85 ms.
const fftSize = 4096

// The sides an Analyzer listens to.
const (
	SideBoth  = -1
	SideLeft  = 0
	SideRight = 1
)

func NewSpectrum() *Spectrum { return &Spectrum{rate: 48000} }

func (s *Spectrum) SetRate(hz float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if hz > 0 {
		s.rate = hz
	}
}

// Add takes the same samples for both sides, -1 to 1.
func (s *Spectrum) Add(samples []float32) { s.AddStereo(samples, samples) }

// AddStereo takes each side's samples, -1 to 1, as many of each.
func (s *Spectrum) AddStereo(left, right []float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range min(len(left), len(right)) {
		s.ring[0][s.pos], s.ring[1][s.pos] = float64(left[i]), float64(right[i])
		s.pos = (s.pos + 1) % fftSize
	}
}

// power is the sound's power by FFT bin for a side, oldest sample first through a Hann window.
func (s *Spectrum) power(side int) ([]float64, float64) {
	s.mu.Lock()
	x := make([]complex128, fftSize)
	for i := range x {
		j := (s.pos + i) % fftSize
		v := s.ring[0][j]
		switch side {
		case SideRight:
			v = s.ring[1][j]
		case SideBoth:
			v = (s.ring[0][j] + s.ring[1][j]) / 2
		}
		hann := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/(fftSize-1))
		x[i] = complex(v*hann, 0)
	}
	rate := s.rate
	s.mu.Unlock()
	fft(x)
	out := make([]float64, fftSize/2)
	for k := range out {
		out[k] = math.Pow(cmplx.Abs(x[k]), 2)
	}
	return out, rate
}

// Analyzer is one side of the sound in bands, each shown against its own recent level, so a
// column jumps when its part of the sound gets louder: measured against one shared level, the
// bass stayed full and a kick lit the treble columns more than its own.
type Analyzer struct {
	edges  []float64
	side   int
	avg    []float64
	spread []float64
	seen   []bool
	level  []float64
	last   float64
}

const (
	spectrumQuiet = -75.0 // dB below which a band counts as silence, and doesn't move its average
	spectrumSlow  = 1.5   // seconds over which a band's average and spread follow it
	spectrumRest  = 0.3   // what a band shows at its average, 0 to 1
	spectrumGain  = 0.25  // how much more it shows per spread above its average
	levelFall     = 3.0   // 0 to 1, a second
)

// NewAnalyzer splits a side of the sound into bands between edges, in Hz, lowest first.
func NewAnalyzer(edges []float64, side int) *Analyzer {
	n := len(edges) - 1
	return &Analyzer{edges: edges, side: side, avg: make([]float64, n), spread: make([]float64, n),
		seen: make([]bool, n), level: make([]float64, n)}
}

// Bands is each band from 0 to 1 at now, in seconds.
func (a *Analyzer) Bands(s *Spectrum, now float64) []float64 {
	dt := min(max(now-a.last, 0), 0.5)
	a.last = now
	follow := min(dt/spectrumSlow, 1)
	power, rate := s.power(a.side)
	out := make([]float64, len(a.level))
	for b := range a.level {
		energy := 0.0
		for k := 1; k < len(power); k++ {
			if f := float64(k) * rate / fftSize; f >= a.edges[b] && f < a.edges[b+1] {
				energy += power[k]
			}
		}
		db := 10 * math.Log10(energy/(fftSize*fftSize)+1e-12)
		v := 0.0
		if db > spectrumQuiet {
			if !a.seen[b] {
				a.avg[b], a.spread[b], a.seen[b] = db, 3, true
			}
			v = min(max(spectrumRest+spectrumGain*(db-a.avg[b])/max(a.spread[b], 2), 0), 1)
			a.avg[b] += (db - a.avg[b]) * follow
			a.spread[b] += (math.Abs(db-a.avg[b]) - a.spread[b]) * follow
		}
		a.level[b] = max(v, a.level[b]-levelFall*dt)
		out[b] = a.level[b]
	}
	return out
}

// EQ is what an EQ pattern shows on the eight columns: "eq" and "eq2" are both sides in eight
// bands each their own way, and "eqgame" the left side's four bands from the left, bass first,
// beside the right side's four from the right, so a sound's side shows where it comes from.
type EQ struct {
	left, right *Analyzer
}

var eqEdges = map[string][]float64{
	"eq":  {30, 80, 160, 400, 1000, 2500, 5000, 9000, 16000},
	"eq2": {20, 100, 400, 1000, 2000, 4000, 8000, 13000, 20000},
}

var eqGameEdges = []float64{20, 400, 3000, 9000, 20000}

// IsEQ tells whether a pattern follows the sound.
func IsEQ(pattern string) bool { return pattern == "eq" || pattern == "eq2" || pattern == "eqgame" }

func NewEQ(pattern string) *EQ {
	if pattern == "eqgame" {
		return &EQ{left: NewAnalyzer(eqGameEdges, SideLeft), right: NewAnalyzer(eqGameEdges, SideRight)}
	}
	edges, ok := eqEdges[pattern]
	if !ok {
		return nil
	}
	return &EQ{left: NewAnalyzer(edges, SideBoth)}
}

// Columns is each column's height from 0 to 1 at now, in seconds, for EQFrame.
func (e *EQ) Columns(s *Spectrum, now float64) [gridCols]float64 {
	var out [gridCols]float64
	left := e.left.Bands(s, now)
	if e.right == nil {
		copy(out[:], left)
		return out
	}
	right := e.right.Bands(s, now)
	half := gridCols / 2
	for i := range half {
		out[i] = left[i]
		out[gridCols-1-i] = right[i]
	}
	return out
}

// fft is an in-place radix-2 transform; len(x) is a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := range size / 2 {
				a, b := x[start+k], x[start+k+size/2]*wk
				x[start+k], x[start+k+size/2] = a+b, a-b
				wk *= w
			}
		}
	}
}
