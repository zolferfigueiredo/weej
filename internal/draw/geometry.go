package draw

import (
	"math"

	"golang.org/x/image/vector"
)

// kappa is the standard cubic-Bezier control-point factor for a quarter circle.
const kappa = 0.5522847498307936

type rgba struct {
	r, g, b float64 // 0..255
	a       float64 // 0..1
}

func hexColor(hex uint32, a float64) rgba {
	return rgba{float64(hex >> 16 & 0xff), float64(hex >> 8 & 0xff), float64(hex & 0xff), a}
}

func grayColor(white, a float64) rgba {
	return rgba{white * 255, white * 255, white * 255, a}
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func smoothstep(t float64) float64 {
	t = clampF(t, 0, 1)
	return t * t * (3 - 2*t)
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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

func addRoundedRect(z *vector.Rasterizer, x, y, w, h, r float32) {
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	k := r * kappa
	z.MoveTo(x+r, y)
	z.LineTo(x+w-r, y)
	z.CubeTo(x+w-r+k, y, x+w, y+r-k, x+w, y+r)
	z.LineTo(x+w, y+h-r)
	z.CubeTo(x+w, y+h-r+k, x+w-r+k, y+h, x+w-r, y+h)
	z.LineTo(x+r, y+h)
	z.CubeTo(x+r-k, y+h, x, y+h-r+k, x, y+h-r)
	z.LineTo(x, y+r)
	z.CubeTo(x, y+r-k, x+r-k, y, x+r, y)
	z.ClosePath()
}

func addEllipse(z *vector.Rasterizer, cx, cy, rx, ry float32) {
	kx, ky := rx*kappa, ry*kappa
	z.MoveTo(cx+rx, cy)
	z.CubeTo(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry)
	z.CubeTo(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy)
	z.CubeTo(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry)
	z.CubeTo(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy)
	z.ClosePath()
}

// vector.Rasterizer only fills, so a round-capped stroked line has to be built as a polygon.
func addCapsule(z *vector.Rasterizer, x0, y0, x1, y1, hw float32) {
	dx, dy := x1-x0, y1-y0
	length := float32(math.Hypot(float64(dx), float64(dy)))
	if length < 1e-6 {
		addEllipse(z, x0, y0, hw, hw)
		return
	}
	ux, uy := dx/length, dy/length
	nx, ny := -uy, ux
	const segs = 16
	z.MoveTo(x0+nx*hw, y0+ny*hw)
	z.LineTo(x1+nx*hw, y1+ny*hw)
	for i := 1; i <= segs; i++ {
		a := math.Pi * float64(i) / float64(segs)
		ca, sa := float32(math.Cos(a)), float32(math.Sin(a))
		z.LineTo(x1+hw*(nx*ca+ux*sa), y1+hw*(ny*ca+uy*sa))
	}
	z.LineTo(x0-nx*hw, y0-ny*hw)
	for i := 1; i <= segs; i++ {
		a := math.Pi * float64(i) / float64(segs)
		ca, sa := float32(math.Cos(a)), float32(math.Sin(a))
		z.LineTo(x0+hw*(-nx*ca-ux*sa), y0+hw*(-ny*ca-uy*sa))
	}
	z.ClosePath()
}

// Angles are degrees clockwise from 12 o'clock, TheeJ's dial convention; in this y-down space
// that direction is (sin(deg), -cos(deg)).
func addThickArc(z *vector.Rasterizer, cx, cy, r, hw, startDeg, endDeg float32) {
	dir := func(deg float32) (float32, float32) {
		rad := float64(deg) * math.Pi / 180
		return float32(math.Sin(rad)), -float32(math.Cos(rad))
	}
	sweep := endDeg - startDeg
	sign := float32(1)
	if sweep < 0 {
		sign = -1
	}
	steps := int(math.Abs(float64(sweep))/2) + 1
	const capSegs = 12

	for i := 0; i <= steps; i++ {
		d := startDeg + sweep*float32(i)/float32(steps)
		nx, ny := dir(d)
		p := [2]float32{cx + nx*(r+hw), cy + ny*(r+hw)}
		if i == 0 {
			z.MoveTo(p[0], p[1])
		} else {
			z.LineTo(p[0], p[1])
		}
	}

	nx, ny := dir(endDeg)
	tx, ty := dir(endDeg + 90*sign)
	ccx, ccy := cx+nx*r, cy+ny*r
	for i := 1; i <= capSegs; i++ {
		a := math.Pi * float64(i) / float64(capSegs)
		ca, sa := float32(math.Cos(a)), float32(math.Sin(a))
		z.LineTo(ccx+hw*(nx*ca+tx*sa), ccy+hw*(ny*ca+ty*sa))
	}

	for i := 1; i <= steps; i++ {
		d := endDeg - sweep*float32(i)/float32(steps)
		inx, iny := dir(d)
		z.LineTo(cx+inx*(r-hw), cy+iny*(r-hw))
	}

	nx0, ny0 := dir(startDeg)
	tx0, ty0 := dir(startDeg + 90*sign)
	ccx0, ccy0 := cx+nx0*r, cy+ny0*r
	for i := 1; i <= capSegs; i++ {
		a := math.Pi * float64(i) / float64(capSegs)
		ca, sa := float32(math.Cos(a)), float32(math.Sin(a))
		z.LineTo(ccx0+hw*(-nx0*ca-tx0*sa), ccy0+hw*(-ny0*ca-ty0*sa))
	}
	z.ClosePath()
}
