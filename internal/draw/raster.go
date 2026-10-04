package draw

import (
	"image"

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

func fillMaskClipped(dst *image.NRGBA, m, clip *image.Alpha, c rgba) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := m.PixOffset(x, y)
			cov := float64(m.Pix[i]) / 255 * float64(clip.Pix[i]) / 255
			if cov > 0 {
				blendPixel(dst, x, y, c, cov)
			}
		}
	}
}

// Stops are evenly spaced, matching CGGradient's default when TheeJ passes no explicit locations.
func paintGradient(dst *image.NRGBA, m *image.Alpha, minY, maxY float64, stops []rgba) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	for y := 0; y < h; y++ {
		t := 0.0
		if maxY > minY {
			t = (float64(y) - minY) / (maxY - minY)
		}
		c := gradientColor(stops, clampF(t, 0, 1))
		for x := 0; x < w; x++ {
			cov := float64(m.Pix[m.PixOffset(x, y)]) / 255
			if cov > 0 {
				blendPixel(dst, x, y, c, cov)
			}
		}
	}
}

func gradientColor(stops []rgba, t float64) rgba {
	n := len(stops)
	if n == 1 {
		return stops[0]
	}
	seg := t * float64(n-1)
	i := int(seg)
	if i >= n-1 {
		i = n - 2
	}
	f := seg - float64(i)
	a, b := stops[i], stops[i+1]
	return rgba{lerp(a.r, b.r, f), lerp(a.g, b.g, f), lerp(a.b, b.b, f), lerp(a.a, b.a, f)}
}
