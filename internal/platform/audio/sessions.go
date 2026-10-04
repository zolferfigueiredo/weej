//go:build windows

package audio

import (
	"sort"
	"strconv"
	"strings"
	"unsafe"

	wca "github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"

	"github.com/zolferfigueiredo/weej/internal/core"
)

// getProcessIdQuirk is AUDCLNT_S_NO_CURRENT_PROCESS (0x0889000D) in decimal. GetProcessId
// returns it for the system-sounds session and for multi-process (e.g. UWP) sessions; the pid
// it wrote may still be usable in the latter case.
const getProcessIdQuirk = "143196173"

type sessionInfo struct {
	key            string
	pid            uint32
	exe            string
	isSystemSounds bool
	state          uint32
}

func (a *Audio) registerDeviceChangeNotification() {
	client := wca.NewIMMNotificationClient(wca.IMMNotificationClientCallback{
		OnDefaultDeviceChanged: func(flow wca.EDataFlow, role wca.ERole, deviceID string) error {
			select {
			case a.deviceChanged <- struct{}{}:
			default:
			}
			return nil
		},
	})

	// go-wca has no unregister call, so this is a one-time, leak-for-the-process-lifetime setup.
	if err := a.enumerator.RegisterEndpointNotificationCallback(client); err != nil {
		a.logOnce("register-device-notify", "Could not watch for default device changes: "+err.Error())
		return
	}
	a.notifyClient = client
}

func (a *Audio) defaultEndpoint(flow uint32) (*wca.IMMDevice, error) {
	var dev *wca.IMMDevice
	if err := a.enumerator.GetDefaultAudioEndpoint(flow, wca.EConsole, &dev); err != nil {
		return nil, err
	}
	return dev, nil
}

func (a *Audio) rangeActiveRenderEndpoints(fn func(endpoint *wca.IMMDevice)) {
	var collection *wca.IMMDeviceCollection
	if err := a.enumerator.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		a.logOnce("enum-endpoints", "Could not list audio output devices: "+err.Error())
		return
	}
	defer collection.Release()

	var count uint32
	if err := collection.GetCount(&count); err != nil {
		a.logOnce("enum-endpoints-count", "Could not count audio output devices: "+err.Error())
		return
	}

	for i := uint32(0); i < count; i++ {
		var dev *wca.IMMDevice
		if err := collection.Item(i, &dev); err != nil {
			continue
		}
		fn(dev)
		dev.Release()
	}
}

func (a *Audio) eachRenderSession(endpoint *wca.IMMDevice, fn func(info sessionInfo, vol *wca.ISimpleAudioVolume)) {
	var manager *wca.IAudioSessionManager2
	if err := endpoint.Activate(wca.IID_IAudioSessionManager2, wca.CLSCTX_ALL, nil, &manager); err != nil {
		a.logOnce("activate-session-manager", "Could not activate an audio session manager: "+err.Error())
		return
	}
	defer manager.Release()

	var enumerator *wca.IAudioSessionEnumerator
	if err := manager.GetSessionEnumerator(&enumerator); err != nil {
		a.logOnce("session-enumerator", "Could not get an audio session enumerator: "+err.Error())
		return
	}
	defer enumerator.Release()

	var count int
	if err := enumerator.GetCount(&count); err != nil {
		a.logOnce("session-count", "Could not count audio sessions: "+err.Error())
		return
	}

	for i := 0; i < count; i++ {
		var ctrl *wca.IAudioSessionControl
		if err := enumerator.GetSession(i, &ctrl); err != nil {
			continue
		}

		dispatch, err := ctrl.QueryInterface(wca.IID_IAudioSessionControl2)
		ctrl.Release()
		if err != nil {
			continue
		}
		ctrl2 := (*wca.IAudioSessionControl2)(unsafe.Pointer(dispatch))

		info, ok := a.describeSession(ctrl2)
		if !ok {
			ctrl2.Release()
			continue
		}

		volDispatch, err := ctrl2.QueryInterface(wca.IID_ISimpleAudioVolume)
		if err != nil {
			ctrl2.Release()
			continue
		}
		vol := (*wca.ISimpleAudioVolume)(unsafe.Pointer(volDispatch))

		fn(info, vol)

		ctrl2.Release()
	}
}

