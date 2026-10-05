//go:build windows

package zoom

import (
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	magnification                 = windows.NewLazySystemDLL("Magnification.dll")
	procMagInitialize             = magnification.NewProc("MagInitialize")
	procMagUninitialize           = magnification.NewProc("MagUninitialize")
	procMagSetFullscreenTransform = magnification.NewProc("MagSetFullscreenTransform")

	user32              = windows.NewLazySystemDLL("user32.dll")
	procGetCursorPos    = user32.NewProc("GetCursorPos")
	procGetSystemMetric = user32.NewProc("GetSystemMetrics")
)

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	maxFactor = 10
	frame     = 16 * time.Millisecond
)

// Zoom magnifies the whole desktop like TheeJ's screen zoom: 1x at the bottom of the knob,
// 10x at the top, with the view following the pointer. One goroutine owns the magnifier.
type Zoom struct {
	log  func(string)
	mu   sync.Mutex
	want float64
	wake chan struct{}
	off  chan chan struct{}
}

func New(log func(string)) *Zoom {
	z := &Zoom{log: log, want: 1, wake: make(chan struct{}, 1), off: make(chan chan struct{})}
	go z.run()
	return z
}

func (z *Zoom) Set(s float64) {
	if s < 0 {
		s = 0
	}
	if s > 1 {
		s = 1
	}
	z.mu.Lock()
	z.want = math.Pow(maxFactor, s)
	z.mu.Unlock()
	select {
	case z.wake <- struct{}{}:
	default:
	}
}

// Off zooms back out and waits briefly for it, so quitting never leaves the screen magnified.
func (z *Zoom) Off() {
	done := make(chan struct{})
	select {
	case z.off <- done:
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	case <-time.After(time.Second):
	}
}

func (z *Zoom) run() {
	runtime.LockOSThread()
	if ok, _, err := procMagInitialize.Call(); ok == 0 {
		z.log("Screen zoom is unavailable: " + err.Error())
		for done := range z.off {
			close(done)
		}
		return
	}

	var (
		ticker  *time.Ticker
		tick    <-chan time.Time
		applied = 1.0
		lastX   = math.MinInt32
		lastY   = math.MinInt32
	)
	apply := func() {
		z.mu.Lock()
		f := z.want
		z.mu.Unlock()
		if f <= 1.001 {
			if applied != 1 {
				setTransform(1, 0, 0)
				applied, lastX, lastY = 1, math.MinInt32, math.MinInt32
			}
			if ticker != nil {
				ticker.Stop()
				ticker, tick = nil, nil
			}
			return
		}
		if ticker == nil {
			ticker = time.NewTicker(frame)
			tick = ticker.C
		}
		x, y := offsets(f)
		if f != applied || x != lastX || y != lastY {
			setTransform(f, x, y)
			applied, lastX, lastY = f, x, y
		}
	}

	for {
		select {
		case <-z.wake:
			apply()
		case <-tick:
			apply()
		case done := <-z.off:
			z.mu.Lock()
			z.want = 1
			z.mu.Unlock()
			apply()
			_, _, _ = procMagUninitialize.Call()
			close(done)
			return
		}
	}
}

// The view's top-left in unmagnified desktop coordinates. Moving it by (1 - 1/f) of the
// pointer's distance from the desktop's edge keeps the pointer over the same content, as
// TheeJ does with mid + (pointer - mid)(1 - 1/f), and lets it reach every edge.
func offsets(f float64) (int, int) {
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	left := metric(smXVirtualScreen)
	top := metric(smYVirtualScreen)
	width := metric(smCXVirtualScreen)
	height := metric(smCYVirtualScreen)
	x := float64(left) + float64(int(pt.X)-left)*(1-1/f)
	y := float64(top) + float64(int(pt.Y)-top)*(1-1/f)
	maxX := float64(left) + float64(width)*(1-1/f)
	maxY := float64(top) + float64(height)*(1-1/f)
	return int(math.Round(math.Min(math.Max(x, float64(left)), maxX))),
		int(math.Round(math.Min(math.Max(y, float64(top)), maxY)))
}

func metric(index uintptr) int {
	v, _, _ := procGetSystemMetric.Call(index)
	return int(int32(v))
}

// The float travels as its bit pattern in an integer slot; on amd64 Go's syscall stub also
// copies it into XMM0, where the Windows x64 ABI expects a float argument. On 386 a float
// argument is a 4-byte stack slot like any other, so the bit pattern is the argument itself.
func setTransform(f float64, x, y int) {
	_, _, _ = procMagSetFullscreenTransform.Call(
		uintptr(math.Float32bits(float32(f))),
		uintptr(int32(x)),
		uintptr(int32(y)),
	)
}
