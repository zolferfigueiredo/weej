//go:build windows

package audio

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca/pkg/wca"
)

// Building the notifier is what panicked in the 32-bit build. The test also registers it
// with Windows, as Audio does, and calls it the way Windows does.
func TestDeviceNotifier(t *testing.T) {
	changed := false
	n := newDeviceNotifier(func() { changed = true })
	this := uintptr(unsafe.Pointer(n))

	var got uintptr
	hr, _, _ := syscall.SyscallN(n.vtbl.queryInterface, this, uintptr(unsafe.Pointer(wca.IID_IMMNotificationClient)), uintptr(unsafe.Pointer(&got)))
	if hr != sOK || got != this {
		t.Fatalf("QueryInterface(IMMNotificationClient) = %#x, %#x; want S_OK and the notifier", hr, got)
	}
	hr, _, _ = syscall.SyscallN(n.vtbl.queryInterface, this, uintptr(unsafe.Pointer(wca.IID_IMMDeviceEnumerator)), uintptr(unsafe.Pointer(&got)))
	if hr != eNoInterface || got != 0 {
		t.Errorf("QueryInterface(IMMDeviceEnumerator) = %#x, %#x; want E_NOINTERFACE", hr, got)
	}

	hr, _, _ = syscall.SyscallN(n.vtbl.onDefaultDeviceChanged, this, wca.ERender, wca.EConsole, 0)
	if hr != sOK || !changed {
		t.Errorf("OnDefaultDeviceChanged = %#x, reached the callback: %v; want S_OK and true", hr, changed)
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil && !benignCoInitError(err) {
		t.Fatal(err)
	}
	defer ole.CoUninitialize()
	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		t.Skip("no audio device enumerator: " + err.Error())
	}
	defer enumerator.Release()
	if err := enumerator.RegisterEndpointNotificationCallback((*wca.IMMNotificationClient)(unsafe.Pointer(n))); err != nil {
		t.Fatal(err)
	}
	hr, _, _ = syscall.SyscallN(enumerator.VTable().UnregisterEndpointNotificationCallback, uintptr(unsafe.Pointer(enumerator)), this)
	if hr != sOK {
		t.Errorf("UnregisterEndpointNotificationCallback = %#x, want S_OK", hr)
	}
}
