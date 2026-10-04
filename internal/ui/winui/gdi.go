//go:build windows

package winui

import (
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procGetObjectW         = gdi32.NewProc("GetObjectW")
	procGetDIBits          = gdi32.NewProc("GetDIBits")
)

// bitmapInfoHeader mirrors BITMAPINFOHEADER; passed where a BITMAPINFO is expected since a
// BI_RGB bitmap carries no color table.
type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

const dibRGBColors = 0

func createCompatibleDC(hdc windows.Handle) windows.Handle {
	r, _, _ := procCreateCompatibleDC.Call(uintptr(hdc))
	return windows.Handle(r)
}

func selectObject(hdc, obj windows.Handle) windows.Handle {
	r, _, _ := procSelectObject.Call(uintptr(hdc), uintptr(obj))
	return windows.Handle(r)
}

func deleteObject(obj windows.Handle) {
	if obj != 0 {
		procDeleteObject.Call(uintptr(obj))
	}
}

func createBitmap(w, h int32, planes, bitCount uint32, bits []byte) windows.Handle {
	var ptr *byte
	if len(bits) > 0 {
		ptr = &bits[0]
	}
	r, _, _ := procCreateBitmap.Call(uintptr(w), uintptr(h), uintptr(planes), uintptr(bitCount), uintptr(unsafe.Pointer(ptr)))
	return windows.Handle(r)
}

// monoMaskBits builds an all-zero AND mask (0 = opaque): the per-pixel alpha channel already
// carries transparency for every bitmap this package builds, so the mask just needs to not hide
// anything. Each scanline pads to a 16-bit boundary, as GDI's 1-bpp DDBs require.
func monoMaskBits(w, h int) []byte {
	rowBytes := ((w + 15) / 16) * 2
	return make([]byte, rowBytes*h)
}

// createDIB32 allocates a top-down (or bottom-up) 32-bpp BI_RGB DIB section and returns both the
// bitmap and a direct pointer to its pixel buffer, which the caller fills in before using the
// bitmap. A screen DC is only needed transiently to pick a compatible format.
func createDIB32(w, h int, topDown bool) (windows.Handle, unsafe.Pointer) {
	hdcScreen := getDC(0)
	defer releaseDC(0, hdcScreen)

	height := int32(h)
	if topDown {
		height = -height
	}
	bi := bitmapInfoHeader{
		size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		width:    int32(w),
		height:   height,
		planes:   1,
		bitCount: 32,
	}
	var bits unsafe.Pointer
	r, _, _ := procCreateDIBSection.Call(uintptr(hdcScreen), uintptr(unsafe.Pointer(&bi)), uintptr(dibRGBColors), uintptr(unsafe.Pointer(&bits)), 0, 0)
	return windows.Handle(r), bits
}

// writeDIBPixels copies an NRGBA image into a top-down 32-bpp BGRA buffer such as createDIB32
// returns. premultiply is required for anything GDI alpha-blends (layered windows, menu
// bitmaps); icon color bitmaps want the straight alpha they were authored with.
func writeDIBPixels(bits unsafe.Pointer, img *image.NRGBA, premultiply bool) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	dst := unsafe.Slice((*byte)(bits), w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := img.PixOffset(img.Rect.Min.X+x, img.Rect.Min.Y+y)
			r, g, b, a := img.Pix[si], img.Pix[si+1], img.Pix[si+2], img.Pix[si+3]
			if premultiply && a != 255 {
				r = byte(uint32(r) * uint32(a) / 255)
				g = byte(uint32(g) * uint32(a) / 255)
				b = byte(uint32(b) * uint32(a) / 255)
			}
			di := (y*w + x) * 4
			dst[di+0] = b
			dst[di+1] = g
			dst[di+2] = r
			dst[di+3] = a
		}
	}
}

// nrgbaFromDIB32 is the inverse of writeDIBPixels, used when reading icons back out of GDI.
func nrgbaFromDIB32(buf []byte, w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := (y*w + x) * 4
			di := img.PixOffset(x, y)
			img.Pix[di+0] = buf[si+2]
			img.Pix[di+1] = buf[si+1]
			img.Pix[di+2] = buf[si+0]
			img.Pix[di+3] = buf[si+3]
		}
	}
	return img
}

// getDIB32 reads a GDI bitmap's pixels back as top-down 32-bpp BGRA, letting GDI itself convert
// from whatever the source bitmap's real depth is (used both for an icon's color bitmap and,
// expanded from 1-bpp, its AND mask).
func getDIB32(hbmp windows.Handle, w, h int) []byte {
	hdc := getDC(0)
	defer releaseDC(0, hdc)
	bi := bitmapInfoHeader{
		size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		width:    int32(w),
		height:   -int32(h),
		planes:   1,
		bitCount: 32,
	}
	buf := make([]byte, w*h*4)
	procGetDIBits.Call(uintptr(hdc), uintptr(hbmp), 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), uintptr(dibRGBColors))
	return buf
}

// bitmapDims mirrors enough of BITMAP (via GetObjectW) to recover a GDI bitmap's own size.
type bitmapDims struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

func bitmapSize(hbmp windows.Handle) (int, int) {
	var bm bitmapDims
	procGetObjectW.Call(uintptr(hbmp), unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	return int(bm.bmWidth), int(bm.bmHeight)
}
