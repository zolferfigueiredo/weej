//go:build windows

package display

import (
	"image"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/zolferfigueiredo/weej/internal/core"
)

type Monitor struct {
	Screen            int    // 0-based index among externals left to right; -1 for the built-in panel
	Name              string // friendly name, e.g. "DELL U2719DC"
	Rect, Work        image.Rectangle
	Internal, Primary bool
	Handle            uintptr // HMONITOR
	DPI               int
}

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	shcore = windows.NewLazySystemDLL("shcore.dll")

	procEnumDisplayMonitors         = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW             = user32.NewProc("GetMonitorInfoW")
	procMonitorFromPoint            = user32.NewProc("MonitorFromPoint")
	procGetCursorPos                = user32.NewProc("GetCursorPos")
	procGetDisplayConfigBufferSizes = user32.NewProc("GetDisplayConfigBufferSizes")
	procQueryDisplayConfig          = user32.NewProc("QueryDisplayConfig")
	procDisplayConfigGetDeviceInfo  = user32.NewProc("DisplayConfigGetDeviceInfo")

	procGetDpiForMonitor = shcore.NewProc("GetDpiForMonitor")
)

const (
	monitorDefaultToNearest = 2
	monitorInfoFPrimary     = 0x1
	mdtEffectiveDPI         = 0

	qdcOnlyActivePaths = 0x00000002

	displayConfigDeviceInfoGetSourceName = 1
	displayConfigDeviceInfoGetTargetName = 2

	// DISPLAYCONFIG_VIDEO_OUTPUT_TECHNOLOGY values that mean the panel is built in rather than a
	// cable to an external monitor.
	outputTechnologyLVDS                = 6
	outputTechnologyDisplayPortEmbedded = 11
	outputTechnologyUDIEmbedded         = 13
	outputTechnologyInternal            = 0x80000000
)

type rect struct {
	Left, Top, Right, Bottom int32
}

type point struct {
	X, Y int32
}

type monitorInfoExW struct {
	CbSize    uint32
	RcMonitor rect
	RcWork    rect
	DwFlags   uint32
	SzDevice  [32]uint16
}

type displayConfigPathSourceInfo struct {
	AdapterId   windows.LUID
	Id          uint32
	ModeInfoIdx uint32
	StatusFlags uint32
}

type displayConfigRational struct {
	Numerator   uint32
	Denominator uint32
}

type displayConfigPathTargetInfo struct {
	AdapterId        windows.LUID
	Id               uint32
	ModeInfoIdx      uint32
	OutputTechnology uint32
	Rotation         uint32
	Scaling          uint32
	RefreshRate      displayConfigRational
	ScanLineOrdering uint32
	TargetAvailable  int32
	StatusFlags      uint32
}

type displayConfigPathInfo struct {
	SourceInfo displayConfigPathSourceInfo
	TargetInfo displayConfigPathTargetInfo
	Flags      uint32
}

// DISPLAYCONFIG_MODE_INFO is a union whose real size we never read fields from; this just has to
// be at least as large as the OS structure (64 bytes) so QueryDisplayConfig has room to write.
type displayConfigModeInfo struct {
	_ [128]byte
}

type displayConfigDeviceInfoHeader struct {
	Type      uint32
	Size      uint32
	AdapterId windows.LUID
	Id        uint32
}

type displayConfigSourceDeviceName struct {
	Header            displayConfigDeviceInfoHeader
	ViewGdiDeviceName [32]uint16
}

type displayConfigTargetDeviceName struct {
	Header                    displayConfigDeviceInfoHeader
	Flags                     uint32
	OutputTechnology          uint32
	EdidManufactureId         uint16
	EdidProductCodeId         uint16
	ConnectorInstance         uint32
	MonitorFriendlyDeviceName [64]uint16
	MonitorDevicePath         [128]uint16
}

type targetInfo struct {
	internal bool
	name     string
}

func Monitors() []Monitor {
	handles := enumMonitorHandles()
	targets := queryDisplayTargets()

	type raw struct {
		handle   uintptr
		rect     rect
		work     rect
		primary  bool
		device   string
		internal bool
		name     string
		dpi      int
	}

	var raws []raw
	var displays []core.Display
	for _, h := range handles {
		mi, ok := getMonitorInfo(h)
		if !ok {
			continue
		}
		t := targets[mi.device]
		name := t.name
		if name == "" {
			name = mi.device
		}
		raws = append(raws, raw{
			handle:   h,
			rect:     mi.rcMonitor,
			work:     mi.rcWork,
			primary:  mi.primary,
			device:   mi.device,
			internal: t.internal,
			name:     name,
			dpi:      getMonitorDPI(h),
		})
		displays = append(displays, core.Display{
			ID:       mi.device,
			Left:     int(mi.rcMonitor.Left),
			Top:      int(mi.rcMonitor.Top),
			Right:    int(mi.rcMonitor.Right),
			Bottom:   int(mi.rcMonitor.Bottom),
			Internal: t.internal,
			Primary:  mi.primary,
		})
	}

	externals := core.Externals(displays)
	screenOf := make(map[string]int, len(externals))
	for i, d := range externals {
		screenOf[d.ID] = i
	}

	out := make([]Monitor, 0, len(raws))
	for _, r := range raws {
		screen := -1
		if !r.internal {
			if idx, ok := screenOf[r.device]; ok {
				screen = idx
			}
		}
		out = append(out, Monitor{
			Screen:   screen,
			Name:     r.name,
			Rect:     image.Rect(int(r.rect.Left), int(r.rect.Top), int(r.rect.Right), int(r.rect.Bottom)),
			Work:     image.Rect(int(r.work.Left), int(r.work.Top), int(r.work.Right), int(r.work.Bottom)),
			Internal: r.internal,
			Primary:  r.primary,
			Handle:   r.handle,
			DPI:      r.dpi,
		})
	}
	return out
}

