//go:build windows

package app

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32           = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
)

const swShowNormal = 1

func openURL(rawURL string) {
	lower := strings.ToLower(rawURL)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		return
	}
	op, _ := windows.UTF16PtrFromString("open")
	u, err := windows.UTF16PtrFromString(rawURL)
	if err != nil {
		return
	}
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(op)), uintptr(unsafe.Pointer(u)), 0, 0, swShowNormal)
}

func launch(path string) {
	op, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(path))
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(op)), uintptr(unsafe.Pointer(file)), 0, uintptr(unsafe.Pointer(dir)), swShowNormal)
}
