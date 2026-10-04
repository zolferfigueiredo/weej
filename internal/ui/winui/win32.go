//go:build windows

package winui

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// DLL handles shared across the package's files; each file declares the procs it needs from
// these with its own NewProc calls.
var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	procRegisterClassExW      = user32.NewProc("RegisterClassExW")
	procCreateWindowExW       = user32.NewProc("CreateWindowExW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procShowWindow            = user32.NewProc("ShowWindow")
	procGetMessageW           = user32.NewProc("GetMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessageW      = user32.NewProc("DispatchMessageW")
	procPostQuitMessage       = user32.NewProc("PostQuitMessage")
	procPostMessageW          = user32.NewProc("PostMessageW")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procLoadCursorW           = user32.NewProc("LoadCursorW")
	procSetTimer              = user32.NewProc("SetTimer")
	procKillTimer             = user32.NewProc("KillTimer")
	procRegisterWindowMessage = user32.NewProc("RegisterWindowMessageW")
	procGetCursorPos          = user32.NewProc("GetCursorPos")
	procMonitorFromPoint      = user32.NewProc("MonitorFromPoint")
	procGetSystemMetricsForDp = user32.NewProc("GetSystemMetricsForDpi")
	procGetDC                 = user32.NewProc("GetDC")
	procReleaseDC             = user32.NewProc("ReleaseDC")

	procGetDpiForMonitor = shcore.NewProc("GetDpiForMonitor")

	procLstrlenW      = kernel32.NewProc("lstrlenW")
	procRtlMoveMemory = kernel32.NewProc("RtlMoveMemory")
)

// Window messages, styles, and other constants the spikes already proved against the real
// shell; kept together here since several files in the package share them.
const (
	wmDestroy         = 0x0002
	wmSettingChange   = 0x001A
	wmDisplayChange   = 0x007E
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmHotkey          = 0x0312
	wmTimer           = 0x0113
	wmApp             = 0x8000
	wmUser            = 0x0400
	wmNull            = 0x0000
	wmContextMenu     = 0x007B
	wmLButtonUp       = 0x0202

	msgInvoke = wmApp + 1 // the first UI-thread message id this package reserves for itself

	wsExToolWindow = 0x00000080

	swHide           = 0
	swShowNoActivate = 4

	idcArrow = 32512

	smCxSmIcon              = 49
	smCxMenuCheck           = 71
	monitorDefaultToPrimary = 1
	monitorDefaultToNearest = 2
	mdtEffectiveDpi         = 0
)

// wndClassExW mirrors WNDCLASSEXW (user32.dll RegisterClassExW); the field order and types are
// load-bearing since this is handed to the OS by raw pointer.
type wndClassExW struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	iconSm     windows.Handle
}

// msgT mirrors MSG.
type msgT struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      pointT
}

// pointT mirrors POINT.
type pointT struct {
	X int32
	Y int32
}

func mustUTF16PtrFromString(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		// s came from this package's own string literals or caller text; a conversion failure
		// here means an embedded NUL, which we treat as a bug rather than a runtime condition.
		panic(err)
	}
	return p
}

func setUTF16(dst []uint16, s string) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		u = []uint16{0}
	}
	n := len(u)
	if n > len(dst) {
		n = len(dst)
		dst[n-1] = 0
	}
	copy(dst, u[:n])
}

func getModuleHandle() windows.Handle {
	r, _, _ := procGetModuleHandleW.Call(0)
	return windows.Handle(r)
}

func registerClassExW(wc *wndClassExW) (uint16, error) {
	r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(wc)))
	if r == 0 {
		return 0, err
	}
	return uint16(r), nil
}

func createWindowExW(exStyle uint32, className, windowName *uint16, style uint32, x, y, w, h int32, parent, menu, instance windows.Handle) (windows.HWND, error) {
	r, _, err := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(parent), uintptr(menu), uintptr(instance), 0,
	)
	if r == 0 {
		return 0, err
	}
	return windows.HWND(r), nil
}

func defWindowProcW(hwnd windows.HWND, msg uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
	return r
}

func showWindow(hwnd windows.HWND, cmd int32) {
	procShowWindow.Call(uintptr(hwnd), uintptr(cmd))
}

