//go:build windows

package app

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/draw"
	"github.com/zolferfigueiredo/weej/internal/platform/display"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
)

type parsedFont = opentype.Font

const hudGlyphSourcePx = 32

func (app *App) showHUD(job core.Job, s float64) {
	app.loop.Invoke(func() {
		mon := app.monitorFor(job)
		scale := float64(mon.DPI) / 96
		if scale <= 0 {
			scale = 1
		}
		bmp := draw.HUD(draw.HUDParams{
			Scale:   scale,
			Dark:    !sys.AppsLight(),
			Glyph:   app.glyphFor(job, s),
			Percent: core.Percent(s),
			Face:    app.faceAt(scale),
			Accent:  sys.Accent(),
		})
		app.hud.Show(monitorKey(mon), mon.Work, scale, bmp)
	})
}

// showProfileHUD names the profile s has just made active, on the same screen and spot as the
// volume HUD, so it replaces one that is still showing instead of stacking.
func (app *App) showProfileHUD(s core.Settings) {
	if s.Active < 0 || s.Active >= len(s.Profiles) {
		return
	}
	app.loop.Invoke(func() {
		mon := app.monitorFor(core.Job{})
		scale := float64(mon.DPI) / 96
		if scale <= 0 {
			scale = 1
		}
		bmp := draw.ProfileHUD(draw.ProfileHUDParams{
			Scale:  scale,
			Dark:   !sys.AppsLight(),
			Icon:   draw.AppIcon(int(math.Round(20 * scale))),
			Name:   app.displayProfileName(s.Profiles[s.Active], s.Active),
			Index:  s.Active,
			Count:  len(s.Profiles),
			Face:   app.faceAt(scale),
			Accent: sys.Accent(),
		})
		app.hud.Show(monitorKey(mon), mon.Work, scale, bmp)
	})
}

func monitorKey(m display.Monitor) string {
	if m.Handle != 0 {
		return fmt.Sprintf("h%d", m.Handle)
	}
	return m.Name
}

func (app *App) monitorFor(job core.Job) display.Monitor {
	mons := display.Monitors()
	switch job.Kind {
	case core.JobBuiltinBrightness:
		for _, m := range mons {
			if m.Internal {
				return m
			}
		}
	case core.JobBrightness, core.JobContrast:
		for _, m := range mons {
			if m.Screen == job.Screen {
				return m
			}
		}
	case core.JobZoom:
		return display.AtPointer()
	}
	for _, m := range mons {
		if m.Primary {
			return m
		}
	}
	if len(mons) > 0 {
		return mons[0]
	}
	return display.Monitor{}
}

func (app *App) glyphInk() color.Color {
	if sys.AppsLight() {
		return color.Black
	}
	return color.White
}

func (app *App) glyphFor(job core.Job, s float64) *image.NRGBA {
	ink := app.glyphInk()
	switch job.Kind {
	case core.JobMaster, core.JobSystemSounds:
		if s <= 0 {
			return draw.Glyph(draw.GlyphSpeakerMuted, hudGlyphSourcePx, ink)
		}
		return draw.Glyph(draw.GlyphSpeaker, hudGlyphSourcePx, ink)
	case core.JobMicrophone:
		return draw.Glyph(draw.GlyphMic, hudGlyphSourcePx, ink)
	case core.JobBuiltinBrightness, core.JobBrightness:
		return draw.Glyph(draw.GlyphSun, hudGlyphSourcePx, ink)
	case core.JobContrast:
		return draw.Glyph(draw.GlyphContrast, hudGlyphSourcePx, ink)
	case core.JobNightLight:
		return draw.Glyph(draw.GlyphMoon, hudGlyphSourcePx, ink)
	case core.JobExternalKeyboard:
		return draw.Glyph(draw.GlyphKeyboard, hudGlyphSourcePx, ink)
	case core.JobZoom:
		return draw.Glyph(draw.GlyphZoom, hudGlyphSourcePx, ink)
	case core.JobApp:
		if icon := app.cachedExeIcon(job.Exe, hudGlyphSourcePx); icon != nil {
			return icon
		}
		return draw.Glyph(draw.GlyphApp, hudGlyphSourcePx, ink)
	case core.JobFocusedApp:
		if app.audio != nil {
			if exe := app.audio.FocusedExe(); exe != "" {
				if icon := app.cachedExeIcon(exe, hudGlyphSourcePx); icon != nil {
					return icon
				}
			}
		}
		return draw.Glyph(draw.GlyphApp, hudGlyphSourcePx, ink)
	default: // otherApps and anything else uses the generic app glyph
		return draw.Glyph(draw.GlyphApp, hudGlyphSourcePx, ink)
	}
}

func (app *App) cachedExeIcon(exe string, px int) *image.NRGBA {
	path := app.lookupAppPath(exe)
	if path == "" {
		return nil
	}
	return winui.ExeIcon(path, px)
}

func (app *App) faceAt(scale float64) font.Face {
	app.faceMu.Lock()
	defer app.faceMu.Unlock()

	if app.faceFont == nil {
		app.faceFont = loadHUDFont()
		app.faceCache = map[int]font.Face{}
	}
	if app.faceFont == nil {
		return nil
	}

	key := int(math.Round(scale * 1000))
	if f, ok := app.faceCache[key]; ok {
		return f
	}
	f, err := opentype.NewFace(app.faceFont, &opentype.FaceOptions{
		Size:    14 * scale,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	app.faceCache[key] = f
	return f
}

func loadHUDFont() *parsedFont {
	for _, path := range []string{`C:\Windows\Fonts\SegUIVar.ttf`, `C:\Windows\Fonts\segoeui.ttf`} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		return f
	}
	return nil
}
