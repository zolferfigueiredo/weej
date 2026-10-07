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
	connected, _, port := app.connectionStatus()
	mixerConnected, _, mixerPort := app.mixerStatus()
	var ports []string
	if connected {
		ports = append(ports, port)
	}
	if mixerConnected {
		ports = append(ports, mixerPort)
	}

	icon := draw.TrayIcon(draw.IconStyle(s.Icon), len(ports) > 0, app.tray.IconSize(), sys.TaskbarLight())
	app.tray.SetIcon(icon)

	var tip string
	if len(ports) > 0 {
		name := ""
		if s.Active >= 0 && s.Active < len(s.Profiles) {
			name = s.Profiles[s.Active].Name
		}
		tip = "WeeJ: " + strings.Join(ports, ", ") + " · " + name
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

	var items []winui.MenuItem

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

	board := s.Port != core.PortOff || app.forcedPort != ""
	items = append(items, winui.MenuItem{Text: app.tr("settings"), OnClick: func() { app.openSettings("") }})
	if board {
		items = append(items, winui.MenuItem{Text: app.tr("calibrate"), Disabled: !connected, OnClick: func() { app.startCalibration(false, false) }})
	}
	items = append(items, winui.MenuItem{Text: app.tr("language"), Children: app.languageMenuItems(s.Language)})

	items = append(items, winui.MenuItem{Separator: true})
	if board {
		items = append(items, winui.MenuItem{Text: app.statusLine(connected, busy, port, ""), Disabled: true})
	}
	if s.MixerPort != "" {
		mixerConnected, mixerBusy, mixerPort := app.mixerStatus()
		items = append(items, winui.MenuItem{Text: app.statusLine(mixerConnected, mixerBusy, mixerPort, s.MixerPort), Disabled: true})
	}
	items = append(items, winui.MenuItem{Text: app.tr("reconnect"), OnClick: app.requestReconnect})

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
	app.unmuteAll()
	if err := app.persistSettings(s); err != nil {
		app.log("Could not save settings: " + err.Error())
	}
	app.refreshTray()
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "profile", "profile": i})
	}
	app.showProfileHUD(s)
}

// statusLine is a device's line in the menu; waitingFor names a device picked but not found.
func (app *App) statusLine(connected, busy bool, port, waitingFor string) string {
	switch {
	case connected:
		return app.trVars("connected", v1("port", port))
	case busy:
		return app.trVars("port_busy", v1("port", port))
	case waitingFor != "":
		return app.trVars("waiting_for", v1("port", waitingFor))
	}
	return app.tr("not_connected")
}

func (app *App) requestReconnect() {
	for _, ch := range []chan struct{}{app.reconnectCh, app.mixerReconnectCh} {
		select {
		case ch <- struct{}{}:
		default:
		}
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
