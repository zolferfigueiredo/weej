//go:build windows

package app

import (
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
	if !strings.HasPrefix(strings.ToLower(rawURL), "https://") {
		return
	}
	op, _ := windows.UTF16PtrFromString("open")
	u, err := windows.UTF16PtrFromString(rawURL)
	if err != nil {
		return
	}
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(op)), uintptr(unsafe.Pointer(u)), 0, 0, swShowNormal)
}
