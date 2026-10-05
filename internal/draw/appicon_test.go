package draw

import "testing"

func TestAppIconCorners(t *testing.T) {
	for _, px := range []int{16, 32, 256} {
		img := AppIcon(px)
		if a := img.NRGBAAt(0, 0).A; a != 0 {
			t.Errorf("px=%d: corner alpha = %d, want 0", px, a)
		}
		c := px / 2
		if a := img.NRGBAAt(c, c).A; a != 255 {
			t.Errorf("px=%d: centre alpha = %d, want 255", px, a)
		}
	}
}

func TestAppIconColors(t *testing.T) {
	img := AppIcon(1024)
	for _, tc := range []struct {
		name string
		x, y int
		want [3]int
	}{
		{"plate", 200, 130, [3]int{0xf1, 0xec, 0xe2}},
		{"W", 329, 532, [3]int{0xff, 0x5a, 0x1f}},
		{"J cable", 901, 700, [3]int{0xff, 0x5a, 0x1f}},
	} {
		c := img.NRGBAAt(tc.x, tc.y)
		got := [3]int{int(c.R), int(c.G), int(c.B)}
		for i, w := range tc.want {
			if d := got[i] - w; d < -6 || d > 6 {
				t.Errorf("%s colour channel %d = %d, want %d +/-6", tc.name, i, got[i], w)
			}
		}
	}
}
