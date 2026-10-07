package core

import (
	"math"
	"math/cmplx"
	"sync"
)

// Spectrum turns the sound playing into eight bands for EQFrame, bass first. It keeps the last
// fftSize samples; each Bands call measures them, scales the bands against the loudest band of
// late, and lets each fall slowly so the lights don't flicker.
type Spectrum struct {
	mu    sync.Mutex
	rate  float64
	ring  [fftSize]float64
	pos   int
	level [gridCols]float64
	peak  float64
	last  float64
}

const fftSize = 1024

// The bands' edges in Hz, and how much each is lifted in dB, since music has less in the highs.
var (
	bandEdges = [gridCols + 1]float64{40, 100, 250, 500, 1000, 2000, 4000, 8000, 16000}
	bandLift  = [gridCols]float64{0, 0, 2, 4, 6, 8, 10, 12}
)

const (
	spectrumRange = 30.0  // dB from the loudest band of late down to an empty column
	spectrumQuiet = -75.0 // dB below which a band counts as silence
	spectrumFloor = -45.0 // dB the loudest band of late never drops under, so hiss stays dark
	peakFall      = 6.0   // dB a second
	levelFall     = 2.5   // columns' worth of 0 to 1, a second
)

func NewSpectrum() *Spectrum { return &Spectrum{rate: 48000, peak: spectrumFloor} }

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

	x := make([]complex128, fftSize)
	for i := range x {
		hann := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/(fftSize-1))
		x[i] = complex(s.ring[(s.pos+i)%fftSize]*hann, 0)
	}
	fft(x)

	var db [gridCols]float64
	loudest := math.Inf(-1)
	for b := range gridCols {
		energy := 0.0
		for k := 1; k < fftSize/2; k++ {
			if f := float64(k) * s.rate / fftSize; f >= bandEdges[b] && f < bandEdges[b+1] {
				energy += math.Pow(cmplx.Abs(x[k]), 2)
			}
		}
		db[b] = 10*math.Log10(energy/(fftSize*fftSize)+1e-12) + bandLift[b]
		loudest = max(loudest, db[b])
	}
	s.peak = max(s.peak-peakFall*dt, loudest, spectrumFloor)

	var out [gridCols]float64
	for b := range gridCols {
		v := 0.0
		if db[b]-bandLift[b] > spectrumQuiet {
			v = min(max((db[b]-(s.peak-spectrumRange))/spectrumRange, 0), 1)
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
