package draw

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

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
	drawPanel(canvas, scale, p.Dark)
	fg := hudInk(p.Dark)
	accent := hudAccent(p.Accent, p.Dark)

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

	drawHUDGlyph(canvas, p.Glyph, scale)

	if p.Face != nil {
		drawPercentText(canvas, p.Face, p.Percent, h, scale, fg)
	}

	return canvas
}

// drawPanel paints the rounded flyout background and its hairline border over the whole canvas.
func drawPanel(canvas *image.NRGBA, scale float64, dark bool) {
	w, h := canvas.Rect.Dx(), canvas.Rect.Dy()
	px := func(dip float64) float32 { return float32(dip * scale) }

	bg := hexColor(0x2b2b2b, 0.96)
	borderColor := grayColor(1, 0.08)
	if !dark {
		bg = hexColor(0xf9f9f9, 0.96)
		borderColor = grayColor(0, 0.06)
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
}

func hudInk(dark bool) rgba {
	if dark {
		return grayColor(1, 1)
	}
	return grayColor(0, 1)
}

func hudAccent(accent color.Color, dark bool) color.Color {
	if accent != nil {
		return accent
	}
	if dark {
		return color.NRGBA{R: 0x4c, G: 0xc2, B: 0xff, A: 0xff}
	}
	return color.NRGBA{R: 0x00, G: 0x78, B: 0xd4, A: 0xff}
}

// drawHUDGlyph puts glyph in the HUD's left slot, vertically centred.
func drawHUDGlyph(canvas, glyph *image.NRGBA, scale float64) {
	if glyph == nil {
		return
	}
	size := int(math.Round(hudGlyphSizeDIP * scale))
	if glyph.Rect.Dx() != size || glyph.Rect.Dy() != size {
		glyph = resizeBilinear(glyph, size, size)
	}
	originX := int(math.Round(hudLeftMarginDIP * scale))
	originY := (canvas.Rect.Dy() - size) / 2
	blitOver(canvas, glyph, originX, originY)
}

type ProfileHUDParams struct {
	Scale  float64
	Dark   bool
	Icon   *image.NRGBA
	Name   string
	Index  int // the active profile, from 0
	Count  int
	Face   font.Face
	Accent color.Color
}

const (
	profileHUDMaxWidthDIP = 360
	profileDotDIP         = 6
	profileDotGapDIP      = 8
	profileNameGapDIP     = 16
	// Past this many profiles the dots stop being countable at a glance, so "2/12" replaces them.
	profileMaxDots = 9
)

// ProfileHUD is the flyout shown when the active profile changes: the app icon, the profile's
// name, and where it sits among the profiles. It is as wide as the name needs, from the volume
// HUD's width up to profileHUDMaxWidthDIP, past which the name is cut short with "…".
func ProfileHUD(p ProfileHUDParams) *image.NRGBA {
	scale := p.Scale
	if scale <= 0 {
		scale = 1
	}
	dip := func(v float64) int { return int(math.Round(v * scale)) }
	ink := hudInk(p.Dark)

	dots := p.Count >= 2 && p.Count <= profileMaxDots
	countText := ""
	if p.Count > profileMaxDots {
		countText = strconv.Itoa(p.Index+1) + "/" + strconv.Itoa(p.Count)
	}
	indicatorW := 0
	switch {
	case dots:
		indicatorW = dip(float64(p.Count)*profileDotDIP + float64(p.Count-1)*profileDotGapDIP)
	case countText != "" && p.Face != nil:
		indicatorW = font.MeasureString(p.Face, countText).Ceil()
	}
	gap := 0
	if indicatorW > 0 {
		gap = dip(profileNameGapDIP)
	}

	nameX := dip(hudLeftMarginDIP + hudGlyphSizeDIP + hudGlyphTrackGapDIP)
	rightMargin := dip(hudRightMarginDIP)
	minW, maxW := dip(hudWidthDIP), dip(profileHUDMaxWidthDIP)
	name := fitText(p.Face, p.Name, maxW-nameX-gap-indicatorW-rightMargin)
	nameW := 0
	if p.Face != nil {
		nameW = font.MeasureString(p.Face, name).Ceil()
	}
	w := clampInt(nameX+nameW+gap+indicatorW+rightMargin, minW, maxW)
	if name != p.Name {
		w = maxW
	}
	h := dip(hudHeightDIP)

	canvas := image.NewNRGBA(image.Rect(0, 0, w, h))
	drawPanel(canvas, scale, p.Dark)
	drawHUDGlyph(canvas, p.Icon, scale)
	if p.Face != nil {
		drawTextAt(canvas, p.Face, name, nameX, h, ink)
	}

	x0 := w - rightMargin - indicatorW
	switch {
	case dots:
		accent := colorToRGBA(hudAccent(p.Accent, p.Dark))
		r := float32(profileDotDIP * scale / 2)
		for i := 0; i < p.Count; i++ {
			cx := float32(x0) + float32(float64(i)*(profileDotDIP+profileDotGapDIP)*scale) + r
			dot := maskRect(w, h, func(z *vector.Rasterizer) { addEllipse(z, cx, float32(h)/2, r, r) })
			c := rgba{ink.r, ink.g, ink.b, 0.2}
			if i == p.Index {
				c = accent
			}
			fillMask(canvas, dot, c)
		}
	case countText != "" && p.Face != nil:
		drawTextAt(canvas, p.Face, countText, x0, h, rgba{ink.r, ink.g, ink.b, 0.6})
	}

	return canvas
}

// fitText shortens s with "…" until it is at most maxPx wide in face.
func fitText(face font.Face, s string, maxPx int) string {
	if face == nil || font.MeasureString(face, s).Ceil() <= maxPx {
		return s
	}
	r := []rune(s)
	for n := len(r) - 1; n > 0; n-- {
		cut := strings.TrimRight(string(r[:n]), " ") + "…"
		if font.MeasureString(face, cut).Ceil() <= maxPx {
			return cut
		}
	}
	return "…"
}

// drawTextAt draws text from x, vertically centred in a canvas h pixels tall.
func drawTextAt(canvas *image.NRGBA, face font.Face, text string, x, h int, c rgba) {
	m := face.Metrics()
	drawer := font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(color.NRGBA{clamp255(c.r), clamp255(c.g), clamp255(c.b), clamp255(c.a * 255)}),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(h/2) + (m.Ascent-m.Descent)/2},
	}
	drawer.DrawString(text)
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
