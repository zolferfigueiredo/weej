//go:build windows

package sys

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const MainWindowClass = "WeeJ.Main"

const mutexName = `Local\com.zolfer.weej`

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW            = user32.NewProc("FindWindowW")
	procRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")
	procPostMessageW           = user32.NewProc("PostMessageW")
)

// The mutex only exists while a handle stays open, so keeping release until exit is what lets a
// second launch see ERROR_ALREADY_EXISTS. Not owned: goroutines move between threads.
func Acquire() (release func(), ok bool) {
	namePtr, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return nil, false
	}
	handle, err := windows.CreateMutex(nil, false, namePtr)
	if err == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(handle)
		return nil, false
	}
	if err != nil {
		return nil, false
	}
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		_ = windows.CloseHandle(handle)
	}
	return release, true
}

// WaitForRelease polls for the mutex every 100ms, the way --quit confirms the running copy
// actually exited: once Acquire succeeds, the mutex was free, so release it right back.
func WaitForRelease(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if release, ok := Acquire(); ok {
			release()
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func Notify(msg string) bool {
	classPtr, err := windows.UTF16PtrFromString(MainWindowClass)
	if err != nil {
		return false
	}
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(classPtr)), 0)
	if hwnd == 0 {
		return false
	}
	namePtr, err := windows.UTF16PtrFromString("WeeJ." + msg)
	if err != nil {
		return false
	}
	registered, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(namePtr)))
	if registered == 0 {
		return false
	}
	ret, _, _ := procPostMessageW.Call(hwnd, registered, 0, 0)
	return ret != 0
}
