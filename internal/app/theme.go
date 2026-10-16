//go:build windows

package app

import (
	"fmt"
	"image/color"

	"github.com/zolferfigueiredo/weej/internal/lang"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

func hexColor(c color.RGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

func themePayload() map[string]any {
	dark := !sys.AppsLight()
	return map[string]any{"dark": dark, "accent": hexColor(sys.AccentFill(dark))}
}

func (app *App) baseInitFields() map[string]any {
	code := app.currentLanguage()
	return map[string]any{
		"type":    "init",
		"lang":    code,
		"strings": lang.Catalog(code),
		"theme":   themePayload(),
	}
}

func (app *App) applyThemeToWebWindows() {
	dark := !sys.AppsLight()
	msg := themePayload()
	msg["type"] = "theme"
	for _, w := range app.openWebWindows() {
		w.SetTheme(dark)
		w.Send(msg)
	}
}
