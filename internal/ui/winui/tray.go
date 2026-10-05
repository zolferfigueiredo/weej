//go:build windows

package winui

import (
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	procShellNotifyIconW   = shell32.NewProc("Shell_NotifyIconW")
	procCreateIconIndirect = user32.NewProc("CreateIconIndirect")
	procDestroyIcon        = user32.NewProc("DestroyIcon")
)

const (
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010
	nifShowTip = 0x00000080 // required for a tooltip to appear once NOTIFYICON_VERSION_4 is set

	niifInfo = 0x00000001

	notifyIconVersion4 = 4

	// NOTIFYICON_VERSION_4 delivers its notification code in the low word of lParam of the
	// app's chosen callback message; WM_CONTEXTMENU arrives the same way on a right-click.
	ninSelect           = wmUser + 0
	ninKeySelect        = wmUser + 1
	ninBalloonUserClick = wmUser + 5
)

// notifyIconDataW mirrors the unicode NOTIFYICONDATAW struct (post-Vista size).
type notifyIconDataW struct {
	size             uint32
	wnd              windows.HWND
	id               uint32
	flags            uint32
	callbackMessage  uint32
	icon             windows.Handle
	tip              [128]uint16
	state            uint32
	stateMask        uint32
	info             [256]uint16
	versionOrTimeout uint32
	infoTitle        [64]uint16
	infoFlags        uint32
	guidItem         windows.GUID
	balloonIcon      windows.Handle
}

func shellNotifyIconW(message uint32, data *notifyIconDataW) bool {
	r, _, _ := procShellNotifyIconW.Call(uintptr(message), uintptr(unsafe.Pointer(data)))
	return r != 0
}

type Tray struct {
	loop           *Loop
	uid            uint32
	msg            uint32
	onSelect       func()
	onMenu         func()
	onBalloonClick func()

	icon  windows.Handle
	tip   string
	added bool
}

func (l *Loop) NewTray(onSelect, onMenu, onBalloonClick func()) *Tray {
	l.mu.Lock()
	if l.taskbarCreatedMsg == 0 {
		l.taskbarCreatedMsg = registerWindowMessageW("TaskbarCreated")
	}
	l.nextTrayID++
	t := &Tray{
		loop:           l,
		uid:            l.nextTrayID,
		msg:            wmApp + 2 + l.nextTrayID,
		onSelect:       onSelect,
		onMenu:         onMenu,
		onBalloonClick: onBalloonClick,
	}
	l.trayListeners = append(l.trayListeners, t)
	l.mu.Unlock()

	l.on(t.msg, t.handleCallback)
	return t
}

func (t *Tray) handleCallback(wparam, lparam uintptr) uintptr {
	code := uint32(lparam) & 0xFFFF
	switch code {
	// Version 4 also sends the raw WM_LBUTTONUP before NIN_SELECT; handling both acts twice.
	case ninSelect, ninKeySelect:
		if t.onSelect != nil {
			t.onSelect()
		}
	case wmContextMenu:
		if t.onMenu != nil {
			t.onMenu()
		}
	case ninBalloonUserClick:
		if t.onBalloonClick != nil {
			t.onBalloonClick()
		}
	}
	return 0
}

func (t *Tray) base(flags uint32) *notifyIconDataW {
	d := &notifyIconDataW{
		size:            uint32(unsafe.Sizeof(notifyIconDataW{})),
		wnd:             t.loop.hwnd,
		id:              t.uid,
		flags:           flags,
		callbackMessage: t.msg,
		icon:            t.icon,
	}
	setUTF16(d.tip[:], t.tip)
	return d
}

func (t *Tray) SetIcon(img *image.NRGBA) {
	old := t.icon
	t.icon = iconFromNRGBA(img)
	if t.added {
		shellNotifyIconW(nimModify, t.base(nifIcon))
	}
	if old != 0 {
		procDestroyIcon.Call(uintptr(old))
	}
}

func (t *Tray) SetTooltip(s string) {
	t.tip = s
	if t.added {
		shellNotifyIconW(nimModify, t.base(nifTip|nifShowTip))
	}
}

func (t *Tray) Show() {
	if t.added {
		return
	}
	shellNotifyIconW(nimAdd, t.base(nifMessage|nifIcon|nifTip|nifShowTip))
	t.added = true
	version := t.base(0)
	version.versionOrTimeout = notifyIconVersion4
	shellNotifyIconW(nimSetVersion, version)
}

func (t *Tray) Hide() {
	if !t.added {
		return
	}
	shellNotifyIconW(nimDelete, t.base(0))
	t.added = false
}

func (t *Tray) Balloon(title, text string) {
	if !t.added {
		return
	}
	d := t.base(nifInfo)
	setUTF16(d.infoTitle[:], title)
	setUTF16(d.info[:], text)
	d.infoFlags = niifInfo
	shellNotifyIconW(nimModify, d)
}

func (t *Tray) IconSize() int {
	return int(getSystemMetricsForDpi(smCxSmIcon, dpiPrimary()))
}

// readd is called when the shell broadcasts TaskbarCreated (Explorer restarted): the icon is
// gone from its new tray, so re-add it if it was supposed to be showing.
func (t *Tray) readd() {
	if !t.added {
		return
	}
	t.added = false
	t.Show()
}

// iconFromNRGBA builds an HICON via CreateIconIndirect from a straight-alpha 32-bpp color
// bitmap plus an all-opaque AND mask; the alpha channel alone drives transparency on anything
// since XP, so the mask just needs to not hide anything.
func iconFromNRGBA(img *image.NRGBA) windows.Handle {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= 0 || h <= 0 {
		return 0
	}
	color, bits := createDIB32(w, h, true)
	writeDIBPixels(bits, img, false)
	mask := createBitmap(int32(w), int32(h), 1, 1, monoMaskBits(w, h))

	info := iconInfo{
		fIcon:    1,
		hbmMask:  uintptr(mask),
		hbmColor: uintptr(color),
	}
	r, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))

	deleteObject(mask)
	deleteObject(color)
	return windows.Handle(r)
}

// iconInfo mirrors ICONINFO.
type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}
