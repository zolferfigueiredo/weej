package draw

import (
	"image"
	"math"

	"golang.org/x/image/vector"
)

func mask(px int, build func(z *vector.Rasterizer)) *image.Alpha {
	return maskRect(px, px, build)
}

func maskRect(w, h int, build func(z *vector.Rasterizer)) *image.Alpha {
	z := vector.NewRasterizer(w, h)
	build(z)
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// TheeJ's template icons "cut" shapes by clearing alpha; the spec ports that as a direct
// subtraction rather than Porter-Duff compositing.
func cutMask(dst, src *image.Alpha) {
	for i := range dst.Pix {
		v := int(dst.Pix[i]) - int(src.Pix[i])
		if v < 0 {
			v = 0
		}
		dst.Pix[i] = uint8(v)
	}
}

func overAlpha(dst, src *image.Alpha, alpha float64) {
	for i, v := range src.Pix {
		sa := float64(v) / 255 * alpha
		da := float64(dst.Pix[i]) / 255
		out := sa + da*(1-sa)
		dst.Pix[i] = clamp255(out * 255)
	}
}

// Combines as independent coverages (1-(1-a)(1-b)) rather than max(), which leaves a faint seam
// where two anti-aliased edges partially overlap.
func unionMask(dst, src *image.Alpha) {
	for i, v := range src.Pix {
		a := float64(dst.Pix[i]) / 255
		b := float64(v) / 255
		dst.Pix[i] = clamp255((1 - (1-a)*(1-b)) * 255)
	}
}

func blendPixel(dst *image.NRGBA, x, y int, c rgba, coverage float64) {
	if coverage <= 0 || c.a <= 0 {
		return
	}
	i := dst.PixOffset(x, y)
	sa := c.a * coverage
	if sa > 1 {
		sa = 1
	}
	dr, dg, db := float64(dst.Pix[i]), float64(dst.Pix[i+1]), float64(dst.Pix[i+2])
	da := float64(dst.Pix[i+3]) / 255
	outA := sa + da*(1-sa)
	var outR, outG, outB float64
	if outA > 0 {
		outR = (c.r*sa + dr*da*(1-sa)) / outA
		outG = (c.g*sa + dg*da*(1-sa)) / outA
		outB = (c.b*sa + db*da*(1-sa)) / outA
	}
	dst.Pix[i+0] = clamp255(outR)
	dst.Pix[i+1] = clamp255(outG)
	dst.Pix[i+2] = clamp255(outB)
	dst.Pix[i+3] = clamp255(outA * 255)
}

func blitOver(dst, src *image.NRGBA, originX, originY int) {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := src.PixOffset(x, y)
			a := float64(src.Pix[i+3]) / 255
			if a <= 0 {
				continue
			}
			c := rgba{float64(src.Pix[i]), float64(src.Pix[i+1]), float64(src.Pix[i+2]), a}
			blendPixel(dst, originX+x, originY+y, c, 1)
		}
	}
}

func fillMask(dst *image.NRGBA, m *image.Alpha, c rgba) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cov := float64(m.Pix[m.PixOffset(x, y)]) / 255
			if cov > 0 {
				blendPixel(dst, x, y, c, cov)
			}
		}
	}
}

// vector.Rasterizer only fills, and capsules overlapping in one rasterizer add up their edge
// coverage, which hardens the anti-aliasing; a per-pixel distance gives clean round joins.
func strokeMask(px int, line polyline, hw float64) *image.Alpha {
	m := image.NewAlpha(image.Rect(0, 0, px, px))
	for i := 1; i < len(line); i++ {
		a, b := line[i-1], line[i]
		x0 := max(0, int(math.Floor(min(a.x, b.x)-hw-1)))
		x1 := min(px, int(math.Ceil(max(a.x, b.x)+hw+1)))
		y0 := max(0, int(math.Floor(min(a.y, b.y)-hw-1)))
		y1 := min(px, int(math.Ceil(max(a.y, b.y)+hw+1)))
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				d := segmentDistance(float64(x)+0.5, float64(y)+0.5, a, b)
				v := clamp255(clampF(hw+0.5-d, 0, 1) * 255)
				if j := m.PixOffset(x, y); v > m.Pix[j] {
					m.Pix[j] = v
				}
			}
		}
	}
	return m
}