// getMessageW returns 0 on WM_QUIT, -1 on error, nonzero otherwise.
func getMessageW(msg *msgT) int32 {
	r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(msg)), 0, 0, 0)
	return int32(r)
}

func translateMessage(msg *msgT) {
	procTranslateMessage.Call(uintptr(unsafe.Pointer(msg)))
}

func dispatchMessageW(msg *msgT) {
	procDispatchMessageW.Call(uintptr(unsafe.Pointer(msg)))
}

func postQuitMessageW(code int32) {
	procPostQuitMessage.Call(uintptr(code))
}

func postMessageW(hwnd windows.HWND, msg uint32, wparam, lparam uintptr) {
	procPostMessageW.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
}

func setForegroundWindow(hwnd windows.HWND) {
	procSetForegroundWindow.Call(uintptr(hwnd))
}

func loadCursorArrow() windows.Handle {
	r, _, _ := procLoadCursorW.Call(0, uintptr(idcArrow))
	return windows.Handle(r)
}

func setTimer(hwnd windows.HWND, id uintptr, elapseMs uint32) {
	procSetTimer.Call(uintptr(hwnd), id, uintptr(elapseMs), 0)
}

func killTimer(hwnd windows.HWND, id uintptr) {
	procKillTimer.Call(uintptr(hwnd), id)
}

func registerWindowMessageW(name string) uint32 {
	r, _, _ := procRegisterWindowMessage.Call(uintptr(unsafe.Pointer(mustUTF16PtrFromString(name))))
	return uint32(r)
}

func getCursorPos() pointT {
	var pt pointT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt
}

func getSystemMetricsForDpi(index int32, dpi uint32) int32 {
	r, _, _ := procGetSystemMetricsForDp.Call(uintptr(index), uintptr(dpi))
	return int32(r)
}

func getDC(hwnd windows.HWND) windows.Handle {
	r, _, _ := procGetDC.Call(uintptr(hwnd))
	return windows.Handle(r)
}

func releaseDC(hwnd windows.HWND, hdc windows.Handle) {
	procReleaseDC.Call(uintptr(hwnd), uintptr(hdc))
}

// packPoint folds a POINT into the single register the x64 calling convention passes an 8-byte
// struct in, for APIs like MonitorFromPoint that take POINT by value rather than by pointer.
func packPoint(x, y int32) uintptr {
	return uintptr(uint32(x)) | uintptr(uint32(y))<<32
}

func monitorFromPoint(x, y int32, flags uint32) windows.Handle {
	r, _, _ := procMonitorFromPoint.Call(packPoint(x, y), uintptr(flags))
	return windows.Handle(r)
}

func getDpiForMonitor(hmon windows.Handle) uint32 {
	var dpiX, dpiY uint32
	ret, _, _ := procGetDpiForMonitor.Call(uintptr(hmon), uintptr(mdtEffectiveDpi), uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY)))
	if ret != 0 || dpiX == 0 {
		return 96
	}
	return dpiX
}

func dpiAtPoint(x, y int32, flags uint32) uint32 {
	hmon := monitorFromPoint(x, y, flags)
	if hmon == 0 {
		return 96
	}
	return getDpiForMonitor(hmon)
}

// dpiPrimary is used where the spec calls for "the dpi of the primary monitor's taskbar"
// (tray icon sizing): the primary monitor regardless of where the mouse happens to be.
func dpiPrimary() uint32 {
	return dpiAtPoint(0, 0, monitorDefaultToPrimary)
}

// dpiForPoint is used for popup menus, which render on whichever monitor the cursor is on.
func dpiForPoint(pt pointT) uint32 {
	return dpiAtPoint(pt.X, pt.Y, monitorDefaultToNearest)
}

// readForeignUTF16 copies a NUL-terminated UTF-16 string out of a raw address such as a
// message's lParam. lstrlenW and RtlMoveMemory read that address directly inside the syscall, so
// this never forms a Go pointer to memory Go did not allocate (the only unsafe.Pointer taken
// here is of the destination buffer, which is ours).
func readForeignUTF16(addr uintptr) string {
	if addr == 0 {
		return ""
	}
	n, _, _ := procLstrlenW.Call(addr)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&buf[0])), addr, n*2)
	return windows.UTF16ToString(buf)
}
