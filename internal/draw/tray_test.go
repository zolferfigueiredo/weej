package draw

import (
	"image"
	"testing"
)

func TestTrayIconFollowsTaskbar(t *testing.T) {
	for _, style := range []IconStyle{StyleMixer, StyleDial} {
		for _, parked := range []bool{false, true} {
			connected := !parked
			light := TrayIcon(style, connected, 32, true)
			dark := TrayIcon(style, connected, 32, false)

			lr, la := mostOpaquePixel(light)
			if la == 0 {
				t.Fatalf("%s parked=%v: light-taskbar icon is fully transparent", style, parked)
			}
			if lr >= 128 {
				t.Errorf("%s parked=%v light taskbar: most opaque pixel red=%d, want <128", style, parked, lr)
			}

			dr, da := mostOpaquePixel(dark)
			if da == 0 {
				t.Fatalf("%s parked=%v: dark-taskbar icon is fully transparent", style, parked)
			}
			if dr < 128 {
				t.Errorf("%s parked=%v dark taskbar: most opaque pixel red=%d, want >=128", style, parked, dr)
			}
		}
	}
}

func mostOpaquePixel(img *image.NRGBA) (red, alpha uint8) {
	bestA := -1
	for i := 0; i+3 < len(img.Pix); i += 4 {
		a := int(img.Pix[i+3])
		if a > bestA {
			bestA = a
			red = img.Pix[i]
		}
	}
	return red, uint8(bestA)
}
