package draw

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

type GlyphKind int

const (
	GlyphSpeaker GlyphKind = iota
	GlyphSpeakerMuted
	GlyphMic
	GlyphSun
	GlyphContrast
	GlyphMoon
	GlyphKeyboard
	GlyphZoom
	GlyphApp
	GlyphPlay
	GlyphPause
	GlyphPlayPause
	GlyphStop
	GlyphPreviousTrack
	GlyphNextTrack
	GlyphVolumeUp
	GlyphVolumeDown
	GlyphMicMuted
	GlyphCloseApp
	GlyphGlobe
	GlyphScreen
	GlyphLock
	GlyphPower
	GlyphArrowLeft
	GlyphArrowRight
	GlyphList
	GlyphGear
	GlyphBulb
	GlyphBulbOn
	GlyphBulbOff
)

const strokeWidth = 1.5

func Glyph(kind GlyphKind, px int, c color.Color) *image.NRGBA {
	s := float32(px) / 24
	hw := strokeWidth * s / 2

	var combined *image.Alpha
	add := func(m *image.Alpha) {
		if combined == nil {
			combined = m
			return
		}
		unionMask(combined, m)
	}
	addShape := func(build func(z *vector.Rasterizer)) {
		add(mask(px, build))
	}
	line := func(x0, y0, x1, y1 float32) {
		addShape(func(z *vector.Rasterizer) { addCapsule(z, x0*s, y0*s, x1*s, y1*s, hw) })
	}
	rect := func(x, y, w, h, r float32) {
		addShape(func(z *vector.Rasterizer) { addRoundedRect(z, x*s, y*s, w*s, h*s, r*s) })
	}
	triangle := func(x0, y0, x1, y1, x2, y2 float32) {
		addShape(func(z *vector.Rasterizer) {
			z.MoveTo(x0*s, y0*s)
			z.LineTo(x1*s, y1*s)
			z.LineTo(x2*s, y2*s)
			z.ClosePath()
		})
	}
	mic := func() {
		rect(9, 3, 6, 10, 3)
		addShape(func(z *vector.Rasterizer) { addThickArc(z, 12*s, 8*s, 7*s, hw, 130, 230) })
		line(12, 15, 12, 19)
		line(8, 21, 16, 21)
	}
	bulb := func(lit bool) {
		if lit {
			addShape(func(z *vector.Rasterizer) { addEllipse(z, 12*s, 10*s, 6.5*s, 6.5*s) })
		} else {
			add(ringEllipse(px, 12*s, 10*s, 6.5*s, hw))
		}
		line(9.5, 18.5, 14.5, 18.5)
		line(10.5, 21, 13.5, 21)
	}

	switch kind {
	case GlyphSpeaker, GlyphSpeakerMuted:
		addShape(func(z *vector.Rasterizer) { speakerBody(z, s) })
		if kind == GlyphSpeaker {
			addShape(func(z *vector.Rasterizer) { addThickArc(z, 12*s, 12*s, 3*s, hw, 50, 130) })
			addShape(func(z *vector.Rasterizer) { addThickArc(z, 12*s, 12*s, 5.5*s, hw, 45, 135) })
		} else {
			addShape(func(z *vector.Rasterizer) { addCapsule(z, 15*s, 6*s, 21*s, 12*s, hw) })
			addShape(func(z *vector.Rasterizer) { addCapsule(z, 15*s, 12*s, 21*s, 6*s, hw) })
		}

	case GlyphMic:
		mic()

	case GlyphMicMuted:
		mic()
		line(4, 4, 20, 20)

	case GlyphSun:
		addShape(func(z *vector.Rasterizer) { addEllipse(z, 12*s, 12*s, 3.5*s, 3.5*s) })
		for angle := 0; angle < 360; angle += 45 {
			rad := float64(angle) * math.Pi / 180
			dx, dy := float32(math.Sin(rad)), -float32(math.Cos(rad))
			addShape(func(z *vector.Rasterizer) {
				addCapsule(z, 12*s+dx*6*s, 12*s+dy*6*s, 12*s+dx*9*s, 12*s+dy*9*s, hw)
			})
		}

	case GlyphContrast:
		add(ringEllipse(px, 12*s, 12*s, 9*s, hw))
		addShape(func(z *vector.Rasterizer) { addPieSlice(z, 12*s, 12*s, 9*s, 180, 360) })

	case GlyphMoon:
		full := mask(px, func(z *vector.Rasterizer) { addEllipse(z, 10*s, 12*s, 8*s, 8*s) })
		bite := mask(px, func(z *vector.Rasterizer) { addEllipse(z, 13.5*s, 9.5*s, 7*s, 7*s) })
		cutMask(full, bite)
		add(full)

	case GlyphKeyboard:
		add(ringRoundedRect(px, 3*s, 6*s, 18*s, 12*s, 2*s, hw))
		for _, kx := range []float32{6, 10, 14, 18} {
			x := kx
			addShape(func(z *vector.Rasterizer) { addRoundedRect(z, x*s, 9*s, 2*s, 2*s, 0.5*s) })
		}
		addShape(func(z *vector.Rasterizer) { addRoundedRect(z, 6*s, 13*s, 12*s, 2*s, 1*s) })

	case GlyphZoom:
		add(ringEllipse(px, 10*s, 10*s, 6*s, hw))
		addShape(func(z *vector.Rasterizer) { addCapsule(z, 14.2*s, 14.2*s, 19*s, 19*s, hw) })
		addShape(func(z *vector.Rasterizer) { addCapsule(z, 7.5*s, 10*s, 12.5*s, 10*s, 0.6*s) })
		addShape(func(z *vector.Rasterizer) { addCapsule(z, 10*s, 7.5*s, 10*s, 12.5*s, 0.6*s) })

	case GlyphApp:
		add(ringRoundedRect(px, 4*s, 4*s, 16*s, 16*s, 4*s, hw))
		addShape(func(z *vector.Rasterizer) { addRoundedRect(z, 8*s, 8*s, 8*s, 8*s, 2*s) })

	case GlyphCloseApp:
		add(ringRoundedRect(px, 4*s, 4*s, 16*s, 16*s, 4*s, hw))
		line(9, 9, 15, 15)
		line(15, 9, 9, 15)

	case GlyphPlay:
		triangle(8, 5, 8, 19, 19, 12)

	case GlyphPause:
		rect(7, 5, 3.5, 14, 1)
		rect(13.5, 5, 3.5, 14, 1)

	case GlyphPlayPause:
		triangle(3, 6, 3, 18, 11.5, 12)
		rect(14, 6, 2.5, 12, 0.8)
		rect(18.5, 6, 2.5, 12, 0.8)

	case GlyphStop:
		rect(6, 6, 12, 12, 2)

	case GlyphPreviousTrack:
		rect(5, 6, 2.5, 12, 0.8)
		triangle(19, 6, 19, 18, 8.5, 12)

	case GlyphNextTrack:
		triangle(5, 6, 5, 18, 15.5, 12)
		rect(16.5, 6, 2.5, 12, 0.8)

	case GlyphVolumeUp, GlyphVolumeDown:
		addShape(func(z *vector.Rasterizer) { speakerBody(z, s) })
		line(16, 12, 21, 12)
		if kind == GlyphVolumeUp {
			line(18.5, 9.5, 18.5, 14.5)
		}

	case GlyphGlobe:
		add(ringEllipse(px, 12*s, 12*s, 9*s, hw))
		meridian := mask(px, func(z *vector.Rasterizer) { addEllipse(z, 12*s, 12*s, 4*s+hw, 9*s+hw) })
		cutMask(meridian, mask(px, func(z *vector.Rasterizer) { addEllipse(z, 12*s, 12*s, 4*s-hw, 9*s-hw) }))
		add(meridian)
		line(3.5, 12, 20.5, 12)

	case GlyphScreen:
		add(ringRoundedRect(px, 3*s, 4*s, 18*s, 12*s, 2*s, hw))
		line(12, 16, 12, 19)
		line(8, 20, 16, 20)

	case GlyphLock:
		rect(5, 11, 14, 10, 2)
		addShape(func(z *vector.Rasterizer) { addThickArc(z, 12*s, 9*s, 4*s, hw, -90, 90) })
		line(8, 9, 8, 11)
		line(16, 9, 16, 11)

	case GlyphPower:
		addShape(func(z *vector.Rasterizer) { addThickArc(z, 12*s, 13*s, 7.5*s, hw, 40, 320) })
		line(12, 3.5, 12, 11)

	case GlyphArrowLeft:
		line(5, 12, 19, 12)
		line(5, 12, 10.5, 6.5)
		line(5, 12, 10.5, 17.5)

	case GlyphArrowRight:
		line(5, 12, 19, 12)
		line(19, 12, 13.5, 6.5)
		line(19, 12, 13.5, 17.5)

	case GlyphList:
		for _, y := range []float32{7, 12, 17} {
			line(5, y, 19, y)
		}

	case GlyphGear:
		gear := mask(px, func(z *vector.Rasterizer) { addGear(z, s) })
		cutMask(gear, mask(px, func(z *vector.Rasterizer) { addEllipse(z, 12*s, 12*s, 3.2*s, 3.2*s) }))
		add(gear)

	case GlyphBulb:
		bulb(false)

	case GlyphBulbOn:
		bulb(true)

	case GlyphBulbOff:
		bulb(false)
		line(4, 4, 20, 20)
	}

	out := image.NewNRGBA(image.Rect(0, 0, px, px))
	if combined == nil {
		return out
	}
	fillMask(out, combined, colorToRGBA(c))
	return out
}

