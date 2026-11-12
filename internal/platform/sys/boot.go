//go:build windows

package sys

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetTickCount64 = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount64")

// BootTime is when Windows last started.
func BootTime() time.Time {
	r1, r2, _ := procGetTickCount64.Call()
	ms := uint64(r1)
	// On 386 the 64-bit result comes back split, its high half in r2.
	if unsafe.Sizeof(uintptr(0)) == 4 {
		ms |= uint64(r2) << 32
	}
	return time.Now().Add(-time.Duration(ms) * time.Millisecond)
}
