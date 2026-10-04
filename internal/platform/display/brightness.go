//go:build windows

package display

import (
	"runtime"
	"sync"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// sFalse is the HRESULT CoInitializeEx returns when COM is already initialized on this thread
// with a compatible concurrency model; that's fine, not an error.
const sFalse = 0x1

// builtin owns the WMI connection for WmiMonitorBrightnessMethods on its own goroutine, MTA COM
// locked to that thread. Desktops without a built-in panel have no instances; that's normal.
type builtin struct {
	startOnce sync.Once

	mu      sync.Mutex
	present bool
	pending int
	have    bool
	wake    chan struct{}
}

var builtinInst builtin

func HasBuiltinBrightness() bool {
	builtinInst.start()
	builtinInst.mu.Lock()
	defer builtinInst.mu.Unlock()
	return builtinInst.present
}

func SetBuiltinBrightness(percent int) {
	builtinInst.start()
	builtinInst.mu.Lock()
	builtinInst.pending = percent
	builtinInst.have = true
	builtinInst.mu.Unlock()
	select {
	case builtinInst.wake <- struct{}{}:
	default:
	}
}

func (b *builtin) start() {
	b.startOnce.Do(func() {
		b.wake = make(chan struct{}, 1)
		ready := make(chan struct{})
		go b.run(ready)
		<-ready
	})
}

func (b *builtin) run(ready chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		if oleErr, ok := err.(*ole.OleError); !ok || oleErr.Code() != sFalse {
			close(ready)
			return
		}
	}
	defer ole.CoUninitialize()

	service, ok := connectWMI()
	if !ok {
		close(ready)
		return
	}
	defer service.Release()

	b.mu.Lock()
	b.present = hasBrightnessInstances(service)
	b.mu.Unlock()
	close(ready)

	for range b.wake {
		b.mu.Lock()
		percent, have := b.pending, b.have
		b.have = false
		b.mu.Unlock()
		if have {
			setBrightnessInstances(service, percent)
		}
	}
}

func connectWMI() (*ole.IDispatch, bool) {
	locatorUnknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, false
	}
	locator, err := locatorUnknown.QueryInterface(ole.IID_IDispatch)
	locatorUnknown.Release()
	if err != nil {
		return nil, false
	}
	defer locator.Release()

	serviceVariant, err := oleutil.CallMethod(locator, "ConnectServer", ".", `root\WMI`)
	if err != nil {
		return nil, false
	}
	service := serviceVariant.ToIDispatch()
	if service == nil {
		return nil, false
	}
	return service, true
}

func execBrightnessMethods(service *ole.IDispatch) (*ole.IDispatch, bool) {
	resultVariant, err := oleutil.CallMethod(service, "ExecQuery", "SELECT * FROM WmiMonitorBrightnessMethods")
	if err != nil {
		return nil, false
	}
	result := resultVariant.ToIDispatch()
	if result == nil {
		return nil, false
	}
	return result, true
}

func hasBrightnessInstances(service *ole.IDispatch) bool {
	result, ok := execBrightnessMethods(service)
	if !ok {
		return false
	}
	defer result.Release()

	found := false
	// A ForEach error means the collection could not be enumerated at all, i.e. no usable
	// instances; the callback itself never returns a non-nil error.
	if err := oleutil.ForEach(result, func(v *ole.VARIANT) error {
		found = true
		return nil
	}); err != nil {
		return false
	}
	return found
}

func setBrightnessInstances(service *ole.IDispatch, percent int) {
	result, ok := execBrightnessMethods(service)
	if !ok {
		return
	}
	defer result.Release()

	// Setting the builtin panel's brightness is best-effort: there is nothing useful to do with
	// a failure here, so both the enumeration and the method call are ignored explicitly.
	_ = oleutil.ForEach(result, func(v *ole.VARIANT) error {
		item := v.ToIDispatch()
		if item == nil {
			return nil
		}
		variant, err := oleutil.CallMethod(item, "WmiSetBrightness", 0, percent)
		if err == nil && variant != nil {
			_ = variant.Clear()
		}
		return nil
	})
}