func (a *Audio) describeSession(ctrl2 *wca.IAudioSessionControl2) (sessionInfo, bool) {
	isSystemSounds := ctrl2.IsSystemSoundsSession() == nil

	var pid uint32
	if err := ctrl2.GetProcessId(&pid); err != nil {
		if !isSystemSounds && !strings.Contains(err.Error(), getProcessIdQuirk) {
			return sessionInfo{}, false
		}
	}

	var state uint32
	_ = ctrl2.GetState(&state)

	var key string
	if err := ctrl2.GetSessionIdentifier(&key); err != nil || key == "" {
		key = "pid:" + strconv.FormatUint(uint64(pid), 10)
	}

	var exe string
	if !isSystemSounds {
		exe = a.exeForPID(pid)
	}

	return sessionInfo{key: key, pid: pid, exe: exe, isSystemSounds: isSystemSounds, state: state}, true
}

func (a *Audio) exeForPID(pid uint32) string {
	if pid == 0 {
		return ""
	}
	if exe, ok := a.exeCache[pid]; ok {
		return exe
	}
	exe := lookupExeName(pid)
	if exe != "" {
		a.exeCache[pid] = exe
	}
	return exe
}

func lookupExeName(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return core.ExeName(windows.UTF16ToString(buf[:size]))
}

func (a *Audio) setSessionVolume(vol *wca.ISimpleAudioVolume, level float64) {
	if err := vol.SetMasterVolume(float32(level), eventCtx); err != nil {
		a.logOnce("set-session-volume", "Could not set a session volume: "+err.Error())
	}
}

func (a *Audio) applyMaster(level float64) {
	dev, err := a.defaultEndpoint(wca.ERender)
	if err != nil {
		a.logOnce("default-render-endpoint", "Could not find the default output device: "+err.Error())
		return
	}
	defer dev.Release()

	var vol *wca.IAudioEndpointVolume
	if err := dev.Activate(wca.IID_IAudioEndpointVolume, wca.CLSCTX_ALL, nil, &vol); err != nil {
		a.logOnce("activate-master-volume", "Could not activate the output volume control: "+err.Error())
		return
	}
	defer vol.Release()

	if err := vol.SetMasterVolumeLevelScalar(float32(level), eventCtx); err != nil {
		a.logOnce("set-master-volume", "Could not set the output volume: "+err.Error())
	}
}

func (a *Audio) applyMic(level float64) {
	dev, err := a.defaultEndpoint(wca.ECapture)
	if err != nil {
		a.logOnce("default-capture-endpoint", "Could not find the default input device: "+err.Error())
		return
	}
	defer dev.Release()

	var vol *wca.IAudioEndpointVolume
	if err := dev.Activate(wca.IID_IAudioEndpointVolume, wca.CLSCTX_ALL, nil, &vol); err != nil {
		a.logOnce("activate-mic-volume", "Could not activate the input volume control: "+err.Error())
		return
	}
	defer vol.Release()

	if err := vol.SetMasterVolumeLevelScalar(float32(level), eventCtx); err != nil {
		a.logOnce("set-mic-volume", "Could not set the input volume: "+err.Error())
	}
}

func (a *Audio) applySystemSounds(level float64) {
	dev, err := a.defaultEndpoint(wca.ERender)
	if err != nil {
		a.logOnce("default-render-endpoint", "Could not find the default output device: "+err.Error())
		return
	}
	defer dev.Release()

	a.eachRenderSession(dev, func(info sessionInfo, vol *wca.ISimpleAudioVolume) {
		defer vol.Release()
		a.knownSessions[info.key] = struct{}{}
		if info.isSystemSounds {
			a.setSessionVolume(vol, level)
		}
	})
}

