//go:build windows

package audio

import (
	wca "github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

// applicationFrameHost hosts the window for UWP apps; its own pid never owns the audio
// session, so the real app has to be found among its child windows.
const applicationFrameHost = "applicationframehost.exe"

func (a *Audio) applyFocused(level float64) {
	exe := a.resolveFocusedExe()
	a.setFocusedExe(exe)
	if exe == "" {
		return
	}

	a.rangeActiveRenderEndpoints(func(endpoint *wca.IMMDevice) {
		a.eachRenderSession(endpoint, func(info sessionInfo, vol *wca.ISimpleAudioVolume) {
			defer vol.Release()
			if info.isSystemSounds || info.exe != exe {
				return
			}
			a.setSessionVolume(vol, level)
		})
	})
}

func (a *Audio) resolveFocusedExe() string {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return ""
	}

	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil || pid == 0 {
		return ""
	}

	exe := a.exeForPID(pid)
	if exe != applicationFrameHost {
		return exe
	}

	if childPID := findChildProcess(hwnd, pid); childPID != 0 {
		if childExe := a.exeForPID(childPID); childExe != "" {
			return childExe
		}
	}
	return exe
}

func findChildProcess(parent windows.HWND, parentPID uint32) uint32 {
	var found uint32
	cb := windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var childPID uint32
		if _, err := windows.GetWindowThreadProcessId(hwnd, &childPID); err == nil && childPID != 0 && childPID != parentPID {
			found = childPID
			return 0
		}
		return 1
	})
	windows.EnumChildWindows(parent, cb, nil)
	return found
}
