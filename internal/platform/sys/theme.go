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
