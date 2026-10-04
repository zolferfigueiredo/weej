//go:build windows

package updater

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// productVersion reads the StringFileInfo ProductVersion string from the fixed 040904b0 (US
// English, Unicode) code page block, the one go-winres always writes alongside the fixed info.
func productVersion(path string) (string, error) {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return "", fmt.Errorf("updater: version info size: %w", err)
	}

	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return "", fmt.Errorf("updater: version info: %w", err)
	}

	var value unsafe.Pointer
	var length uint32
	err = windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\StringFileInfo\040904b0\ProductVersion`, unsafe.Pointer(&value), &length)
	if err != nil || length == 0 {
		return "", fmt.Errorf("updater: no ProductVersion resource: %w", err)
	}
	return windows.UTF16PtrToString((*uint16)(value)), nil
}
