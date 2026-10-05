//go:build windows

package via

import (
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Any HID device works here (a mouse or a keyboard), VIA or not: the detail call is the same.
func TestDeviceInterfacePath(t *testing.T) {
	set, err := windows.SetupDiGetClassDevsEx(&guidDevinterfaceHID, "", 0, windows.DIGCF_PRESENT|windows.DIGCF_DEVICEINTERFACE, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()

	data := spDeviceInterfaceData{size: uint32(unsafe.Sizeof(spDeviceInterfaceData{}))}
	r, _, _ := procSetupDiEnumDeviceInterfaces.Call(uintptr(set), 0, uintptr(unsafe.Pointer(&guidDevinterfaceHID)), 0, uintptr(unsafe.Pointer(&data)))
	if r == 0 {
		t.Skip("no HID device on this machine")
	}

	path, ok := deviceInterfacePath(set, &data)
	if !ok || !strings.HasPrefix(path, `\\?\hid#`) {
		t.Fatalf("deviceInterfacePath() = %q, %v; want a \\\\?\\hid# path", path, ok)
	}
}
