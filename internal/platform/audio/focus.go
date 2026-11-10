//go:build windows

package audio

import (
	"os"
	"sync"

	wca "github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"

	"github.com/zolferfigueiredo/weej/internal/core"
)

// applicationFrameHost hosts the window for UWP apps; its own pid never owns the audio
// session, so the real app has to be found among its child windows.
const applicationFrameHost = "applicationframehost.exe"

// applyFocused turns the app in front, which then keeps its level once another comes to front.
func (a *Audio) applyFocused(level float64) {
	exe := a.exeForPID(focusedPID())
	if exe == "" {
		return
	}
	a.volumes.Set(exe, level)

	a.rangeActiveRenderEndpoints(func(endpoint *wca.IMMDevice) {
		a.eachRenderSession(endpoint, func(info sessionInfo, vol *wca.ISimpleAudioVolume) {
			defer vol.Release()
			if info.isSystemSounds || info.exe != exe {
				return
			}
			a.setSessionVolume(info, vol, level, "the Focused app knob")
		})
	})
}

// ForegroundApp is the app in front, its exe name and full path, worked out when asked rather than
// at the audio thread's last turn, so the HUD shows the app that is in front now.
func ForegroundApp() (exe, path string) {
	path = processImagePath(focusedPID())
	if path == "" {
		return "", ""
	}
	return core.ExeName(path), path
}

// focusedPID is the app Focused app turns, none while WeeJ itself is in front.
func focusedPID() uint32 {
	if pid := foregroundPID(); pid != uint32(os.Getpid()) {
		return pid
	}
	return 0
}

func foregroundPID() uint32 {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return 0
	}

	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil || pid == 0 {
		return 0
	}

	if lookupExeName(pid) != applicationFrameHost {
		return pid
	}
	if childPID := findChildProcess(hwnd, pid); childPID != 0 {
		return childPID
	}
	return pid
}

// One callback serves every search: each windows.NewCallback takes one of the few callback slots a Go
// program has, for good, so making one per search would end in a crash.
var (
	childMu     sync.Mutex
	childParent uint32
	childFound  uint32
	childSearch = windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err == nil && pid != 0 && pid != childParent {
			childFound = pid
			return 0
		}
		return 1
	})
)

func findChildProcess(parent windows.HWND, parentPID uint32) uint32 {
	childMu.Lock()
	defer childMu.Unlock()
	childParent, childFound = parentPID, 0
	windows.EnumChildWindows(parent, childSearch, nil)
	return childFound
}
