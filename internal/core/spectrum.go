package core

import (
	"math"
	"math/cmplx"
	"sync"
)

// Spectrum turns the sound playing into eight bands for EQFrame, bass first. It keeps the last
// fftSize samples, and each Bands call measures them. A band is shown against its own recent
// level, so a column jumps when its part of the sound gets louder: measured against one shared
// level, the bass stayed full and a kick lit the treble columns more than its own.
type Spectrum struct {
	mu     sync.Mutex
	rate   float64
	ring   [fftSize]float64
	pos    int
	avg    [gridCols]float64
	spread [gridCols]float64
	seen   [gridCols]bool
	level  [gridCols]float64
	last   float64
}

// 4096 samples tell a kick (30 to 80 Hz) from a bass line; at 48 kHz they span 85 ms.
const fftSize = 4096

// The bands' edges in Hz.
var bandEdges = [gridCols + 1]float64{30, 80, 160, 400, 1000, 2500, 5000, 9000, 16000}

const (
	spectrumQuiet = -75.0 // dB below which a band counts as silence, and doesn't move its average
	spectrumSlow  = 1.5   // seconds over which a band's average and spread follow it
	spectrumRest  = 0.3   // what a band shows at its average, 0 to 1
	spectrumGain  = 0.25  // how much more it shows per spread above its average
	levelFall     = 3.0   // 0 to 1, a second
)

func NewSpectrum() *Spectrum { return &Spectrum{rate: 48000} }

func (s *Spectrum) SetRate(hz float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if hz > 0 {
		s.rate = hz
	}
}

// Add takes mono samples, -1 to 1.
func (s *Spectrum) Add(samples []float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range samples {
		s.ring[s.pos] = float64(v)
		s.pos = (s.pos + 1) % fftSize
	}
}

// Bands is each band from 0 to 1 at now, in seconds.
func (s *Spectrum) Bands(now float64) [gridCols]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	dt := min(max(now-s.last, 0), 0.5)
	s.last = now
	follow := min(dt/spectrumSlow, 1)

	x := make([]complex128, fftSize)
	for i := range x {
		hann := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/(fftSize-1))
		x[i] = complex(s.ring[(s.pos+i)%fftSize]*hann, 0)
	}
	fft(x)

	var out [gridCols]float64
	for b := range gridCols {
		energy := 0.0
		for k := 1; k < fftSize/2; k++ {
			if f := float64(k) * s.rate / fftSize; f >= bandEdges[b] && f < bandEdges[b+1] {
				energy += math.Pow(cmplx.Abs(x[k]), 2)
			}
		}
		db := 10 * math.Log10(energy/(fftSize*fftSize)+1e-12)
		v := 0.0
		if db > spectrumQuiet {
			if !s.seen[b] {
				s.avg[b], s.spread[b], s.seen[b] = db, 3, true
			}
			v = min(max(spectrumRest+spectrumGain*(db-s.avg[b])/max(s.spread[b], 2), 0), 1)
			s.avg[b] += (db - s.avg[b]) * follow
			s.spread[b] += (math.Abs(db-s.avg[b]) - s.spread[b]) * follow
		}
		s.level[b] = max(v, s.level[b]-levelFall*dt)
		out[b] = s.level[b]
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
