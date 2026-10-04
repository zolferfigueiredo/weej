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
		addShape(func(z *vector.Rasterizer) { addRoundedRect(z, 9*s, 3*s, 6*s, 10*s, 3*s) })
		addShape(func(z *vector.Rasterizer) { addThickArc(z, 12*s, 8*s, 7*s, hw, 130, 230) })
		addShape(func(z *vector.Rasterizer) { addCapsule(z, 12*s, 15*s, 12*s, 19*s, hw) })
		addShape(func(z *vector.Rasterizer) { addCapsule(z, 8*s, 21*s, 16*s, 21*s, hw) })

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