func speakerBody(z *vector.Rasterizer, s float32) {
	z.MoveTo(3*s, 9*s)
	z.LineTo(7*s, 9*s)
	z.LineTo(13*s, 4*s)
	z.LineTo(13*s, 20*s)
	z.LineTo(7*s, 15*s)
	z.LineTo(3*s, 15*s)
	z.ClosePath()
}

// addGear outlines a gear of eight teeth around the middle of the 24-unit grid.
func addGear(z *vector.Rasterizer, s float32) {
	const teeth = 8
	first := true
	for i := range teeth {
		a := float64(i) * 360 / teeth
		for _, p := range [][2]float64{{a - 16, 7}, {a - 8, 9.5}, {a + 8, 9.5}, {a + 16, 7}} {
			rad := p[0] * math.Pi / 180
			x, y := float32(12+math.Sin(rad)*p[1])*s, float32(12-math.Cos(rad)*p[1])*s
			if first {
				z.MoveTo(x, y)
				first = false
			} else {
				z.LineTo(x, y)
			}
		}
	}
	z.ClosePath()
}

func addPieSlice(z *vector.Rasterizer, cx, cy, r, startDeg, endDeg float32) {
	dir := func(deg float32) (float32, float32) {
		rad := float64(deg) * math.Pi / 180
		return float32(math.Sin(rad)), -float32(math.Cos(rad))
	}
	sweep := endDeg - startDeg
	steps := int(math.Abs(float64(sweep))/3) + 1
	z.MoveTo(cx, cy)
	for i := 0; i <= steps; i++ {
		d := startDeg + sweep*float32(i)/float32(steps)
		nx, ny := dir(d)
		z.LineTo(cx+nx*r, cy+ny*r)
	}
	z.ClosePath()
}

func ringRoundedRect(px int, x, y, w, h, r, hw float32) *image.Alpha {
	outer := mask(px, func(z *vector.Rasterizer) {
		addRoundedRect(z, x-hw, y-hw, w+2*hw, h+2*hw, r+hw)
	})
	ir := r - hw
	if ir < 0 {
		ir = 0
	}
	inner := mask(px, func(z *vector.Rasterizer) {
		addRoundedRect(z, x+hw, y+hw, w-2*hw, h-2*hw, ir)
	})
	cutMask(outer, inner)
	return outer
}

func ringEllipse(px int, cx, cy, r, hw float32) *image.Alpha {
	outer := mask(px, func(z *vector.Rasterizer) { addEllipse(z, cx, cy, r+hw, r+hw) })
	inner := mask(px, func(z *vector.Rasterizer) { addEllipse(z, cx, cy, r-hw, r-hw) })
	cutMask(outer, inner)
	return outer
}

func colorToRGBA(c color.Color) rgba {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return rgba{float64(n.R), float64(n.G), float64(n.B), float64(n.A) / 255}
}
