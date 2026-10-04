//go:build windows

package winui

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	dwmwaUseImmersiveDarkMode = 20
	dwmwaSystemBackdropType   = 38
	dwmSBTMainWindow          = 2 // DWMSBT_MAINWINDOW (Mica)
)

func SetDarkTitleBar(hwnd windows.HWND, dark bool) {
	var v int32
	if dark {
		v = 1
	}
	windows.DwmSetWindowAttribute(hwnd, dwmwaUseImmersiveDarkMode, unsafe.Pointer(&v), uint32(unsafe.Sizeof(v)))
}

// SetMica returns no error: DwmSetWindowAttribute fails for this attribute on Windows 10, where
// the backdrop simply does not apply, which is the documented and expected outcome.
func SetMica(hwnd windows.HWND) {
	v := int32(dwmSBTMainWindow)
	windows.DwmSetWindowAttribute(hwnd, dwmwaSystemBackdropType, unsafe.Pointer(&v), uint32(unsafe.Sizeof(v)))
}