func (a *Audio) applyApps(level float64, exes map[string]struct{}) {
	a.rangeActiveRenderEndpoints(func(endpoint *wca.IMMDevice) {
		a.eachRenderSession(endpoint, func(info sessionInfo, vol *wca.ISimpleAudioVolume) {
			defer vol.Release()
			a.knownSessions[info.key] = struct{}{}
			if info.isSystemSounds || info.exe == "" {
				return
			}
			if _, ok := exes[info.exe]; ok {
				a.setSessionVolume(vol, level)
			}
		})
	})
}

type heldSession struct {
	exe string
	vol *wca.ISimpleAudioVolume
}

func (a *Audio) applyOtherApps(level float64, mapped []string) {
	var held []heldSession
	a.rangeActiveRenderEndpoints(func(endpoint *wca.IMMDevice) {
		a.eachRenderSession(endpoint, func(info sessionInfo, vol *wca.ISimpleAudioVolume) {
			a.knownSessions[info.key] = struct{}{}
			if info.isSystemSounds || info.exe == "" {
				vol.Release()
				return
			}
			held = append(held, heldSession{exe: info.exe, vol: vol})
		})
	})

	exes := make([]string, len(held))
	for i, h := range held {
		exes[i] = h.exe
	}
	others := toSet(core.OtherApps(exes, mapped, a.selfExe))

	for _, h := range held {
		if _, ok := others[h.exe]; ok {
			a.setSessionVolume(h.vol, level)
		}
		h.vol.Release()
	}
}

// pollNewSessions runs every second and on default-device change: any session not seen before
// that matches a stored target gets that level applied right away. Sessions we've already
// handled are left alone so manual adjustments in the Windows volume mixer stick between polls.
func (a *Audio) pollNewSessions() {
	a.rangeActiveRenderEndpoints(func(endpoint *wca.IMMDevice) {
		a.eachRenderSession(endpoint, func(info sessionInfo, vol *wca.ISimpleAudioVolume) {
			defer vol.Release()

			if _, known := a.knownSessions[info.key]; known {
				return
			}
			a.knownSessions[info.key] = struct{}{}

			if info.isSystemSounds {
				if a.haveSystemSoundsTarget {
					a.setSessionVolume(vol, a.systemSoundsLevel)
				}
				return
			}
			if info.exe == "" {
				return
			}
			if a.haveAppsTarget {
				if _, ok := a.appsExes[info.exe]; ok {
					a.setSessionVolume(vol, a.appsLevel)
					return
				}
			}
			if a.haveOtherAppsTarget {
				if others := core.OtherApps([]string{info.exe}, a.otherAppsMapped, a.selfExe); len(others) > 0 {
					a.setSessionVolume(vol, a.otherAppsLevel)
				}
			}
		})
	})
}

func (a *Audio) computePlaying() []string {
	visible := visiblePIDs()
	seen := map[string]struct{}{}

	a.rangeActiveRenderEndpoints(func(endpoint *wca.IMMDevice) {
		a.eachRenderSession(endpoint, func(info sessionInfo, vol *wca.ISimpleAudioVolume) {
			vol.Release()
			if info.isSystemSounds || info.exe == "" {
				return
			}
			if info.state != wca.AudioSessionStateActive {
				return
			}
			if _, ok := visible[info.pid]; !ok {
				return
			}
			seen[info.exe] = struct{}{}
		})
	})

	out := make([]string, 0, len(seen))
	for exe := range seen {
		out = append(out, exe)
	}
	sort.Strings(out)
	return out
}

func visiblePIDs() map[uint32]struct{} {
	result := map[uint32]struct{}{}
	cb := windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		if !windows.IsWindowVisible(hwnd) {
			return 1
		}
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err == nil && pid != 0 {
			result[pid] = struct{}{}
		}
		return 1
	})
	_ = windows.EnumWindows(cb, nil)
	return result
}
