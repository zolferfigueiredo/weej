package draw

import (
	"image"
	"math"

	"golang.org/x/image/vector"
)

// Below this size the plate grain and shadows alias badly rendered directly, so AppIcon
// supersamples at 4x and box-downsamples instead.
const appIconSupersampleBelow = 48

func AppIcon(px int) *image.NRGBA {
	work := px
	if px <= appIconSupersampleBelow {
		work = px * 4
	}
	img := renderAppIcon(work)
	if work != px {
		img = boxDownsample(img, px)
	}
	return img
}

func renderAppIcon(work int) *image.NRGBA {
	s := float64(work) / 1024
	p := func(v float64) float32 { return float32(v * s) }

	canvas := image.NewNRGBA(image.Rect(0, 0, work, work))

	tile := mask(work, func(z *vector.Rasterizer) {
		addRoundedRect(z, p(100), p(100), p(824), p(824), p(185))
	})

	paintPlate(canvas, tile, work, s)
	innerShadow(canvas, tile, 16*s, 34*s, grayColor(0, 0.2))
	innerShadow(canvas, tile, -5*s, 6*s, grayColor(1, 0.9))

	faders := [3][2]float64{{322, 360}, {512, 610}, {702, 470}}
	for _, f := range faders {
		drawFader(canvas, work, s, f[0], f[1])
	}

	drawLED(canvas, work, s)
	return canvas
}

func paintPlate(canvas *image.NRGBA, tile *image.Alpha, work int, s float64) {
	grain := plateGrain(work)
	top := [3]float64{0xf1, 0xec, 0xe2}
	bottom := [3]float64{0xdd, 0xd5, 0xc6}
	for row := 0; row < work; row++ {
		logicalY := float64(row) / s
		t := smoothstep((logicalY - 100) / 824)
		base := [3]float64{
			lerp(top[0], bottom[0], t),
			lerp(top[1], bottom[1], t),
			lerp(top[2], bottom[2], t),
		}
		for col := 0; col < work; col++ {
			i := row*work + col
			cov := float64(tile.Pix[i]) / 255
			if cov <= 0 {
				continue
			}
			g := grain[i]
			c := rgba{clampF(base[0]+g, 0, 255), clampF(base[1]+g, 0, 255), clampF(base[2]+g, 0, 255), 1}
			blendPixel(canvas, col, row, c, cov)
		}
	}
}

// Walking bottom row first, left to right (not top-down) reproduces TheeJ's exact LCG sequence,
// so the grain pattern matches pixel-for-pixel.
func plateGrain(work int) []float64 {
	grain := make([]float64, work*work)
	var seed uint64 = 1
	for row := work - 1; row >= 0; row-- {
		base := row * work
		for col := 0; col < work; col++ {
			seed = seed*6364136223846793005 + 1442695040888963407
			top8 := float64(seed >> 56)
			grain[base+col] = (top8 - 128) * 0.04
		}
	}
	return grain
}

func drawFader(canvas *image.NRGBA, work int, s, x, knobY float64) {
	p := func(v float64) float32 { return float32(v * s) }
	pf := func(v float64) float64 { return v * s }

	slot := mask(work, func(z *vector.Rasterizer) {
		addRoundedRect(z, p(x-17), p(250), p(34), p(540), p(17))
	})
	fillMask(canvas, slot, hexColor(0x161514, 1))
	innerShadow(canvas, slot, -10*s, 14*s, grayColor(0, 1))

	rimHighlight := ringRoundedRect(work, p(x-17), p(252), p(34), p(540), p(17), p(1.5))
	fillMask(canvas, rimHighlight, grayColor(1, 0.35))

	for y := 270.0; y <= 770; y += 100 {
		tick := mask(work, func(z *vector.Rasterizer) {
			addCapsule(z, p(x+58), p(y), p(x+84), p(y), p(4))
		})
		fillMask(canvas, tick, hexColor(0xa39d92, 1))
	}

	capX, capY, capW, capH := x-66, knobY-38, 132.0, 76.0
	body := mask(work, func(z *vector.Rasterizer) {
		addRoundedRect(z, p(capX), p(capY), p(capW), p(capH), p(15.2))
	})
	dropShadow(canvas, body, -12*s, 24*s, grayColor(0, 0.55))
	paintGradient(canvas, body, pf(capY), pf(capY+capH),
		[]rgba{grayColor(0.24, 1), grayColor(0.1, 1), grayColor(0.05, 1)})

	inner := mask(work, func(z *vector.Rasterizer) {
		addRoundedRect(z, p(capX+5.3), p(capY+6.1), p(capW-10.6), p(capH-12.2), p(10.6))
	})
	paintGradient(canvas, inner, pf(capY+6.1), pf(capY+capH-6.1), []rgba{grayColor(0.2, 1), grayColor(0.08, 1)})

	sheen := mask(work, func(z *vector.Rasterizer) {
		addCapsule(z, p(capX+15), p(capY+2), p(capX+capW-15), p(capY+2), p(2))
	})
	fillMaskClipped(canvas, sheen, body, grayColor(1, 0.18))

	grip := mask(work, func(z *vector.Rasterizer) {
		addRoundedRect(z, p(x-39.6), p(knobY-4.6), p(79.2), p(9.2), p(4.6))
	})
	fillMask(canvas, grip, hexColor(0xebe5d9, 1))
}

