//go:build windows

package app

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/draw"
	"github.com/zolferfigueiredo/weej/internal/lang"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
	"github.com/zolferfigueiredo/weej/internal/updater"
)

func (app *App) setupTray(l *winui.Loop) {
	app.tray = l.NewTray(app.onTrayClick, func() { app.manualCheckUpdate() })
	app.hud = l.NewHUD()
	l.OnThemeChange(func() {
		app.refreshTray()
		app.applyThemeToWebWindows()
	})
	app.refreshTray()
}

func (app *App) onTrayClick() {
	items := app.buildMenu()
	app.loop.PopupMenu(items, !sys.TaskbarLight())
}

func (app *App) refreshTray() {
	app.loop.Invoke(app.refreshTrayNow)
}

func (app *App) refreshTrayNow() {
	if app.tray == nil {
		return
	}
	s := app.snapshotSettings()
	connected, _, port := app.connectionStatus()

	icon := draw.TrayIcon(draw.IconStyle(s.Icon), connected, app.tray.IconSize(), sys.TaskbarLight())
	app.tray.SetIcon(icon)

	var tip string
	if connected {
		name := ""
		if s.Active >= 0 && s.Active < len(s.Profiles) {
			name = s.Profiles[s.Active].Name
		}
		tip = "WeeJ: " + port + " · " + name
	} else {
		tip = "WeeJ: " + app.tr("not_connected")
	}
	app.tray.SetTooltip(tip)

	if s.HideIcon {
		app.tray.Hide()
		return
	}
	app.tray.Show()
	if !s.TrayTipShown {
		app.tray.Balloon(app.tr("tray_tip_title"), app.tr("tray_tip"))
		next := app.snapshotSettings()
		next.TrayTipShown = true
		if err := app.persistSettings(next); err != nil {
			app.log("Could not save settings: " + err.Error())
		}
	}
}

func (app *App) buildMenu() []winui.MenuItem {
	s := app.snapshotSettings()
	connected, busy, port := app.connectionStatus()
	ink := color.Color(color.Black)
	if !sys.TaskbarLight() {
		ink = color.White
	}

	var items []winui.MenuItem

	items = append(items, winui.MenuItem{
		Text: app.tr("show_data"), Checked: s.ShowDataInMenu,
		OnClick: func() {
			cur := app.snapshotSettings()
			cur.ShowDataInMenu = !cur.ShowDataInMenu
			if err := app.persistSettings(cur); err != nil {
				app.log("Could not save settings: " + err.Error())
			}
		},
	})
	if connected && s.ShowDataInMenu {
		for _, line := range app.engine.Lines() {
			items = append(items, winui.MenuItem{
				Text:     fmt.Sprintf("%s %d%%", line.Job.Title(app.trFunc(), app.appDisplayName), line.Percent),
				Disabled: true,
				Icon:     app.jobMenuIcon(line.Job, ink),
			})
		}
	}
	items = append(items, winui.MenuItem{Separator: true})

	if s.ShowProfiles {
		items = append(items, winui.MenuItem{Text: app.tr("profiles"), Disabled: true})
		labels := app.shortcutLabels(s.Setup)
		for i := range s.Profiles {
			i := i
			p := s.Profiles[i]
			label := ""
			if p.Shortcut != nil {
				if data, err := json.Marshal(p.Shortcut); err == nil {
					label = labels[string(data)]
				}
			}
			items = append(items, winui.MenuItem{
				Text:     core.Clipped(app.displayProfileName(p, i), 30),
				Checked:  i == s.Active,
				Shortcut: label,
				OnClick:  func() { app.switchProfile(i) },
			})
		}
		items = append(items, winui.MenuItem{Separator: true})
	}

	items = append(items,
		winui.MenuItem{Text: app.tr("settings"), OnClick: func() { app.openSettings("") }},
		winui.MenuItem{Text: app.tr("calibrate"), Disabled: !connected, OnClick: func() { app.startCalibration(false) }},
		winui.MenuItem{Text: app.tr("language"), Children: app.languageMenuItems(s.Language)},
	)

	items = append(items, winui.MenuItem{Separator: true})
	var connLine string
	switch {
	case connected:
		connLine = app.trVars("connected", v1("port", port))
	case busy:
		connLine = app.trVars("port_busy", v1("port", port))
	default:
		connLine = app.tr("not_connected")
	}
	items = append(items,
		winui.MenuItem{Text: connLine, Disabled: true},
		winui.MenuItem{Text: app.tr("reconnect"), OnClick: app.requestReconnect},
	)

	items = append(items, winui.MenuItem{Separator: true})
	items = append(items, winui.MenuItem{
		Text: app.tr("login"), Checked: sys.LoginEnabled(app.self), Disabled: !sys.IsInstalledCopy(app.self),
		OnClick: app.toggleLogin,
	})

	items = append(items, winui.MenuItem{Separator: true})
	items = append(items, winui.MenuItem{Text: app.tr("about"), OnClick: func() { app.openSettings("about") }})

	items = append(items, winui.MenuItem{Separator: true})
	items = append(items, app.updateMenuItem(s))
	items = append(items, winui.MenuItem{Text: app.tr("auto"), Children: app.autoUpdateMenuItems(s.UpdateEvery)})

	items = append(items, winui.MenuItem{Separator: true})
	items = append(items, winui.MenuItem{Text: app.tr("quit"), OnClick: app.quit})

	return items
}

