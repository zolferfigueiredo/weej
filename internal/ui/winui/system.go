//go:build windows

package winui

import (
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	powrprof = windows.NewLazySystemDLL("powrprof.dll")

	procLockWorkStation     = user32.NewProc("LockWorkStation")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procGetWindow           = user32.NewProc("GetWindow")
	procGetWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	procIsIconic            = user32.NewProc("IsIconic")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procAttachThreadInput   = user32.NewProc("AttachThreadInput")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")
	procSetSuspendState     = powrprof.NewProc("SetSuspendState")
)

const (
	wmAppCommand   = 0x0319
	wmClose        = 0x0010
	wmSysCommand   = 0x0112
	scMonitorPower = 0xF170
	hwndBroadcast  = 0xFFFF
	gwOwner        = 4
	swRestore      = 9

	AppCommandMediaPlay  = 46
	AppCommandMediaPause = 47
)

// AppCommand hands a WM_APPCOMMAND to the taskbar, which passes it on through the shell to the
// app playing media, the way a keyboard's media keys reach it.
func AppCommand(cmd int) {
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(mustUTF16PtrFromString("Shell_TrayWnd"))), 0)
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmAppCommand, hwnd, uintptr(cmd)<<16)
	}
}

func LockPC() { procLockWorkStation.Call() }

// SleepPC needs the shutdown privilege, which a normal user holds but has switched off.
func SleepPC() {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err == nil {
		var luid windows.LUID
		if windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeShutdownPrivilege"), &luid) == nil {
			privs := windows.Tokenprivileges{PrivilegeCount: 1}
			privs.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
			_ = windows.AdjustTokenPrivileges(token, false, &privs, 0, nil, nil)
		}
		token.Close()
	}
	procSetSuspendState.Call(0, 0, 0)
}

func ScreensOff() { procPostMessageW.Call(hwndBroadcast, wmSysCommand, scMonitorPower, 2) }

// FocusApp brings the main window of a running app to the front and reports whether it found
// one. exe is a file name such as "spotify.exe".
func FocusApp(exe string) bool {
	wins := appWindows(exe)
	if len(wins) == 0 {
		return false
	}
	hwnd := uintptr(wins[0])
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}
	// Windows only lets the app the user is working in hand focus away, so this borrows its
	// input queue for the moment it takes.
	fg, _, _ := procGetForegroundWindow.Call()
	fgThread, _ := windows.GetWindowThreadProcessId(windows.HWND(fg), nil)
	self := windows.GetCurrentThreadId()
	if fgThread != 0 && fgThread != self {
		procAttachThreadInput.Call(uintptr(self), uintptr(fgThread), 1)
		defer procAttachThreadInput.Call(uintptr(self), uintptr(fgThread), 0)
	}
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
	return true
}

// CloseApp asks every main window of a running app to close, as its X button would.
func CloseApp(exe string) {
	for _, hwnd := range appWindows(exe) {
		procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
	}
}

var (
	findMu     sync.Mutex
	findExe    string
	findResult []windows.HWND
	// One callback for every search, since Go never frees callbacks.
	findWindow = windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		if !windows.IsWindowVisible(hwnd) {
			return 1
		}
		if owner, _, _ := procGetWindow.Call(uintptr(hwnd), gwOwner); owner != 0 {
			return 1
		}
		if n, _, _ := procGetWindowTextLength.Call(uintptr(hwnd)); n == 0 {
			return 1
		}
		var pid uint32
		_, _ = windows.GetWindowThreadProcessId(hwnd, &pid)
		if strings.EqualFold(processExe(pid), findExe) {
			findResult = append(findResult, hwnd)
		}
		return 1
	})
)

// appWindows lists the visible, titled, unowned top-level windows of every process running exe.
func appWindows(exe string) []windows.HWND {
	findMu.Lock()
	defer findMu.Unlock()
	findExe, findResult = exe, nil
	_ = windows.EnumWindows(findWindow, nil)
	return findResult
}

func processExe(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:n]))
}
