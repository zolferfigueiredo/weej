//go:build windows

package winui

import (
	"image"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	uxtheme = windows.NewLazySystemDLL("uxtheme.dll")

	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procInsertMenuItemW  = user32.NewProc("InsertMenuItemW")
	procSetMenuDefaultIt = user32.NewProc("SetMenuDefaultItem")
	procTrackPopupMenuEx = user32.NewProc("TrackPopupMenuEx")

	ordSetPreferredAppMode uintptr
	ordFlushMenuThemes     uintptr
)

const (
	miimState   = 0x00000001
	miimID      = 0x00000002
	miimSubMenu = 0x00000004
	miimType    = 0x00000010
	miimBitmap  = 0x00000080

	mftString    = 0x00000000
	mftSeparator = 0x00000800

	mfsChecked  = 0x00000008
	mfsDisabled = 0x00000003

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100
)

// menuItemInfoW mirrors the unicode MENUITEMINFOW struct.
type menuItemInfoW struct {
	size       uint32
	mask       uint32
	typ        uint32
	state      uint32
	id         uint32
	subMenu    windows.Handle
	checked    windows.Handle
	unchecked  windows.Handle
	itemData   uintptr
	typeData   *uint16
	cch        uint32
	bitmapItem windows.Handle
}

type MenuItem struct {
	Text, Shortcut string
	Checked        bool
	Disabled       bool
	Bold           bool
	Separator      bool
	Icon           *image.NRGBA
	Children       []MenuItem
	OnClick        func()
}

type builtMenu struct {
	root    windows.Handle
	bitmaps []windows.Handle
	actions map[uint32]func()
}

func (l *Loop) PopupMenu(items []MenuItem, dark bool) {
	applyMenuTheme(dark)

	pt := getCursorPos()
	dpi := dpiForPoint(pt)

	bm := &builtMenu{actions: make(map[uint32]func())}
	var nextID uint32
	bm.root = buildMenu(items, dpi, bm, &nextID)

	setForegroundWindow(l.hwnd)
	cmd, _, _ := procTrackPopupMenuEx.Call(uintptr(bm.root), uintptr(tpmRightButton|tpmReturnCmd), uintptr(pt.X), uintptr(pt.Y), uintptr(l.hwnd), 0)
	postMessageW(l.hwnd, wmNull, 0, 0)

	procDestroyMenu.Call(uintptr(bm.root)) // also destroys owned submenus
	for _, b := range bm.bitmaps {
		deleteObject(b)
	}

	if fn, ok := bm.actions[uint32(cmd)]; ok && fn != nil {
		fn()
	}
}

func buildMenu(items []MenuItem, dpi uint32, bm *builtMenu, nextID *uint32) windows.Handle {
	hmenu, _, _ := procCreatePopupMenu.Call()
	menu := windows.Handle(hmenu)

	for pos, it := range items {
		pos := uint32(pos)

		if it.Separator {
			insertMenuItem(menu, pos, &menuItemInfoW{
				mask: miimType,
				typ:  mftSeparator,
			})
			continue
		}

		var state uint32
		if it.Checked {
			state |= mfsChecked
		}
		if it.Disabled {
			state |= mfsDisabled
		}

		mii := &menuItemInfoW{
			mask:     miimType | miimState,
			typ:      mftString,
			state:    state,
			typeData: itemText(it),
			cch:      uint32(len([]rune(it.Text))),
		}
		if it.Icon != nil {
			bmp := iconToMenuBitmap(it.Icon, dpi)
			bm.bitmaps = append(bm.bitmaps, bmp)
			mii.mask |= miimBitmap
			mii.bitmapItem = bmp
		}

		var id uint32
		if len(it.Children) > 0 {
			mii.mask |= miimSubMenu
			mii.subMenu = buildMenu(it.Children, dpi, bm, nextID)
		} else {
			*nextID++
			id = *nextID
			mii.mask |= miimID
			mii.id = id
		}
		insertMenuItem(menu, pos, mii)

		if id != 0 && it.OnClick != nil {
			bm.actions[id] = it.OnClick
		}
		if it.Bold && id != 0 {
			procSetMenuDefaultIt.Call(uintptr(menu), uintptr(id), 0)
		}
	}
	return menu
}

// itemText renders the shortcut right-aligned with a tab, as menus natively support.
func itemText(it MenuItem) *uint16 {
	s := it.Text
	if it.Shortcut != "" {
		s += "\t" + it.Shortcut
	}
	return mustUTF16PtrFromString(s)
}

func insertMenuItem(menu windows.Handle, pos uint32, mii *menuItemInfoW) {
	mii.size = uint32(unsafe.Sizeof(menuItemInfoW{}))
	procInsertMenuItemW.Call(uintptr(menu), uintptr(pos), 1, uintptr(unsafe.Pointer(mii)))
}

// iconToMenuBitmap scales the caller's 16x16 icon to the system's menu-check metric at dpi and
// builds a premultiplied DIB section, which Windows composites correctly as an hbmpItem.
func iconToMenuBitmap(icon *image.NRGBA, dpi uint32) windows.Handle {
	size := int(getSystemMetricsForDpi(smCxMenuCheck, dpi))
	if size <= 0 {
		size = 16
	}
	img := icon
	if img.Rect.Dx() != size || img.Rect.Dy() != size {
		img = resizeNRGBA(img, size, size)
	}
	bmp, bits := createDIB32(size, size, true)
	writeDIBPixels(bits, img, true)
	return bmp
}

// applyMenuTheme uses uxtheme's undocumented ordinals 135/136 to match the taskbar's dark or
// light mode, as the spike proved; any failure (e.g. an older Windows build) is silently
// ignored, since the menu still works without the matching theme.
func applyMenuTheme(dark bool) {
	mode := int32(3) // ForceLight
	if dark {
		mode = 2 // ForceDark
	}
	addr, err := ordinalAddr(&ordSetPreferredAppMode, 135)
	if err != nil {
		return
	}
	_, _, _ = syscall.SyscallN(addr, uintptr(mode))

	addr, err = ordinalAddr(&ordFlushMenuThemes, 136)
	if err != nil {
		return
	}
	_, _, _ = syscall.SyscallN(addr)
}

func ordinalAddr(cache *uintptr, ordinal uintptr) (uintptr, error) {
	if *cache != 0 {
		return *cache, nil
	}
	if err := uxtheme.Load(); err != nil {
		return 0, err
	}
	addr, err := windows.GetProcAddressByOrdinal(windows.Handle(uxtheme.Handle()), ordinal)
	if err != nil || addr == 0 {
		return 0, err
	}
	*cache = addr
	return addr, nil
}
