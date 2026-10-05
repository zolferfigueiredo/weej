package draw

import (
	"image"
	"math"
)

// Three box blurs approximate the Gaussian CoreGraphics' shadow blur would produce, per the spec.
func boxBlur3(buf []float64, w, h int, sigma float64) {
	r := boxRadius(sigma)
	if r <= 0 {
		return
	}
	tmp := make([]float64, w*h)
	for i := 0; i < 3; i++ {
		boxBlur1D(buf, tmp, w, h, r, true)
		boxBlur1D(tmp, buf, w, h, r, false)
	}
}

// Splitting the target variance evenly across three passes gives this box half-width.
func boxRadius(sigma float64) int {
	if sigma <= 0 {
		return 0
	}
	v := 4*sigma*sigma + 1
	return int((math.Sqrt(v)-1)/2 + 0.5)
}

func boxBlur1D(src, dst []float64, w, h, r int, horizontal bool) {
	if horizontal {
		for y := 0; y < h; y++ {
			boxBlurLine(src[y*w:y*w+w], dst[y*w:y*w+w], r)
		}
		return
	}
	col := make([]float64, h)
	out := make([]float64, h)
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			col[y] = src[y*w+x]
		}
		boxBlurLine(col, out, r)
		for y := 0; y < h; y++ {
			dst[y*w+x] = out[y]
		}
	}
}

// A running sum keeps this O(n) instead of O(n*r).
func boxBlurLine(src, dst []float64, r int) {
	n := len(src)
	if n == 0 {
		return
	}
	get := func(i int) float64 {
		if i < 0 {
			i = 0
		} else if i >= n {
			i = n - 1
		}
		return src[i]
	}
	width := float64(2*r + 1)
	sum := 0.0
	for i := -r; i <= r; i++ {
		sum += get(i)
	}
	for i := 0; i < n; i++ {
		dst[i] = sum / width
		sum += get(i+r+1) - get(i-r)
	}
}

// Unclipped: callers must paint the shape's own fill afterward to cover the shadow under it, as
// CoreGraphics does when a shadow is active while filling.
func dropShadow(dst *image.NRGBA, shapeMask *image.Alpha, dyPx, blurPx float64, c rgba) {
	w, h := shapeMask.Rect.Dx(), shapeMask.Rect.Dy()
	buf := make([]float64, w*h)
	for i, v := range shapeMask.Pix {
		buf[i] = float64(v)
	}
	shifted := shiftVertical(buf, w, h, dyPx)
	boxBlur3(shifted, w, h, blurPx/2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cov := shifted[y*w+x] / 255
			if cov > 0 {
				blendPixel(dst, x, y, c, cov)
			}
		}
	}
}

func shiftVertical(buf []float64, w, h int, dyPx float64) []float64 {
	dy := int(math.Round(dyPx))
	out := make([]float64, w*h)
	for y := 0; y < h; y++ {
		sy := y - dy
		if sy < 0 || sy >= h {
			continue
		}
		copy(out[y*w:y*w+w], buf[sy*w:sy*w+w])
	}
	return out
}
