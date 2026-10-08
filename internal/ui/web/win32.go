//go:build windows

package web

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procShowWindow             = user32.NewProc("ShowWindow")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procLoadCursorW            = user32.NewProc("LoadCursorW")
	procGetDpiForWindow        = user32.NewProc("GetDpiForWindow")
	procSystemParametersInfoW  = user32.NewProc("SystemParametersInfoW")
	procSetWindowPos           = user32.NewProc("SetWindowPos")
	procGetWindowRect          = user32.NewProc("GetWindowRect")
	procGetClientRect          = user32.NewProc("GetClientRect")
	procGetSystemMenu          = user32.NewProc("GetSystemMenu")
	procDeleteMenu             = user32.NewProc("DeleteMenu")
	procSetWindowTextW         = user32.NewProc("SetWindowTextW")
	procSetTimer               = user32.NewProc("SetTimer")
	procKillTimer              = user32.NewProc("KillTimer")
	procFillRect               = user32.NewProc("FillRect")
	procInvalidateRect         = user32.NewProc("InvalidateRect")
	procLoadImageW             = user32.NewProc("LoadImageW")
	procGetSystemMetricsForDpi = user32.NewProc("GetSystemMetricsForDpi")
	procSendMessageW           = user32.NewProc("SendMessageW")
	procPostMessageW           = user32.NewProc("PostMessageW")
	procClientToScreen         = user32.NewProc("ClientToScreen")
	procMonitorFromWindow      = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW        = user32.NewProc("GetMonitorInfoW")
	procIsZoomed               = user32.NewProc("IsZoomed")

	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject     = gdi32.NewProc("DeleteObject")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")

	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	wsOverlapped  = 0x00000000
	wsPopup       = 0x80000000
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
	wsMaximizeBox = 0x00010000
	wsThickFrame  = 0x00040000
	wsClipChilden = 0x02000000
	wsVisible     = 0x10000000

	cwUseDefault = 0x80000000 // CW_USEDEFAULT (INT_MIN), written as its unsigned bit pattern so it fits uintptr

	wsExToolWindow = 0x00000080 // keeps a popup off the taskbar and out of Alt+Tab

	swHide    = 0
	swShow    = 5
	swRestore = 9

	csHRedraw     = 0x0002
	csVRedraw     = 0x0001
	csDropShadow  = 0x00020000
	monitorNearer = 2 // MONITOR_DEFAULTTONEAREST

	idcArrow = 32512

	wmMove       = 0x0003
	wmSize       = 0x0005
	wmActivate   = 0x0006
	wmSetFocus   = 0x0007
	wmClose      = 0x0010
	wmDestroy    = 0x0002
	wmEraseBkgnd = 0x0014
	wmTimer      = 0x0113
	wmSysCommand = 0x0112
	wmDpiChanged = 0x02E0
	wmSetIcon    = 0x0080
	wmSizing     = 0x0214
	wmMinMaxInfo = 0x0024
	wmAppHide    = 0x8001 // WM_APP+1: hide a popup once its activation change is done

	iconSmall  = 0
	iconBig    = 1
	imageIcon  = 1
	smCxIcon   = 11
	smCxSmIcon = 49

	waInactive = 0

	scClose     = 0xF060
	mfByCommand = 0x00000000

	spiGetWorkArea = 0x0030

	swpNoZorder   = 0x0004
	swpNoActivate = 0x0010
	swpNoMove     = 0x0002

	dwmwaUseImmersiveDarkMode = 20
	dwmwaCornerPreference     = 33
	dwmwcpRound               = 2
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

var (
	windowClassName = utf16Ptr("WeeJ.Web")
	popupClassName  = utf16Ptr("WeeJ.WebPopup")
)

var registerOnce, registerPopupOnce sync.Once

// wndProcPtr must stay reachable for the lifetime of the process: user32 holds this
// address in the registered window class and calls back into it indefinitely.
var wndProcPtr = windows.NewCallback(wndProcDispatch)

func registerWindowClass() {
	registerOnce.Do(func() {
		moduleHandle, _, _ := procGetModuleHandleW.Call(0)
		cursor, _, _ := procLoadCursorW.Call(0, uintptr(idcArrow))
		wc := wndClassExW{
			style:     csHRedraw | csVRedraw,
			wndProc:   wndProcPtr,
			instance:  moduleHandle,
			cursor:    cursor,
			className: windowClassName,
		}
		wc.size = uint32(unsafe.Sizeof(wc))
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
}

// A popup's class adds the drop shadow menus have; Windows 11 rounds it with the corners.
func registerPopupClass() {
	registerPopupOnce.Do(func() {
		moduleHandle, _, _ := procGetModuleHandleW.Call(0)
		cursor, _, _ := procLoadCursorW.Call(0, uintptr(idcArrow))
		wc := wndClassExW{
			style:     csHRedraw | csVRedraw | csDropShadow,
			wndProc:   wndProcPtr,
			instance:  moduleHandle,
			cursor:    cursor,
			className: popupClassName,
		}
		wc.size = uint32(unsafe.Sizeof(wc))
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
}

func createPopupHidden(owner uintptr, x, y int32) uintptr {
	registerPopupClass()
	moduleHandle, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, _ := procCreateWindowExW.Call(
		wsExToolWindow,
		uintptr(unsafe.Pointer(popupClassName)),
		uintptr(unsafe.Pointer(utf16Ptr("WeeJ"))),
		uintptr(wsPopup|wsClipChilden),
		uintptr(x), uintptr(y), 1, 1,
		owner, 0, moduleHandle, 0,
	)
	if hwnd != 0 {
		v := int32(dwmwcpRound)
		procDwmSetWindowAttribute.Call(hwnd, uintptr(dwmwaCornerPreference), uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	}
	return hwnd
}

func createWindowHidden(title string, resizable bool) uintptr {
	registerWindowClass()
	moduleHandle, _, _ := procGetModuleHandleW.Call(0)
	style := uintptr(wsOverlapped | wsCaption | wsSysMenu | wsMinimizeBox | wsClipChilden)
	if resizable {
		style |= wsThickFrame | wsMaximizeBox
	}
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(windowClassName)),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		style,
		uintptr(cwUseDefault), uintptr(cwUseDefault), 400, 300,
		0, 0, moduleHandle, 0,
	)
	if hwnd != 0 {
		dpi, _, _ := procGetDpiForWindow.Call(hwnd)
		setWindowIcons(hwnd, uint32(dpi))
	}
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

func clientOrigin(hwnd uintptr) point32 {
	var p point32
	procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&p)))
	return p
}

// monitorWorkArea is the work area of the monitor hwnd is on, which a popup must fit in;
// workArea is the primary monitor's.
func monitorWorkArea(hwnd uintptr) rect32 {
	var info struct {
		size    uint32
		monitor rect32
		work    rect32
		flags   uint32
	}
	info.size = uint32(unsafe.Sizeof(info))
	mon, _, _ := procMonitorFromWindow.Call(hwnd, monitorNearer)
	if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&info))); r == 0 {
		return workArea()
	}
	return info.work
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

