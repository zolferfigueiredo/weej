package draw

import (
	"image/color"
	"math"
	"testing"
)

func TestHUDSize(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		img := HUD(HUDParams{Scale: scale, Dark: true})
		wantW := int(math.Round(hudWidthDIP * scale))
		wantH := int(math.Round(hudHeightDIP * scale))
		if img.Rect.Dx() != wantW || img.Rect.Dy() != wantH {
			t.Errorf("scale %v: got %dx%d, want %dx%d", scale, img.Rect.Dx(), img.Rect.Dy(), wantW, wantH)
		}
	}
}

func TestHUDTrackPercent(t *testing.T) {
	const scale = 2.0
	accent := color.NRGBA{R: 0x00, G: 0x78, B: 0xd4, A: 0xff}
	trackX := int(math.Round((hudLeftMarginDIP + hudGlyphSizeDIP + hudGlyphTrackGapDIP) * scale))
	trackY := int(math.Round((hudHeightDIP - hudTrackHeightDIP) / 2 * scale))
	trackW := int(math.Round(hudTrackWidthDIP * scale))
	trackH := int(math.Round(hudTrackHeightDIP * scale))
	midY := trackY + trackH/2

	empty := HUD(HUDParams{Scale: scale, Dark: false, Percent: 0, Accent: accent})
	for x := trackX; x < trackX+trackW; x++ {
		for y := trackY; y < trackY+trackH; y++ {
			if closeColor(empty.NRGBAAt(x, y), accent, 10) {
				t.Fatalf("0%%: found accent-coloured pixel in track at (%d,%d)", x, y)
			}
		}
	}

	full := HUD(HUDParams{Scale: scale, Dark: false, Percent: 100, Accent: accent})
	if got := full.NRGBAAt(trackX+trackW-3, midY); !closeColor(got, accent, 10) {
		t.Errorf("100%%: fill does not reach the track's end, pixel = %+v, want near %+v", got, accent)
	}
}

func closeColor(a, b color.NRGBA, tol int) bool {
	d := func(x, y uint8) int {
		v := int(x) - int(y)
		if v < 0 {
			return -v
		}
		return v
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}
