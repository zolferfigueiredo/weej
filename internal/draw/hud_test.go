package draw

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
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

func TestProfileHUDWidthFollowsTheName(t *testing.T) {
	face := basicfont.Face7x13
	minW := int(math.Round(hudWidthDIP))
	maxW := int(math.Round(profileHUDMaxWidthDIP))
	h := int(math.Round(hudHeightDIP))

	short := ProfileHUD(ProfileHUDParams{Scale: 1, Name: "Games", Index: 0, Count: 3, Face: face})
	if short.Rect.Dx() != minW || short.Rect.Dy() != h {
		t.Errorf("short name: %dx%d, want %dx%d", short.Rect.Dx(), short.Rect.Dy(), minW, h)
	}
	medium := ProfileHUD(ProfileHUDParams{Scale: 1, Name: "Night gaming setup", Index: 0, Count: 3, Face: face})
	if w := medium.Rect.Dx(); w <= minW || w >= maxW {
		t.Errorf("medium name: width %d, want between %d and %d", w, minW, maxW)
	}
	long := ProfileHUD(ProfileHUDParams{Scale: 1, Name: strings.Repeat("Very long profile name ", 5), Index: 0, Count: 3, Face: face})
	if w := long.Rect.Dx(); w != maxW {
		t.Errorf("long name: width %d, want the %d maximum", w, maxW)
	}
}

func TestFitTextCutsWithAnEllipsis(t *testing.T) {
	face := basicfont.Face7x13
	got := fitText(face, "Very long profile name", 70)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("fitText = %q, want it cut with …", got)
	}
	if w := font.MeasureString(face, got).Ceil(); w > 70 {
		t.Errorf("fitText = %q is %d px, want at most 70", got, w)
	}
	if got := fitText(face, "Games", 70); got != "Games" {
		t.Errorf("fitText = %q, want a short name untouched", got)
	}
}

func TestProfileHUDMarksTheActiveProfile(t *testing.T) {
	const scale = 2.0
	accent := color.NRGBA{R: 0x00, G: 0x78, B: 0xd4, A: 0xff}
	img := ProfileHUD(ProfileHUDParams{Scale: scale, Dark: true, Name: "Games", Index: 1, Count: 3, Face: basicfont.Face7x13, Accent: accent})

	dotsW := int(math.Round((3*profileDotDIP + 2*profileDotGapDIP) * scale))
	x0 := img.Rect.Dx() - int(math.Round(hudRightMarginDIP*scale)) - dotsW
	y := img.Rect.Dy() / 2
	for i := 0; i < 3; i++ {
		cx := x0 + int(math.Round((float64(i)*(profileDotDIP+profileDotGapDIP)+profileDotDIP/2)*scale))
		isAccent := closeColor(img.NRGBAAt(cx, y), accent, 10)
		if i == 1 && !isAccent {
			t.Errorf("dot %d (active) = %+v, want near %+v", i, img.NRGBAAt(cx, y), accent)
		}
		if i != 1 && isAccent {
			t.Errorf("dot %d (inactive) is accent-coloured", i)
		}
	}
}

func TestProfileHUDWithoutAFontStillDraws(t *testing.T) {
	img := ProfileHUD(ProfileHUDParams{Scale: 1.5, Name: "Games", Index: 0, Count: 12})
	if img.Rect.Dx() != int(math.Round(hudWidthDIP*1.5)) {
		t.Errorf("width %d, want the minimum", img.Rect.Dx())
	}
}
