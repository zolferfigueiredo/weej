//go:build windows

package winui

import (
	"image"
	"math"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")

	hudClassOnce sync.Once
)

const (
	hudClassName = "WeeJ.HUD"

	wsPopup         = 0x80000000
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExNoActivate  = 0x08000000
	wsExTopMost     = 0x00000008

	ulwAlpha = 0x00000002

	hudHoldDuration  = 1 * time.Second
	hudFadeSteps     = 10
	hudFadeStepEvery = 300 * time.Millisecond / hudFadeSteps
)

// sizeT mirrors SIZE.
type sizeT struct {
	cx int32
	cy int32
}

// blendFunction mirrors BLENDFUNCTION; all-byte fields need no manual padding.
type blendFunction struct {
	blendOp             byte
	blendFlags          byte
	sourceConstantAlpha byte
	alphaFormat         byte
}

type hudWindow struct {
	hwnd    windows.HWND
	memDC   windows.Handle
	hbitmap windows.Handle
	x, y    int
	w, h    int
	gen     uint64
	stop    func()
}

type HUD struct {
	loop *Loop

	mu   sync.Mutex
	wins map[string]*hudWindow
}

func (l *Loop) NewHUD() *HUD {
	return &HUD{loop: l, wins: make(map[string]*hudWindow)}
}

// Show paints the caller's pre-rendered bitmap at the bottom-centre of work, then restarts the
// hold-and-fade timer for this monitor's HUD window, creating it on first use.
func (h *HUD) Show(key string, work image.Rectangle, scale float64, bmp *image.NRGBA) {
	if bmp == nil || bmp.Rect.Dx() <= 0 || bmp.Rect.Dy() <= 0 {
		return
	}

	h.mu.Lock()
	hw, ok := h.wins[key]
	if !ok {
		hw = newHUDWindow()
		h.wins[key] = hw
	}
	if hw.stop != nil {
		hw.stop()
		hw.stop = nil
	}
	hw.gen++
	gen := hw.gen
	h.mu.Unlock()

	w, hgt := bmp.Rect.Dx(), bmp.Rect.Dy()
	x := work.Min.X + (work.Dx()-w)/2
	y := work.Max.Y - int(math.Round(24*scale)) - hgt

	renderHUD(hw, bmp, x, y)
	showWindow(hw.hwnd, swShowNoActivate)

	stop := h.loop.After(hudHoldDuration, func() {
		h.fadeStep(hw, gen, 0)
	})
	h.mu.Lock()
	hw.stop = stop
	h.mu.Unlock()
}

func (h *HUD) fadeStep(hw *hudWindow, gen uint64, step int) {
	h.mu.Lock()
	current := hw.gen
	h.mu.Unlock()
	if current != gen {
		return // superseded by a later Show
	}

	if step >= hudFadeSteps {
		showWindow(hw.hwnd, swHide)
		return
	}

	alpha := byte(255 - (255*(step+1))/hudFadeSteps)
	updateLayeredWindow(hw.hwnd, hw.x, hw.y, hw.w, hw.h, hw.memDC, alpha)

	stop := h.loop.After(hudFadeStepEvery, func() {
		h.fadeStep(hw, gen, step+1)
	})
	h.mu.Lock()
	hw.stop = stop
	h.mu.Unlock()
}

func newHUDWindow() *hudWindow {
	ensureHUDClass()
	className := mustUTF16PtrFromString(hudClassName)
	exStyle := uint32(wsExLayered | wsExTransparent | wsExToolWindow | wsExNoActivate | wsExTopMost)
	hwnd, _ := createWindowExW(exStyle, className, className, wsPopup, 0, 0, 0, 0, 0, 0, getModuleHandle())
	return &hudWindow{hwnd: hwnd}
}

func ensureHUDClass() {
	hudClassOnce.Do(func() {
		wc := wndClassExW{
			size:      uint32(unsafe.Sizeof(wndClassExW{})),
			wndProc:   windows.NewCallback(hudWndProc),
			instance:  getModuleHandle(),
			cursor:    loadCursorArrow(),
			className: mustUTF16PtrFromString(hudClassName),
		}
		registerClassExW(&wc)
	})
}

func hudWndProc(hwnd windows.HWND, msg uint32, wparam, lparam uintptr) uintptr {
	return defWindowProcW(hwnd, msg, wparam, lparam)
}

// renderHUD replaces the window's DIB content and paints it at full opacity; the DC and bitmap
// are kept selected afterwards so later fade steps can repaint with only the blend alpha
// changed, without rebuilding the bitmap.
func renderHUD(hw *hudWindow, img *image.NRGBA, x, y int) {
	w, hgt := img.Rect.Dx(), img.Rect.Dy()
	if hw.memDC == 0 {
		hw.memDC = createCompatibleDC(0)
	}
	newBmp, bits := createDIB32(w, hgt, true)
	writeDIBPixels(bits, img, true)
	old := selectObject(hw.memDC, newBmp)
	if hw.hbitmap != 0 {
		deleteObject(old)
	}
	hw.hbitmap = newBmp
	hw.x, hw.y, hw.w, hw.h = x, y, w, hgt

	updateLayeredWindow(hw.hwnd, x, y, w, hgt, hw.memDC, 255)
}

func updateLayeredWindow(hwnd windows.HWND, x, y, w, h int, hdcSrc windows.Handle, alpha byte) {
	hdcScreen := getDC(0)
	defer releaseDC(0, hdcScreen)

	ptDst := pointT{int32(x), int32(y)}
	size := sizeT{int32(w), int32(h)}
	ptSrc := pointT{0, 0}
	blend := blendFunction{blendOp: 0, sourceConstantAlpha: alpha, alphaFormat: 1}

	procUpdateLayeredWindow.Call(
		uintptr(hwnd), uintptr(hdcScreen),
		uintptr(unsafe.Pointer(&ptDst)), uintptr(unsafe.Pointer(&size)),
		uintptr(hdcSrc), uintptr(unsafe.Pointer(&ptSrc)),
		0, uintptr(unsafe.Pointer(&blend)), uintptr(ulwAlpha),
	)
}
