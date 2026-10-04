//go:build windows

package web

// Raw Win32 declarations for this package only (platform.md: no shared bindings
// package). golang.org/x/sys/windows covers kernel/dwm/registry calls but not the
// classic GUI surface (window class, CreateWindowEx, the message loop), so those
// are declared here; DwmSetWindowAttribute is redeclared too, so this package
// never reaches into winui's or another package's syscall wiring.

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")

	procRegisterClassExW      = user32.NewProc("RegisterClassExW")
	procCreateWindowExW       = user32.NewProc("CreateWindowExW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procDestroyWindow         = user32.NewProc("DestroyWindow")
	procShowWindow            = user32.NewProc("ShowWindow")
	procGetMessageW           = user32.NewProc("GetMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessageW      = user32.NewProc("DispatchMessageW")
	procPostQuitMessage       = user32.NewProc("PostQuitMessage")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procLoadCursorW           = user32.NewProc("LoadCursorW")
	procGetDpiForWindow       = user32.NewProc("GetDpiForWindow")
	procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procGetWindowRect         = user32.NewProc("GetWindowRect")
	procGetClientRect         = user32.NewProc("GetClientRect")
	procGetSystemMenu         = user32.NewProc("GetSystemMenu")
	procDeleteMenu            = user32.NewProc("DeleteMenu")
	procSetWindowTextW        = user32.NewProc("SetWindowTextW")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	wsOverlapped  = 0x00000000
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
	wsClipChilden = 0x02000000
	wsVisible     = 0x10000000

	cwUseDefault = 0x80000000 // CW_USEDEFAULT (INT_MIN), written as its unsigned bit pattern so it fits uintptr

	swShow    = 5
	swRestore = 9

	csHRedraw = 0x0002
	csVRedraw = 0x0001

	colorWindowBrush = 6 // COLOR_WINDOW + 1, a stock brush handle, not a GDI object to free

	idcArrow = 32512

	wmSize       = 0x0005
	wmClose      = 0x0010
	wmDestroy    = 0x0002
	wmSysCommand = 0x0112

	scClose     = 0xF060
	mfByCommand = 0x00000000

	spiGetWorkArea = 0x0030

	swpNoZorder   = 0x0004
	swpNoActivate = 0x0010
	swpNoMove     = 0x0002

	dwmwaUseImmersiveDarkMode = 20
	dwmwaSystemBackdropType   = 38
	dwmSBTMainWindow          = 2 // DWMSBT_MAINWINDOW, i.e. Mica
)

type wndClassExW struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

type point32 struct{ X, Y int32 }

type msg32 struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point32
}

type rect32 struct{ Left, Top, Right, Bottom int32 }

func utf16Ptr(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		// s is always a literal in this package; a conversion failure here would
		// mean a NUL byte snuck into one, which is a bug, not a runtime condition.
		panic(err)
	}
	return p
}

var windowClassName = utf16Ptr("WeeJ.Web")

var registerOnce sync.Once

// wndProcPtr must stay reachable for the lifetime of the process: user32 holds this
// address in the registered window class and calls back into it indefinitely.
var wndProcPtr = windows.NewCallback(wndProcDispatch)

func registerWindowClass() {
	registerOnce.Do(func() {
		moduleHandle, _, _ := procGetModuleHandleW.Call(0)
		cursor, _, _ := procLoadCursorW.Call(0, uintptr(idcArrow))
		wc := wndClassExW{
			style:      csHRedraw | csVRedraw,
			wndProc:    wndProcPtr,
			instance:   moduleHandle,
			cursor:     cursor,
			background: colorWindowBrush,
			className:  windowClassName,
		}
		wc.size = uint32(unsafe.Sizeof(wc))
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
}

func createWindowHidden(title string) uintptr {
	registerWindowClass()
	moduleHandle, _, _ := procGetModuleHandleW.Call(0)
	style := uintptr(wsOverlapped | wsCaption | wsSysMenu | wsMinimizeBox | wsClipChilden)
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(windowClassName)),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		style,
		uintptr(cwUseDefault), uintptr(cwUseDefault), 400, 300,
		0, 0, moduleHandle, 0,
	)
	return hwnd
}

