package draw

import (
	"image"
	"math"
)

// Averages in alpha-weighted (premultiplied) space so transparent supersamples at an edge don't
// darken the surviving opaque pixel.
func boxDownsample(src *image.NRGBA, dstSize int) *image.NRGBA {
	srcSize := src.Rect.Dx()
	factor := srcSize / dstSize
	dst := image.NewNRGBA(image.Rect(0, 0, dstSize, dstSize))
	n := float64(factor * factor)
	for y := 0; y < dstSize; y++ {
		for x := 0; x < dstSize; x++ {
			var rs, gs, bs, as float64
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					i := src.PixOffset(x*factor+dx, y*factor+dy)
					a := float64(src.Pix[i+3])
					rs += float64(src.Pix[i+0]) * a
					gs += float64(src.Pix[i+1]) * a
					bs += float64(src.Pix[i+2]) * a
					as += a
				}
			}
			j := dst.PixOffset(x, y)
			if as > 0 {
				dst.Pix[j+0] = clamp255(rs / as)
				dst.Pix[j+1] = clamp255(gs / as)
				dst.Pix[j+2] = clamp255(bs / as)
			}
			dst.Pix[j+3] = clamp255(as / n)
		}
	}
	return dst
}

func resizeBilinear(src *image.NRGBA, w, h int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if sw == 0 || sh == 0 || w == 0 || h == 0 {
		return dst
	}
	for y := 0; y < h; y++ {
		sy := (float64(y)+0.5)*float64(sh)/float64(h) - 0.5
		for x := 0; x < w; x++ {
			sx := (float64(x)+0.5)*float64(sw)/float64(w) - 0.5
			i := dst.PixOffset(x, y)
			r, g, b, a := sampleBilinear(src, sx, sy)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = r, g, b, a
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
