package draw

import (
	"image"

	"golang.org/x/image/vector"
)

// Below this size the thin tracks and cap shadows alias badly rendered directly, so AppIcon
// supersamples at 4x and box-downsamples instead.
const appIconSupersampleBelow = 48

// Below this size the J cable crowds the W, so small icons draw the W alone on a full-frame plate.
const appIconCableFrom = 32

const plateLipDepth = 41

var (
	plateLip  = hexColor(0xd6ccba, 1)
	plateFace = hexColor(0xf1ece2, 1)
	plateRim  = hexColor(0xd3c9b7, 1)
	trackInk  = hexColor(0x24201d, 1)
	ledOrange = hexColor(0xff5a1f, 1)
	capInk    = hexColor(0x35302b, 1)
	gripInk   = hexColor(0xe9e2d5, 1)
	plugInk   = hexColor(0x2e2a26, 1)
)

// In design-grid units of 1024.
type faderLayout struct {
	left, gap        float64 // first track's centre x, distance between track centres
	top, bottom      float64
	trackW           float64
	high, low, mid   float64 // cap centre y at the two outer tops, the two valleys, the middle peak
	strokeHW         float64
	capW, capH, capR float64
	gripW, gripH     float64
}

var (
	cabledLayout = faderLayout{left: 148, gap: 120.5, top: 184, bottom: 737, trackW: 31,
		high: 246, low: 655, mid: 410, strokeHW: 23, capW: 96, capH: 72, capR: 26, gripW: 49, gripH: 12}
	smallLayout = faderLayout{left: 205, gap: 153.5, top: 164, bottom: 860, trackW: 36,
		high: 266, low: 737, mid: 451, strokeHW: 28, capW: 123, capH: 82, capR: 31, gripW: 61, gripH: 14}
)

func AppIcon(px int) *image.NRGBA {
	work := px
	if px <= appIconSupersampleBelow {
		work = px * 4
	}
	var img *image.NRGBA
	if px < appIconCableFrom {
		img = renderSmallAppIcon(work)
	} else {
		img = renderAppIcon(work)
	}
	if work != px {
		img = boxDownsample(img, px)
	}
	return img
}

func renderAppIcon(work int) *image.NRGBA {
	s := float64(work) / 1024
	p := func(v float64) float32 { return float32(v * s) }
	canvas := image.NewNRGBA(image.Rect(0, 0, work, work))

	// Painted before the box, which hides where the cable starts.
	cable := polyline{{717, 451}}
	cable.lineTo(819, 451)
	cable.quadTo(pt{901, 451}, pt{901, 532})
	cable.lineTo(901, 840)
	cable.quadTo(pt{901, 973}, pt{819, 973})
	cable.quadTo(pt{748, 973}, pt{748, 901})
	fillMask(canvas, strokeMask(work, cable.scaled(s), 31*s), ledOrange)
	plug := mask(work, func(z *vector.Rasterizer) {
		addRoundedRect(z, p(717), p(809), p(61), p(92), p(15))
	})
	fillMask(canvas, plug, plugInk)

	paintPlate(canvas, work, s, 41, 82, 696, 737, 164)
	drawFaders(canvas, work, s, cabledLayout)
	return canvas
}

func renderSmallAppIcon(work int) *image.NRGBA {
	s := float64(work) / 1024
	canvas := image.NewNRGBA(image.Rect(0, 0, work, work))
	paintPlate(canvas, work, s, 41, 41, 942, 901, 200)
	drawFaders(canvas, work, s, smallLayout)
	return canvas
}

func paintPlate(canvas *image.NRGBA, work int, s, x, y, w, h, r float64) {
	p := func(v float64) float32 { return float32(v * s) }
	lip := mask(work, func(z *vector.Rasterizer) {
		addRoundedRect(z, p(x), p(y+plateLipDepth), p(w), p(h), p(r))
	})
	fillMask(canvas, lip, plateLip)
	face := mask(work, func(z *vector.Rasterizer) {
		addRoundedRect(z, p(x), p(y), p(w), p(h), p(r))
	})
	fillMask(canvas, face, plateFace)
	fillMask(canvas, ringRoundedRect(work, p(x+6), p(y+6), p(w-12), p(h-12), p(r-6), p(6)), plateRim)
}

func drawFaders(canvas *image.NRGBA, work int, s float64, l faderLayout) {
	p := func(v float64) float32 { return float32(v * s) }
	caps := l.capCentres()

	tracks := mask(work, func(z *vector.Rasterizer) {
		for _, c := range caps {
			addRoundedRect(z, p(c.x-l.trackW/2), p(l.top), p(l.trackW), p(l.bottom-l.top), p(l.trackW/2))
		}
	})
	fillMask(canvas, tracks, trackInk)
	fillMask(canvas, strokeMask(work, wCurve(caps).scaled(s), l.strokeHW*s), ledOrange)

	body := mask(work, func(z *vector.Rasterizer) {
		for _, c := range caps {
			addRoundedRect(z, p(c.x-l.capW/2), p(c.y-l.capH/2), p(l.capW), p(l.capH), p(l.capR))
		}
	})
	dropShadow(canvas, body, 14*s, 16*s, grayColor(0, 0.3))
	fillMask(canvas, body, capInk)

	grips := mask(work, func(z *vector.Rasterizer) {
		for _, c := range caps {
			addRoundedRect(z, p(c.x-l.gripW/2), p(c.y-l.gripH/2), p(l.gripW), p(l.gripH), p(l.gripH/2))
		}
	})
	fillMask(canvas, grips, gripInk)
}

func (l faderLayout) capCentres() [5]pt {
	ys := [5]float64{l.high, l.low, l.mid, l.low, l.high}
	var out [5]pt
	for i := range out {
		out[i] = pt{l.left + float64(i)*l.gap, ys[i]}
	}
	return out
}

// Handle ratios reproduce the approved mockup: horizontal tangents at every cap, steep at both ends.
func wCurve(caps [5]pt) polyline {
	h := 0.4 * (caps[1].x - caps[0].x)
	ex, ey := 0.13*(caps[1].x-caps[0].x), 0.48*(caps[1].y-caps[0].y)
	line := polyline{caps[0]}
	line.cubeTo(pt{caps[0].x + ex, caps[0].y + ey}, pt{caps[1].x - h, caps[1].y}, caps[1])
	for i := 1; i < 3; i++ {
		line.cubeTo(pt{caps[i].x + h, caps[i].y}, pt{caps[i+1].x - h, caps[i+1].y}, caps[i+1])
	}
	line.cubeTo(pt{caps[3].x + h, caps[3].y}, pt{caps[4].x - ex, caps[4].y + ey}, caps[4])
	return line
}
