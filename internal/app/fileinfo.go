//go:build windows

package app

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func runningProcessPath(exe string) string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(snap) }()

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return ""
	}
	for {
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if name == exe {
			if path := fullProcessImagePath(entry.ProcessID); path != "" {
				return path
			}
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			return ""
		}
	}
}

func fullProcessImagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

func appPathsRegistryPath(exe string) string {
	const sub = `Software\Microsoft\Windows\CurrentVersion\App Paths\`
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		k, err := registry.OpenKey(root, sub+exe, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("")
		k.Close()
		if err == nil && v != "" {
			return v
		}
	}
	return ""
}

func fileDescription(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}

	for _, block := range translationBlocks(buf) {
		sub := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, block.lang, block.codePage)
		if s := verQueryString(buf, sub); s != "" {
			return s
		}
	}
	// A handful of common blocks third-party installers use even without an
	// advertised translation table.
	for _, sub := range []string{
		`\StringFileInfo\040904b0\FileDescription`,
		`\StringFileInfo\040904e4\FileDescription`,
		`\StringFileInfo\000004b0\FileDescription`,
	} {
		if s := verQueryString(buf, sub); s != "" {
			return s
		}
	}
	return ""
}

type translationBlock struct{ lang, codePage uint16 }

func translationBlocks(buf []byte) []translationBlock {
	var value unsafe.Pointer
	var length uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&value), &length); err != nil || length < 4 {
		return nil
	}
	n := int(length) / 4
	out := make([]translationBlock, 0, n)
	words := unsafe.Slice((*uint16)(value), n*2)
	for i := 0; i < n; i++ {
		out = append(out, translationBlock{lang: words[i*2], codePage: words[i*2+1]})
	}
	return out
}

func verQueryString(buf []byte, subBlock string) string {
	var value unsafe.Pointer
	var length uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), subBlock, unsafe.Pointer(&value), &length); err != nil || length == 0 {
		return ""
	}
	return windows.UTF16PtrToString((*uint16)(value))
}
