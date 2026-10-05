//go:build windows

package audio

import (
	"sync"
	"sync/atomic"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

// deviceNotifier is the IMMNotificationClient Windows calls when the default audio device
// changes. go-wca has one, but its callbacks return int64, which Go can turn into a Windows
// callback only on 64-bit: the 32-bit build panicked at startup.
type deviceNotifier struct {
	vtbl *deviceNotifierVtbl
}

type deviceNotifierVtbl struct {
	queryInterface         uintptr
	addRef                 uintptr
	release                uintptr
	onDeviceStateChanged   uintptr
	onDeviceAdded          uintptr
	onDeviceRemoved        uintptr
	onDefaultDeviceChanged uintptr
	onPropertyValueChanged uintptr
}

const (
	sOK          = 0
	eNoInterface = 0x80004002
)

var (
	// Windows callbacks are never freed, so the vtable is built once per process.
	notifierOnce sync.Once
	notifierVtbl *deviceNotifierVtbl

	// There is one Audio per process. Windows calls in on its own threads.
	defaultDeviceChanged atomic.Pointer[func()]
	notifierRefs         atomic.Int32
)

func newDeviceNotifier(onDefaultChanged func()) *deviceNotifier {
	notifierOnce.Do(func() {
		notifierVtbl = &deviceNotifierVtbl{
			queryInterface:         windows.NewCallback(notifierQueryInterface),
			addRef:                 windows.NewCallback(notifierAddRef),
			release:                windows.NewCallback(notifierRelease),
			onDeviceStateChanged:   windows.NewCallback(notifierDeviceStateChanged),
			onDeviceAdded:          windows.NewCallback(notifierDeviceAddedOrRemoved),
			onDeviceRemoved:        windows.NewCallback(notifierDeviceAddedOrRemoved),
			onDefaultDeviceChanged: windows.NewCallback(notifierDefaultDeviceChanged),
			onPropertyValueChanged: windows.NewCallback(notifierPropertyValueChanged),
		}
	})
	defaultDeviceChanged.Store(&onDefaultChanged)
	return &deviceNotifier{vtbl: notifierVtbl}
}

func notifierQueryInterface(this uintptr, riid *ole.GUID, ppv *uintptr) uintptr {
	if ole.IsEqualGUID(riid, ole.IID_IUnknown) || ole.IsEqualGUID(riid, wca.IID_IMMNotificationClient) {
		*ppv = this
		notifierRefs.Add(1)
		return sOK
	}
	*ppv = 0
	return eNoInterface
}

// The notifier lives as long as the process, so the count is only ever reported.
func notifierAddRef(this uintptr) uintptr {
	return uintptr(notifierRefs.Add(1))
}

func notifierRelease(this uintptr) uintptr {
	return uintptr(max(notifierRefs.Add(-1), 1))
}

func notifierDeviceStateChanged(this, deviceID, state uintptr) uintptr {
	return sOK
}

func notifierDeviceAddedOrRemoved(this, deviceID uintptr) uintptr {
	return sOK
}

// The flow and role enums are each their own argument, a slot apiece on either architecture.
func notifierDefaultDeviceChanged(this, flow, role, deviceID uintptr) uintptr {
	if f := defaultDeviceChanged.Load(); f != nil {
		(*f)()
	}
	return sOK
}
