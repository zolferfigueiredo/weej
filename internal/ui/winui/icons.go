//go:build windows

package winui

import (
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procExtractIconExW = shell32.NewProc("ExtractIconExW")
	procSHGetFileInfoW = shell32.NewProc("SHGetFileInfoW")
	procGetIconInfo    = user32.NewProc("GetIconInfo")
)

const (
	shgfiIcon      = 0x000000100
	shgfiLargeIcon = 0x00000000
)

// shFileInfoW mirrors SHFILEINFOW.
type shFileInfoW struct {
	hIcon         uintptr
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

func ExeIcon(path string, px int) *image.NRGBA {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil
	}

	var large, small uintptr
	n, _, _ := procExtractIconExW.Call(uintptr(unsafe.Pointer(pathPtr)), 0, uintptr(unsafe.Pointer(&large)), uintptr(unsafe.Pointer(&small)), 1)

	hicon := windows.Handle(large)
	if n == 0 || hicon == 0 {
		if small != 0 {
			procDestroyIcon.Call(small)
		}
		hicon = shellFileIcon(pathPtr)
		if hicon == 0 {
			return nil
		}
	} else if small != 0 {
		procDestroyIcon.Call(small)
	}
	defer procDestroyIcon.Call(uintptr(hicon))

	img := nrgbaFromHICON(hicon)
	if img == nil {
		return nil
	}
	if img.Rect.Dx() != px || img.Rect.Dy() != px {
		img = resizeNRGBA(img, px, px)
	}
	return img
}

func shellFileIcon(pathPtr *uint16) windows.Handle {
	var info shFileInfoW
	r, _, _ := procSHGetFileInfoW.Call(uintptr(unsafe.Pointer(pathPtr)), 0, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), uintptr(shgfiIcon|shgfiLargeIcon))
	if r == 0 {
		return 0
	}
	return windows.Handle(info.hIcon)
}

// nrgbaFromHICON reads an HICON's bitmaps back via GetIconInfo+GetDIBits. Modern exe icons carry
// their own alpha channel; the rare legacy icon that does not (every extracted alpha byte comes
// back zero) falls back to deriving alpha from the AND mask instead.
func nrgbaFromHICON(hicon windows.Handle) *image.NRGBA {
	var info iconInfo
	r, _, _ := procGetIconInfo.Call(uintptr(hicon), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return nil
	}
	hbmColor := windows.Handle(info.hbmColor)
	hbmMask := windows.Handle(info.hbmMask)
	defer deleteObject(hbmColor)
	defer deleteObject(hbmMask)

	w, h := bitmapSize(hbmColor)
	if w <= 0 || h <= 0 {
		return nil
	}

	colorBuf := getDIB32(hbmColor, w, h)
	if !hasAlpha(colorBuf) {
		maskBuf := getDIB32(hbmMask, w, h)
		applyMaskAlpha(colorBuf, maskBuf)
	}
	return nrgbaFromDIB32(colorBuf, w, h)
}

func hasAlpha(buf []byte) bool {
	for i := 3; i < len(buf); i += 4 {
		if buf[i] != 0 {
			return true
		}
	}
	return false
}

// applyMaskAlpha reads the AND mask expanded to 32-bpp: GDI maps mask bit 0 (opaque) to black
// and bit 1 (transparent) to white, so the blue channel alone tells the two apart.
func applyMaskAlpha(colorBuf, maskBuf []byte) {
	for i := 0; i+3 < len(colorBuf) && i+2 < len(maskBuf); i += 4 {
		if maskBuf[i] == 0 {
			colorBuf[i+3] = 255
		} else {
			colorBuf[i+3] = 0
		}
	}
}
