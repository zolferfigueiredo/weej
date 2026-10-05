package draw

import (
	"image"
	"math"

	"golang.org/x/image/vector"
)

type IconStyle string

const (
	StyleMixer IconStyle = "mixer"
	StyleDial  IconStyle = "dial"
	StyleApp   IconStyle = "app"
)

func TrayIcon(style IconStyle, connected bool, px int, lightTaskbar bool) *image.NRGBA {
	if style == StyleApp {
		return AppIcon(px)
	}

	var m *image.Alpha
	if style == StyleMixer {
		m = mixerMask(px, !connected)
	} else {
		m = dialMask(px, connected)
	}

	ink := uint8(255)
	if lightTaskbar {
		ink = 0
	}
	out := image.NewNRGBA(image.Rect(0, 0, px, px))
	for i, a := range m.Pix {
		j := i * 4
		out.Pix[j], out.Pix[j+1], out.Pix[j+2], out.Pix[j+3] = ink, ink, ink, a
	}
	return out
}

var trayLayout = faderLayout{left: 3, gap: 4.5, high: 5, low: 18, mid: 10.1, strokeHW: 1.1,
	capW: 3.6, capH: 2.6, capR: 0.8}

// Parked flattens the W into a line, so the glyph only spells W while the box is connected.
func mixerMask(px int, parked bool) *image.Alpha {
	s := float64(px) / 24
	l := trayLayout
	caps := l.capCentres(parked)
	m := strokeMask(px, wCurve(caps).scaled(s), l.strokeHW*s)
	body := mask(px, func(z *vector.Rasterizer) {
		for _, c := range caps {
			addRoundedRect(z, float32((c.x-l.capW/2)*s), float32((c.y-l.capH/2)*s),
				float32(l.capW*s), float32(l.capH*s), float32(l.capR*s))
		}
	})
	unionMask(m, body)
	return m
}

func dialMask(px int, connected bool) *image.Alpha {
	s := float32(px) / 24
	cx, cy := 12*s, 13.17*s
	radius := float32(9.67) * s
	hw := float32(1.33) * s / 2
	pointer := float32(-135)
	if connected {
		pointer = 45
	}

	base := mask(px, func(z *vector.Rasterizer) {
		addThickArc(z, cx, cy, radius, hw, -135, 135)
	})
	for i, v := range base.Pix {
		base.Pix[i] = clamp255(float64(v) * 0.35)
	}

	if connected {
		lit := mask(px, func(z *vector.Rasterizer) {
			addThickArc(z, cx, cy, radius, hw, -135, pointer)
		})
		overAlpha(base, lit, 1)
	}

	knob := mask(px, func(z *vector.Rasterizer) {
		addEllipse(z, cx, cy, 6.33*s, 6.33*s)
	})
	overAlpha(base, knob, 1)

	rad := float64(pointer) * math.Pi / 180
	dx, dy := float32(math.Sin(rad)), -float32(math.Cos(rad))
	notch := mask(px, func(z *vector.Rasterizer) {
		addCapsule(z, cx+dx*1.67*s, cy+dy*1.67*s, cx+dx*4.33*s, cy+dy*4.33*s, float32(1.6)*s/2)
	})
	cutMask(base, notch)
	return base
}
