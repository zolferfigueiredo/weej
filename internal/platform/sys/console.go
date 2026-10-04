//go:build windows

package sys

import (
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func StdoutIsTerminal() bool {
	return handleIsTerminal(windows.Handle(os.Stdout.Fd()))
}

func handleIsTerminal(h windows.Handle) bool {
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err == nil {
		return true
	}
	return isMsysPty(h)
}

// fileNameInfo mirrors the Win32 FILE_NAME_INFO struct: a length in bytes followed by the name.
type fileNameInfo struct {
	FileNameLength uint32
	FileName       [512]uint16
}

// isMsysPty recognizes Git Bash and other MSYS/Cygwin terminals: stdout there is a pipe whose
// name encodes the pty, not a console, so GetConsoleMode alone would miss it.
func isMsysPty(h windows.Handle) bool {
	var info fileNameInfo
	if err := windows.GetFileInformationByHandleEx(h, windows.FileNameInfo, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return false
	}
	n := info.FileNameLength / 2
	if n > uint32(len(info.FileName)) {
		n = uint32(len(info.FileName))
	}
	name := strings.ToLower(windows.UTF16ToString(info.FileName[:n]))
	if !strings.Contains(name, "-pty") {
		return false
	}
	return strings.Contains(name, `\msys-`) || strings.Contains(name, `\cygwin-`)
}