func AtPointer() Monitor {
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// MonitorFromPoint takes the POINT by value. On the amd64 calling convention an 8-byte
	// struct is passed packed into a single register, so X and Y go into one uintptr.
	packed := uintptr(uint64(uint32(pt.X)) | uint64(uint32(pt.Y))<<32)
	h, _, _ := procMonitorFromPoint.Call(packed, monitorDefaultToNearest)

	for _, m := range Monitors() {
		if m.Handle == h {
			return m
		}
	}
	return Monitor{}
}

func enumMonitorHandles() []uintptr {
	var handles []uintptr
	cb := windows.NewCallback(func(hMonitor uintptr, _ uintptr, _ *rect, _ uintptr) uintptr {
		handles = append(handles, hMonitor)
		return 1
	})
	procEnumDisplayMonitors.Call(0, 0, cb, 0)
	return handles
}

type monitorInfo struct {
	rcMonitor rect
	rcWork    rect
	primary   bool
	device    string
}

func getMonitorInfo(h uintptr) (monitorInfo, bool) {
	var mi monitorInfoExW
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	ret, _, _ := procGetMonitorInfoW.Call(h, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		return monitorInfo{}, false
	}
	return monitorInfo{
		rcMonitor: mi.RcMonitor,
		rcWork:    mi.RcWork,
		primary:   mi.DwFlags&monitorInfoFPrimary != 0,
		device:    windows.UTF16ToString(mi.SzDevice[:]),
	}, true
}

func getMonitorDPI(h uintptr) int {
	var dpiX, dpiY uint32
	ret, _, _ := procGetDpiForMonitor.Call(h, mdtEffectiveDPI, uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY)))
	if ret != 0 { // HRESULT, S_OK == 0
		return 96
	}
	return int(dpiX)
}

// queryDisplayTargets maps a monitor's GDI device name (e.g. "\\.\DISPLAY1", the same string
// GetMonitorInfoW reports) to its output technology and EDID friendly name. An empty map means
// QueryDisplayConfig didn't cooperate; callers fall back to treating everything as external.
func queryDisplayTargets() map[string]targetInfo {
	result := map[string]targetInfo{}

	for attempt := 0; attempt < 5; attempt++ {
		var numPaths, numModes uint32
		ret, _, _ := procGetDisplayConfigBufferSizes.Call(
			uintptr(qdcOnlyActivePaths),
			uintptr(unsafe.Pointer(&numPaths)),
			uintptr(unsafe.Pointer(&numModes)),
		)
		if ret != 0 || numPaths == 0 {
			return result
		}

		paths := make([]displayConfigPathInfo, numPaths)
		modes := make([]displayConfigModeInfo, numModes)
		var modesPtr unsafe.Pointer
		if numModes > 0 {
			modesPtr = unsafe.Pointer(&modes[0])
		}

		ret, _, _ = procQueryDisplayConfig.Call(
			uintptr(qdcOnlyActivePaths),
			uintptr(unsafe.Pointer(&numPaths)),
			uintptr(unsafe.Pointer(&paths[0])),
			uintptr(unsafe.Pointer(&numModes)),
			uintptr(modesPtr),
			0,
		)
		if ret == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			continue // topology changed between the two calls: retry from the top
		}
		if ret != 0 {
			return result
		}

		for i := uint32(0); i < numPaths; i++ {
			p := &paths[i]
			device, ok := sourceGdiDeviceName(p.SourceInfo.AdapterId, p.SourceInfo.Id)
			if !ok {
				continue
			}
			result[device] = targetInfo{
				internal: isInternalTechnology(p.TargetInfo.OutputTechnology),
				name:     targetFriendlyName(p.TargetInfo.AdapterId, p.TargetInfo.Id),
			}
		}
		return result
	}
	return result
}

func sourceGdiDeviceName(adapterId windows.LUID, id uint32) (string, bool) {
	var req displayConfigSourceDeviceName
	req.Header.Type = displayConfigDeviceInfoGetSourceName
	req.Header.Size = uint32(unsafe.Sizeof(req))
	req.Header.AdapterId = adapterId
	req.Header.Id = id
	ret, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&req)))
	if ret != 0 {
		return "", false
	}
	return windows.UTF16ToString(req.ViewGdiDeviceName[:]), true
}

func targetFriendlyName(adapterId windows.LUID, id uint32) string {
	var req displayConfigTargetDeviceName
	req.Header.Type = displayConfigDeviceInfoGetTargetName
	req.Header.Size = uint32(unsafe.Sizeof(req))
	req.Header.AdapterId = adapterId
	req.Header.Id = id
	ret, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&req)))
	if ret != 0 {
		return ""
	}
	return windows.UTF16ToString(req.MonitorFriendlyDeviceName[:])
}

func isInternalTechnology(tech uint32) bool {
	switch tech {
	case outputTechnologyInternal, outputTechnologyLVDS, outputTechnologyDisplayPortEmbedded, outputTechnologyUDIEmbedded:
		return true
	default:
		return false
	}
}