func setTimer(hwnd uintptr, id, ms uintptr) {
	procSetTimer.Call(hwnd, id, ms, 0)
}

func killTimer(hwnd uintptr, id uintptr) {
	procKillTimer.Call(hwnd, id)
}

func createSolidBrush(r, g, b uint8) uintptr {
	h, _, _ := procCreateSolidBrush.Call(uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16)
	return h
}

func deleteObject(h uintptr) {
	procDeleteObject.Call(h)
}

func invalidate(hwnd uintptr) {
	procInvalidateRect.Call(hwnd, 0, 1)
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
	case wmEraseBkgnd:
		if w != nil && w.brush != 0 {
			r := clientRect(hwnd)
			procFillRect.Call(wparam, uintptr(unsafe.Pointer(&r)), w.brush)
			return 1
		}
	case wmSize:
		if w != nil {
			w.resizeWebView()
		}
		return 0
	case wmSizing:
		if w != nil {
			w.userSized = true
		}
	case wmMinMaxInfo:
		if w != nil && w.opts.Resizable {
			// MINMAXINFO.ptMinTrackSize follows three POINTs. It can't be dragged shorter
			// than the page, so nothing on it is cut off.
			minH := max(dipToPx(minResizeH, w.scale), w.neededPx)
			minSize := [2]int32{dipToPx(minResizeW, w.scale) + w.frameW, minH + w.frameH}
			procRtlMoveMemory.Call(lparam+24, uintptr(unsafe.Pointer(&minSize)), unsafe.Sizeof(minSize))
			return 0
		}
	case wmMove:
		if w != nil && w.chromium != nil && !w.isClosed() {
			_ = w.chromium.NotifyParentWindowPositionChanged()
		}
	case wmActivate:
		if w != nil && wparam&0xFFFF != waInactive {
			w.focusWebView()
		}
		// A popup goes away the moment another window takes focus, as a menu does. Posted,
		// not hidden here, so the activation change finishes first.
		if w != nil && w.popup && w.visible && wparam&0xFFFF == waInactive {
			procPostMessageW.Call(hwnd, wmAppHide, 0, 0)
		}
	case wmAppHide:
		if w != nil && w.popup {
			w.hidePopup()
		}
		return 0
	case wmSetFocus:
		if w != nil {
			w.focusWebView()
		}
		return 0
	case wmTimer:
		if w != nil && wparam == showFallbackTimer {
			w.showOnce()
		}
		return 0
	case wmDpiChanged:
		if w != nil {
			// lparam points at a RECT owned by Windows; copy it rather than convert the address.
			var suggested rect32
			procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&suggested)), lparam, unsafe.Sizeof(suggested))
			w.scale = float64(wparam&0xFFFF) / 96.0
			setWindowIcons(hwnd, uint32(wparam&0xFFFF))
			setWindowPos(hwnd, suggested.Left, suggested.Top, suggested.Right-suggested.Left, suggested.Bottom-suggested.Top, swpNoZorder|swpNoActivate)
			return 0
		}
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

var (
	iconMu    sync.Mutex
	iconCache = map[uintptr]uintptr{}
)

// The exe's own icon group (named APP in winres.json), loaded at the exact pixel size a DPI
// needs. Every window shares these, so they live for the whole process.
func appIcon(size uintptr) uintptr {
	iconMu.Lock()
	defer iconMu.Unlock()
	if h, ok := iconCache[size]; ok {
		return h
	}
	module, _, _ := procGetModuleHandleW.Call(0)
	h, _, _ := procLoadImageW.Call(module, uintptr(unsafe.Pointer(utf16Ptr("APP"))), imageIcon, size, size, 0)
	if h != 0 {
		iconCache[size] = h
	}
	return h
}

func setWindowIcons(hwnd uintptr, dpi uint32) {
	if dpi == 0 {
		dpi = 96
	}
	small, _, _ := procGetSystemMetricsForDpi.Call(smCxSmIcon, uintptr(dpi))
	big, _, _ := procGetSystemMetricsForDpi.Call(smCxIcon, uintptr(dpi))
	if h := appIcon(small); h != 0 {
		procSendMessageW.Call(hwnd, wmSetIcon, iconSmall, h)
	}
	if h := appIcon(big); h != 0 {
		procSendMessageW.Call(hwnd, wmSetIcon, iconBig, h)
	}
}
