//go:build windows

package app

import (
	"fmt"
	"image/color"

	"github.com/zolferfigueiredo/weej/internal/lang"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

func hexColor(c color.RGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

func (app *App) baseInitFields() map[string]any {
	code := app.currentLanguage()
	return map[string]any{
		"lang":    code,
		"strings": lang.Catalog(code),
		"theme":   map[string]any{"dark": !sys.AppsLight(), "accent": hexColor(sys.Accent())},
	}
}