func drawLED(canvas *image.NRGBA, work int, s float64) {
	ledX, ledY := 212*s, 212*s

	for y := 0; y < work; y++ {
		for x := 0; x < work; x++ {
			dx, dy := float64(x)+0.5-ledX, float64(y)+0.5-ledY
			dist := math.Hypot(dx, dy)
			if dist < 13*s || dist > 70*s {
				continue
			}
			t := (dist - 13*s) / (70*s - 13*s)
			blendPixel(canvas, x, y, hexColor(0xff5a1f, lerp(0.55, 0, t)), 1)
		}
	}

	c0x, c0y := ledX-6.6*s, ledY-7.7*s
	r1 := 22 * s
	dcx, dcy := ledX-c0x, ledY-c0y
	a := dcx*dcx + dcy*dcy - r1*r1
	stops := []rgba{hexColor(0xffd999, 1), hexColor(0xff5a1f, 1), hexColor(0xb33c0c, 1)}
	locs := []float64{0, 0.45, 1}

	for y := 0; y < work; y++ {
		for x := 0; x < work; x++ {
			px, py := float64(x)+0.5-ledX, float64(y)+0.5-ledY
			if px*px+py*py > r1*r1 {
				continue
			}
			t := bulbGradientT(float64(x)+0.5-c0x, float64(y)+0.5-c0y, dcx, dcy, a, r1)
			blendPixel(canvas, x, y, stopColor(stops, locs, t), 1)
		}
	}
}

// CG's two-circle radial gradient (start a point, end circle offset by (dcx,dcy), radius r1) puts
// each pixel at the smallest t where a circle growing from the point to the end circle reaches it;
// solving |Q-tD|=t*r1 for t gives the quadratic below. A pixel the sweep never reaches (no root in
// [0,1]) takes the gradient's last color, matching CG's drawsAfterEndLocation.
func bulbGradientT(qx, qy, dcx, dcy, a, r1 float64) float64 {
	qd := qx*dcx + qy*dcy
	qq := qx*qx + qy*qy
	disc := qd*qd - a*qq
	if disc < 0 {
		return 1
	}
	sq := math.Sqrt(disc)
	best := -1.0
	for _, root := range [2]float64{(qd - sq) / a, (qd + sq) / a} {
		if root >= -1e-9 && root <= 1 && (best < 0 || root < best) {
			best = root
		}
	}
	if best < 0 {
		return 1
	}
	return clampF(best, 0, 1)
}

func stopColor(stops []rgba, locs []float64, t float64) rgba {
	if t <= locs[0] {
		return stops[0]
	}
	for i := 1; i < len(locs); i++ {
		if t <= locs[i] {
			f := (t - locs[i-1]) / (locs[i] - locs[i-1])
			a, b := stops[i-1], stops[i]
			return rgba{lerp(a.r, b.r, f), lerp(a.g, b.g, f), lerp(a.b, b.b, f), lerp(a.a, b.a, f)}
		}
	}
	return stops[len(stops)-1]
}
