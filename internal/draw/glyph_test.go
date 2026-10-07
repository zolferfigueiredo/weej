package draw

import (
	"image/color"
	"testing"
)

func TestEveryGlyphDraws(t *testing.T) {
	for kind := GlyphSpeaker; kind <= GlyphBulbOff; kind++ {
		img := Glyph(kind, 32, color.White)
		inked := 0
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] > 128 {
				inked++
			}
		}
		// A glyph covers some of its square and leaves most of it clear.
		if inked < 20 || inked > 32*32*3/4 {
			t.Errorf("glyph %d inks %d of %d pixels", kind, inked, 32*32)
		}
	}
}