func dpiForWindow(hwnd uintptr) float64 {
	r, _, _ := procGetDpiForWindow.Call(hwnd)
	if r == 0 {
		return 1
	}
	return float64(r) / 96.0
}

func windowRect(hwnd uintptr) rect32 {
	var r rect32
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func clientRect(hwnd uintptr) rect32 {
	var r rect32
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func workArea() rect32 {
	var r rect32
	procSystemParametersInfoW.Call(uintptr(spiGetWorkArea), 0, uintptr(unsafe.Pointer(&r)), 0)
	if r.Right <= r.Left || r.Bottom <= r.Top {
		// A zero rect means the call failed; fall back to something sane rather
		// than placing the window at a degenerate (0,0,0,0) origin.
		r = rect32{Left: 0, Top: 0, Right: 1920, Bottom: 1080}
	}
	return r
}

func setWindowPos(hwnd uintptr, x, y, w, h int32, flags uintptr) {
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), flags)
}

func showWindow(hwnd uintptr, cmd int32) {
	procShowWindow.Call(hwnd, uintptr(cmd))
}

func destroyWindow(hwnd uintptr) {
	procDestroyWindow.Call(hwnd)
}

func setForegroundWindow(hwnd uintptr) {
	procSetForegroundWindow.Call(hwnd)
}

func setWindowText(hwnd uintptr, s string) {
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(s))))
}

// disableCloseBox removes SC_CLOSE from the system menu, which is also what makes
// Windows grey out the title bar's own X button; wndProcDispatch still swallows
// WM_CLOSE/WM_SYSCOMMAND defensively since Alt+F4 does not consult this menu.
func disableCloseBox(hwnd uintptr) {
	menu, _, _ := procGetSystemMenu.Call(hwnd, 0)
	if menu == 0 {
		return
	}
	procDeleteMenu.Call(menu, uintptr(scClose), uintptr(mfByCommand))
}

func setDarkTitleBar(hwnd uintptr, dark bool) {
	var v int32
	if dark {
		v = 1
	}
	procDwmSetWindowAttribute.Call(hwnd, uintptr(dwmwaUseImmersiveDarkMode), uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
}

func setMica(hwnd uintptr) {
	v := int32(dwmSBTMainWindow)
	// Ignored by DWM on Windows 10, which has no Mica; nothing to fall back to.
	procDwmSetWindowAttribute.Call(hwnd, uintptr(dwmwaSystemBackdropType), uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
}

// pumpOne runs one GetMessage/Translate/Dispatch cycle. It reports whether a
// WM_QUIT was seen (ok=false), so callers (Open's modal loop) can repost it and
// stop rather than swallowing the whole application's shutdown signal.
func pumpOne() (ok bool) {
	var m msg32
	r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
	if int32(r) <= 0 {
		return false
	}
	procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
	procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	return true
}

func postQuitMessage() {
	procPostQuitMessage.Call(0)
}

var (
	liveMu sync.Mutex
	live   = map[uintptr]*Window{}
)

func registerLive(hwnd uintptr, w *Window) {
	liveMu.Lock()
	live[hwnd] = w
	liveMu.Unlock()
}

func unregisterLive(hwnd uintptr) {
	liveMu.Lock()
	delete(live, hwnd)
	liveMu.Unlock()
}

func windowFor(hwnd uintptr) *Window {
	liveMu.Lock()
	w := live[hwnd]
	liveMu.Unlock()
	return w
}

func wndProcDispatch(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	w := windowFor(hwnd)
	switch message {
	case wmSize:
		if w != nil && w.chromium != nil {
			w.chromium.Resize()
		}
		return 0
	case wmClose:
		if w != nil && w.opts.NoClose {
			return 0
		}
		destroyWindow(hwnd)
		return 0
	case wmSysCommand:
		if w != nil && w.opts.NoClose && wparam&0xFFF0 == scClose {
			return 0
		}
	case wmDestroy:
		unregisterLive(hwnd)
		if w != nil {
			w.handleDestroyed()
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wparam, lparam)
	return r
}
