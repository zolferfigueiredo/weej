//go:build windows

package app

import (
	"encoding/json"
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
	app.tray = l.NewTray(func() { app.openSettings("") }, app.showTrayMenu, func() { app.manualCheckUpdate() })
	app.hud = l.NewHUD()
	l.OnThemeChange(func() {
		app.refreshTray()
		app.applyThemeToWebWindows()
	})
	app.refreshTray()
}

func (app *App) showTrayMenu() {
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
	var names []string
	for _, d := range s.Devices {
		if r := app.runnerFor(d.ID); r != nil {
			if connected, _, _ := r.status(); connected {
				names = append(names, d.Name+" · "+app.displayProfileName(d.ActiveProfile(), d.Active))
			}
		}
	}

	icon := draw.TrayIcon(draw.IconStyle(s.Icon), len(names) > 0, app.tray.IconSize(), sys.TaskbarLight())
	app.tray.SetIcon(icon)

	tip := "WeeJ: " + app.tr("not_connected")
	if len(names) > 0 {
		tip = "WeeJ: " + strings.Join(names, ", ")
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

// buildMenu has a section per board that is on: its name and status, its profiles, and Calibrate
// for one that can be calibrated.
func (app *App) buildMenu() []winui.MenuItem {
	s := app.snapshotSettings()
	labels := app.shortcutLabels(s)
	label := func(sc *core.Shortcut) string {
		if sc == nil {
			return ""
		}
		data, err := json.Marshal(sc)
		if err != nil {
			return ""
		}
		return labels[string(data)]
	}

	var items []winui.MenuItem
	for _, d := range s.Devices {
		if !d.Enabled {
			continue
		}
		id := d.ID
		connected, busy, port := false, false, ""
		if r := app.runnerFor(id); r != nil {
			connected, busy, port = r.status()
		}
		items = append(items, winui.MenuItem{
			Text:     app.trVars("tray.board", map[string]string{"name": core.Clipped(d.Name, 30), "status": app.statusLine(connected, busy, port)}),
			Disabled: true,
		})
		if s.ShowProfiles {
			for i, p := range d.Profiles {
				i := i
				items = append(items, winui.MenuItem{
					Text:     core.Clipped(app.displayProfileName(p, i), 30),
					Checked:  i == d.Active,
					Shortcut: label(p.Shortcut),
					OnClick:  func() { app.setProfiles([]core.HotkeyTarget{{Device: id, Profile: i}}) },
				})
			}
		}
		if d.Type != core.DeviceSMC {
			items = append(items, winui.MenuItem{Text: app.tr("calibrate"), OnClick: func() {
				app.openSettings("boards")
				app.startWizard(id, nil)
			}})
		}
		items = append(items, winui.MenuItem{Separator: true})
	}

	items = append(items, winui.MenuItem{Text: app.tr("settings"), OnClick: func() { app.openSettings("") }})
	items = append(items, winui.MenuItem{Text: app.tr("language"), Children: app.languageMenuItems(s.Language)})
	items = append(items, winui.MenuItem{Text: app.tr("reconnect"), OnClick: func() { app.requestReconnect("") }})

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

func (app *App) displayProfileName(p core.DeviceProfile, i int) string {
	if strings.TrimSpace(p.Name) != "" {
		return p.Name
	}
	return app.trVars("profile_n", v1("n", strconv.Itoa(i+1)))
}

// statusLine is a board's status in the tray.
func (app *App) statusLine(connected, busy bool, port string) string {
	switch {
	case connected:
		return app.trVars("connected", v1("port", port))
	case busy:
		return app.trVars("port_busy", v1("port", port))
	}
	return app.tr("not_connected")
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
