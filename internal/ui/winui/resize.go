//go:build windows

package winui

// Menu icons and extracted exe icons both need resizing to an arbitrary target size; the
// equivalent in internal/draw is unexported, so this package carries its own small copy rather
// than widening that package's surface for one caller.

import (
	"image"
	"math"
)

func resizeNRGBA(src *image.NRGBA, w, h int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if sw == 0 || sh == 0 || w <= 0 || h <= 0 {
		return dst
	}
	if sw == w && sh == h {
		copy(dst.Pix, src.Pix)
		return dst
	}
	for y := 0; y < h; y++ {
		sy := (float64(y)+0.5)*float64(sh)/float64(h) - 0.5
		for x := 0; x < w; x++ {
			sx := (float64(x)+0.5)*float64(sw)/float64(w) - 0.5
			r, g, b, a := sampleBilinear(src, sx, sy)
			di := dst.PixOffset(x, y)
			dst.Pix[di], dst.Pix[di+1], dst.Pix[di+2], dst.Pix[di+3] = r, g, b, a
		}
	}
	return dst
}

func sampleBilinear(src *image.NRGBA, x, y float64) (uint8, uint8, uint8, uint8) {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	get := func(xi, yi int) (float64, float64, float64, float64) {
		if xi < 0 {
			xi = 0
		} else if xi >= sw {
			xi = sw - 1
		}
		if yi < 0 {
			yi = 0
		} else if yi >= sh {
			yi = sh - 1
		}
		i := src.PixOffset(xi, yi)
		return float64(src.Pix[i]), float64(src.Pix[i+1]), float64(src.Pix[i+2]), float64(src.Pix[i+3])
	}
	r00, g00, b00, a00 := get(x0, y0)
	r10, g10, b10, a10 := get(x0+1, y0)
	r01, g01, b01, a01 := get(x0, y0+1)
	r11, g11, b11, a11 := get(x0+1, y0+1)
	blend := func(v00, v10, v01, v11 float64) float64 {
		top := v00 + (v10-v00)*fx
		bot := v01 + (v11-v01)*fx
		return top + (bot-top)*fy
	}
	return clamp255(blend(r00, r10, r01, r11)), clamp255(blend(g00, g10, g01, g11)),
		clamp255(blend(b00, b10, b01, b11)), clamp255(blend(a00, a10, a01, a11))
}

func clamp255(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}