func (app *App) displayProfileName(p core.Profile, i int) string {
	if strings.TrimSpace(p.Name) != "" {
		return p.Name
	}
	return app.trVars("profile_n", v1("n", strconv.Itoa(i+1)))
}

func (app *App) switchProfile(i int) {
	s := app.snapshotSettings()
	if i < 0 || i >= len(s.Profiles) {
		return
	}
	s.Active = i
	if err := app.persistSettings(s); err != nil {
		app.log("Could not save settings: " + err.Error())
	}
	app.refreshTray()
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "profile", "profile": i})
	}
}

func (app *App) requestReconnect() {
	select {
	case app.reconnectCh <- struct{}{}:
	default:
	}
}

func (app *App) toggleLogin() {
	on := !sys.LoginEnabled(app.self)
	if err := sys.SetLogin(on, app.self, nil); err != nil {
		app.log("Could not change Launch at login: " + err.Error())
	}
}

func (app *App) languageMenuItems(current string) []winui.MenuItem {
	items := make([]winui.MenuItem, 0, len(lang.Languages))
	for _, l := range lang.Languages {
		code := l.Code
		items = append(items, winui.MenuItem{
			Text: l.Name, Checked: code == current,
			OnClick: func() { app.setLanguage(code) },
		})
	}
	return items
}

func (app *App) updateMenuItem(s core.Settings) winui.MenuItem {
	app.mu.Lock()
	checking, installing, step := app.checking, app.installing, app.installStep
	app.mu.Unlock()

	if checking || installing {
		return winui.MenuItem{Text: app.updateStepText(step, s.AvailableVersion), Disabled: true}
	}
	if s.AvailableVersion != "" && core.IsNewer(s.AvailableVersion, core.AppVersion) {
		text := app.tr("update_available") + " " + app.trVars("install_now", v1("version", s.AvailableVersion))
		return winui.MenuItem{Text: text, Bold: true, OnClick: app.startUpdateNow}
	}
	return winui.MenuItem{Text: app.tr("check"), OnClick: func() { app.manualCheckUpdate() }}
}

func (app *App) updateStepText(step updater.Step, availableVersion string) string {
	switch step {
	case updater.Checking:
		return app.tr("checking_download")
	case updater.Installing:
		return app.tr("installing")
	case updater.Done:
		return app.trVars("installed", v1("version", availableVersion))
	default:
		return app.trVars("downloading", v1("version", availableVersion))
	}
}

func (app *App) autoUpdateMenuItems(every int) []winui.MenuItem {
	opts := []struct {
		key     string
		seconds int
	}{{"daily", 86400}, {"weekly", 604800}, {"never", 0}}
	items := make([]winui.MenuItem, 0, len(opts))
	for _, o := range opts {
		seconds := o.seconds
		items = append(items, winui.MenuItem{
			Text: app.tr(o.key), Checked: every == seconds,
			OnClick: func() {
				cur := app.snapshotSettings()
				cur.UpdateEvery = seconds
				if err := app.persistSettings(cur); err != nil {
					app.log("Could not save settings: " + err.Error())
				}
			},
		})
	}
	return items
}

func (app *App) quit() {
	app.shutdown()
	app.loop.Quit(0)
}

func (app *App) jobMenuIcon(job core.Job, ink color.Color) *image.NRGBA {
	const px = 16
	switch job.Kind {
	case core.JobMaster, core.JobSystemSounds:
		return draw.Glyph(draw.GlyphSpeaker, px, ink)
	case core.JobMicrophone:
		return draw.Glyph(draw.GlyphMic, px, ink)
	case core.JobBuiltinBrightness, core.JobBrightness:
		return draw.Glyph(draw.GlyphSun, px, ink)
	case core.JobContrast:
		return draw.Glyph(draw.GlyphContrast, px, ink)
	case core.JobNightLight:
		return draw.Glyph(draw.GlyphMoon, px, ink)
	case core.JobExternalKeyboard:
		return draw.Glyph(draw.GlyphKeyboard, px, ink)
	case core.JobZoom:
		return draw.Glyph(draw.GlyphZoom, px, ink)
	case core.JobApp:
		if icon := app.cachedExeIcon(job.Exe, px); icon != nil {
			return icon
		}
		return draw.Glyph(draw.GlyphApp, px, ink)
	default:
		return draw.Glyph(draw.GlyphApp, px, ink)
	}
}
