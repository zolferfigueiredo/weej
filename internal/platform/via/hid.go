//go:build windows

package via

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modSetupAPI = windows.NewLazySystemDLL("setupapi.dll")
	modHID      = windows.NewLazySystemDLL("hid.dll")

	procSetupDiEnumDeviceInterfaces      = modSetupAPI.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW = modSetupAPI.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procHidDGetPreparsedData             = modHID.NewProc("HidD_GetPreparsedData")
	procHidDFreePreparsedData            = modHID.NewProc("HidD_FreePreparsedData")
	procHidPGetCaps                      = modHID.NewProc("HidP_GetCaps")
)

// GUID_DEVINTERFACE_HID, from hidclass.h.
var guidDevinterfaceHID = windows.GUID{
	Data1: 0x4d1e55b2,
	Data2: 0xf16f,
	Data3: 0x11cf,
	Data4: [8]byte{0x88, 0xcb, 0x00, 0x11, 0x11, 0x00, 0x00, 0x30},
}

const (
	viaUsagePage = 0xFF60
	viaUsage     = 0x61

	// HIDP_STATUS_SUCCESS.
	hidpStatusSuccess = 0x00110000

	// Report id byte plus the 32-byte report; used when a device's own cap is 0.
	defaultOutputReportLength = 33
)

// Mirrors SP_DEVICE_INTERFACE_DATA. The trailing field pads to ULONG_PTR like the C
// struct does on amd64.
type spDeviceInterfaceData struct {
	size      uint32
	classGUID windows.GUID
	flags     uint32
	_         uintptr
}

// Mirrors HIDP_CAPS; only the fields via needs are named, the rest is layout padding.
type hidpCaps struct {
	usage                   uint16
	usagePage               uint16
	inputReportByteLength   uint16
	outputReportByteLength  uint16
	featureReportByteLength uint16
	reserved                [17]uint16
	_                       [10]uint16
}

type hidDevice struct {
	path      string
	handle    windows.Handle
	outputLen int
}

func (d *hidDevice) open() error {
	pathPtr, err := windows.UTF16PtrFromString(d.path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(pathPtr, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return err
	}
	d.handle = h
	return nil
}

func (d *hidDevice) close() {
	if d.handle != 0 && d.handle != windows.InvalidHandle {
		_ = windows.CloseHandle(d.handle)
	}
	d.handle = 0
}

func (d *hidDevice) write(report []byte) error {
	buf := make([]byte, d.outputLen)
	copy(buf[1:], report) // buf[0] stays 0x00, the report id QMK's raw HID expects
	var written uint32
	return windows.WriteFile(d.handle, buf, &written, nil)
}

func findKeyboards(log func(string)) []*hidDevice {
	set, err := windows.SetupDiGetClassDevsEx(&guidDevinterfaceHID, "", 0, windows.DIGCF_PRESENT|windows.DIGCF_DEVICEINTERFACE, 0, "")
	if err != nil {
		log("Could not enumerate HID devices: " + err.Error())
		return nil
	}
	defer set.Close()

	var devices []*hidDevice
	for index := uint32(0); ; index++ {
		data := spDeviceInterfaceData{size: uint32(unsafe.Sizeof(spDeviceInterfaceData{}))}
		r, _, callErr := procSetupDiEnumDeviceInterfaces.Call(
			uintptr(set),
			0,
			uintptr(unsafe.Pointer(&guidDevinterfaceHID)),
			uintptr(index),
			uintptr(unsafe.Pointer(&data)),
		)
		if r == 0 {
			if callErr != windows.ERROR_NO_MORE_ITEMS {
				log("Could not enumerate HID device interfaces: " + callErr.Error())
			}
			break
		}

		path, ok := deviceInterfacePath(set, &data)
		if !ok {
			continue
		}

		dev := probeVIADevice(path)
		if dev == nil {
			continue
		}
		if err := dev.open(); err != nil {
			continue
		}
		devices = append(devices, dev)
	}
	return devices
}

func deviceInterfacePath(set windows.DevInfo, data *spDeviceInterfaceData) (string, bool) {
	var required uint32
	procSetupDiGetDeviceInterfaceDetailW.Call(
		uintptr(set),
		uintptr(unsafe.Pointer(data)),
		0,
		0,
		uintptr(unsafe.Pointer(&required)),
		0,
	)
	if required == 0 {
		return "", false
	}

	buf := make([]byte, required)
	// SP_DEVICE_INTERFACE_DETAIL_DATA_W.cbSize is the struct's fixed size, whatever the
	// buffer's length: a DWORD and one WCHAR, packed to 6 bytes on 32-bit but aligned to
	// 8 on 64-bit. The API rejects any other value with ERROR_INVALID_USER_BUFFER.
	detailSize := uint32(6)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		detailSize = 8
	}
	*(*uint32)(unsafe.Pointer(&buf[0])) = detailSize

	r, _, _ := procSetupDiGetDeviceInterfaceDetailW.Call(
		uintptr(set),
		uintptr(unsafe.Pointer(data)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(required),
		0,
		0,
	)
	if r == 0 {
		return "", false
	}

	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(&buf[4]))), true
}

func probeVIADevice(path string) *hidDevice {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil
	}
	h, err := windows.CreateFile(pathPtr, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil
	}
	defer func() { _ = windows.CloseHandle(h) }()

	var preparsed uintptr
	r, _, _ := procHidDGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&preparsed)))
	if r == 0 || preparsed == 0 {
		return nil
	}
	defer procHidDFreePreparsedData.Call(preparsed)

	var caps hidpCaps
	status, _, _ := procHidPGetCaps.Call(preparsed, uintptr(unsafe.Pointer(&caps)))
	if status != hidpStatusSuccess {
		return nil
	}
	if caps.usagePage != viaUsagePage || caps.usage != viaUsage {
		return nil
	}

	outLen := int(caps.outputReportByteLength)
	if outLen <= 0 {
		outLen = defaultOutputReportLength
	}
	return &hidDevice{path: path, outputLen: outLen}
}
