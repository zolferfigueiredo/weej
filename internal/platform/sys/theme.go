//go:build windows

package sys

import (
	"image/color"

	"golang.org/x/sys/windows/registry"
)

const personalizePath = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`

func TaskbarLight() bool { return lightValue("SystemUsesLightTheme") }

func AppsLight() bool { return lightValue("AppsUseLightTheme") }

func lightValue(name string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, personalizePath, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return true
	}
	return v != 0
}

var defaultAccent = color.RGBA{R: 0x00, G: 0x78, B: 0xD4, A: 0xFF}

// AccentFill is the shade of the accent color Windows 11 fills its own controls with: a lighter one
// on the dark theme and a darker one on the light theme, rather than the accent itself. It falls back
// to Accent when Windows has no AccentPalette.
func AccentFill(dark bool) color.RGBA {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\Accent`, registry.QUERY_VALUE)
	if err != nil {
		return Accent()
	}
	defer k.Close()
	p, _, err := k.GetBinaryValue("AccentPalette")
	if err != nil {
		return Accent()
	}
	if c, ok := paletteShade(p, dark); ok {
		return c
	}
	return Accent()
}

// paletteShade picks from AccentPalette's eight RGBA colours, lightest to darkest with the accent
// fourth: Light2 (the second) on the dark theme, Dark1 (the fifth) on the light theme.
func paletteShade(p []byte, dark bool) (color.RGBA, bool) {
	if len(p) < 32 {
		return color.RGBA{}, false
	}
	i := 4 * 4
	if dark {
		i = 1 * 4
	}
	return color.RGBA{R: p[i], G: p[i+1], B: p[i+2], A: 0xFF}, true
}

// Accent reads DWM's AccentColor, stored as a packed ABGR DWORD (alpha in the high byte, red in
// the low byte), the reverse of the usual RGBA byte order.
func Accent() color.RGBA {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\DWM`, registry.QUERY_VALUE)
	if err != nil {
		return defaultAccent
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AccentColor")
	if err != nil {
		return defaultAccent
	}
	return color.RGBA{
		R: byte(v),
		G: byte(v >> 8),
		B: byte(v >> 16),
		A: byte(v >> 24),
	}
}
