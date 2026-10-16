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
	"github.com/zolferfigueiredo/weej/internal/platform/audio"
	"github.com/zolferfigueiredo/weej/internal/platform/display"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
)

type parsedFont = opentype.Font

const hudGlyphSourcePx = 32

func (app *App) showHUD(job core.Job, s float64) {
	app.loop.Invoke(func() { app.drawLevelHUD(app.monitorFor(job), app.glyphFor(job, s), s) })
}

// showGlyphHUD shows a level by one of the glyphs, as a button's action does, on the screen a
// profile's name shows on.
func (app *App) showGlyphHUD(glyph draw.GlyphKind, s float64) {
	app.loop.Invoke(func() {
		app.drawLevelHUD(app.monitorFor(core.Job{}), draw.Glyph(glyph, hudGlyphSourcePx, app.glyphInk()), s)
	})
}

func hudScale(mon display.Monitor) float64 {
	if scale := float64(mon.DPI) / 96; scale > 0 {
		return scale
	}
	return 1
}

func (app *App) drawLevelHUD(mon display.Monitor, glyph *image.NRGBA, s float64) {
	scale := hudScale(mon)
	bmp := draw.HUD(draw.HUDParams{
		Scale:   scale,
		Dark:    !sys.AppsLight(),
		Glyph:   glyph,
		Percent: core.Percent(s),
		Face:    app.faceAt(scale),
		Accent:  sys.AccentFill(!sys.AppsLight()),
	})
	app.hud.Show(monitorKey(mon), mon.Work, scale, bmp)
}

// showProfileHUD names the profile a board has just made active, and the board too when there
// are several, on the same screen and spot as the volume HUD, so it replaces one that is still
// showing instead of stacking.
func (app *App) showProfileHUD(d core.Device, named bool) {
	if d.Active < 0 || d.Active >= len(d.Profiles) {
		return
	}
	name := app.displayProfileName(d.Profiles[d.Active], d.Active)
	if named {
		name = d.Name + " · " + name
	}
	app.showNameHUD(name, d.Active, len(d.Profiles), func(scale float64) *image.NRGBA {
		return draw.AppIcon(int(math.Round(20 * scale)))
	})
}

// showLightsHUD names the light pattern an SMC-Mixer has just taken, as a profile is named.
func (app *App) showLightsHUD(name string, on bool) {
	glyph := draw.GlyphBulbOff
	if on {
		glyph = draw.GlyphBulbOn
	}
	app.showNameHUD(name, 0, 0, func(float64) *image.NRGBA { return draw.Glyph(glyph, hudGlyphSourcePx, app.glyphInk()) })
}

// showNameHUD shows a name by an icon, with dots for where index sits among count when there are
// a few.
func (app *App) showNameHUD(name string, index, count int, icon func(scale float64) *image.NRGBA) {
	app.loop.Invoke(func() {
		mon := app.monitorFor(core.Job{})
		scale := hudScale(mon)
		bmp := draw.ProfileHUD(draw.ProfileHUDParams{
			Scale:  scale,
			Dark:   !sys.AppsLight(),
			Icon:   icon(scale),
			Name:   name,
			Index:  index,
			Count:  count,
			Face:   app.faceAt(scale),
			Accent: sys.AccentFill(!sys.AppsLight()),
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
		if icon := app.exeIcon(app.resolveAppPath(job.Exe), hudGlyphSourcePx); icon != nil {
			return icon
		}
		return draw.Glyph(draw.GlyphApp, hudGlyphSourcePx, ink)
	case core.JobFocusedApp:
		// The app in front now, by its own process: the audio thread may not have got to this turn yet.
		if _, path := audio.ForegroundApp(); path != "" {
			if icon := app.exeIcon(path, hudGlyphSourcePx); icon != nil {
				return icon
			}
		}
		return draw.Glyph(draw.GlyphApp, hudGlyphSourcePx, ink)
	default: // otherApps and anything else uses the generic app glyph
		return draw.Glyph(draw.GlyphApp, hudGlyphSourcePx, ink)
	}
}

type exeIconKey struct {
	path string
	px   int
}

// exeIcon is the icon in the exe at path, read from the file the first time and then kept, since the
// HUD redraws it at every step of a turn.
func (app *App) exeIcon(path string, px int) *image.NRGBA {
	if path == "" {
		return nil
	}
	key := exeIconKey{path, px}
	app.appCacheMu.Lock()
	icon, ok := app.exeIcons[key]
	app.appCacheMu.Unlock()
	if ok {
		return icon
	}
	icon = winui.ExeIcon(path, px)
	app.appCacheMu.Lock()
	app.exeIcons[key] = icon
	app.appCacheMu.Unlock()
	return icon
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
