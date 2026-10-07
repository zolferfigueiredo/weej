//go:build windows && 386

package audio

import (
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca/pkg/wca"
)

// initLoopback calls IAudioClient::Initialize for a shared loopback stream. On 32-bit Windows each
// 64-bit REFERENCE_TIME takes two argument slots, low half first, which go-wca's own Initialize
// doesn't do.
func initLoopback(ac *wca.IAudioClient, wfx *wca.WAVEFORMATEX) error {
	hr, _, _ := syscall.SyscallN(ac.VTable().Initialize, uintptr(unsafe.Pointer(ac)),
		wca.AUDCLNT_SHAREMODE_SHARED, wca.AUDCLNT_STREAMFLAGS_LOOPBACK, loopbackBuffer, 0, 0, 0,
		uintptr(unsafe.Pointer(wfx)), 0)
	if hr != 0 {
		return ole.NewError(hr)
	}
	return nil
}
