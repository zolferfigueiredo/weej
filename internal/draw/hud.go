package draw

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

type HUDParams struct {
	Scale   float64
	Dark    bool
	Glyph   *image.NRGBA
	Percent int
	Face    font.Face
	Accent  color.Color
}

const (
	hudWidthDIP         = 192
	hudHeightDIP        = 48
	hudCornerRadiusDIP  = 8
	hudBorderDIP        = 1
	hudGlyphSizeDIP     = 20
	hudLeftMarginDIP    = 16
	hudGlyphTrackGapDIP = 12
	hudTrackHeightDIP   = 4
	hudTrackWidthDIP    = 92
	hudRightMarginDIP   = 16
)

func HUD(p HUDParams) *image.NRGBA {
	scale := p.Scale
	if scale <= 0 {
		scale = 1
	}
	w := int(math.Round(hudWidthDIP * scale))
	h := int(math.Round(hudHeightDIP * scale))
	px := func(dip float64) float32 { return float32(dip * scale) }

	canvas := image.NewNRGBA(image.Rect(0, 0, w, h))

	bg := hexColor(0x2b2b2b, 0.96)
	borderColor := grayColor(1, 0.08)
	fg := grayColor(1, 1)
	accent := p.Accent
	if !p.Dark {
		bg = hexColor(0xf9f9f9, 0.96)
		borderColor = grayColor(0, 0.06)
		fg = grayColor(0, 1)
	}
	if accent == nil {
		if p.Dark {
			accent = color.NRGBA{R: 0x4c, G: 0xc2, B: 0xff, A: 0xff}
		} else {
			accent = color.NRGBA{R: 0x00, G: 0x78, B: 0xd4, A: 0xff}
		}
	}

	panel := maskRect(w, h, func(z *vector.Rasterizer) {
		addRoundedRect(z, 0, 0, float32(w), float32(h), px(hudCornerRadiusDIP))
	})
	fillMask(canvas, panel, bg)

	bw := px(hudBorderDIP)
	inside := maskRect(w, h, func(z *vector.Rasterizer) {
		addRoundedRect(z, bw, bw, float32(w)-2*bw, float32(h)-2*bw, px(hudCornerRadiusDIP)-bw)
	})
	cutMask(panel, inside)
	fillMask(canvas, panel, borderColor)

	trackX := px(hudLeftMarginDIP + hudGlyphSizeDIP + hudGlyphTrackGapDIP)
	trackY := px((hudHeightDIP - hudTrackHeightDIP) / 2)
	trackH := px(hudTrackHeightDIP)
	trackR := px(hudTrackHeightDIP / 2)
	trackBG := maskRect(w, h, func(z *vector.Rasterizer) {
		addRoundedRect(z, trackX, trackY, px(hudTrackWidthDIP), trackH, trackR)
	})
	fillMask(canvas, trackBG, rgba{fg.r, fg.g, fg.b, 0.2})

	pct := clampF(float64(p.Percent)/100, 0, 1)
	if pct > 0 {
		fillW := px(hudTrackWidthDIP * pct)
		trackFill := maskRect(w, h, func(z *vector.Rasterizer) {
			addRoundedRect(z, trackX, trackY, fillW, trackH, trackR)
		})
		fillMask(canvas, trackFill, colorToRGBA(accent))
	}

	if p.Glyph != nil {
		size := int(math.Round(hudGlyphSizeDIP * scale))
		glyph := p.Glyph
		if glyph.Rect.Dx() != size || glyph.Rect.Dy() != size {
			glyph = resizeBilinear(glyph, size, size)
		}
		originX := int(math.Round(hudLeftMarginDIP * scale))
		originY := (h - size) / 2
		blitOver(canvas, glyph, originX, originY)
	}

	if p.Face != nil {
		drawPercentText(canvas, p.Face, p.Percent, h, scale, fg)
	}

	return canvas
}

func drawPercentText(canvas *image.NRGBA, face font.Face, percent, h int, scale float64, fg rgba) {
	text := strconv.Itoa(clampInt(percent, 0, 999))
	advance := font.MeasureString(face, text)
	m := face.Metrics()
	rightEdge := fixed.I(int(math.Round((hudWidthDIP - hudRightMarginDIP) * scale)))
	dot := fixed.Point26_6{
		X: rightEdge - advance,
		Y: fixed.I(h/2) + (m.Ascent-m.Descent)/2,
	}
	drawer := font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(color.NRGBA{clamp255(fg.r), clamp255(fg.g), clamp255(fg.b), clamp255(fg.a * 255)}),
		Face: face,
		Dot:  dot,
	}
	drawer.DrawString(text)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
