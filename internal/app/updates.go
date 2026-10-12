//go:build windows

package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/ui/web"
	"github.com/zolferfigueiredo/weej/internal/updater"
)

func (app *App) setupAutoUpdateChecks() {
	app.loop.Every(time.Hour, app.autoCheckUpdate)
	app.autoCheckUpdate()
}

func (app *App) fetchRelease(ctx context.Context) (updater.Release, error) {
	return updater.Check(ctx, app.updateSite())
}

// checkUpdateComplete says so in a notification, since an update installed on its own restarts
// WeeJ without anyone watching.
func (app *App) checkUpdateComplete() {
	s := app.snapshotSettings()
	if s.UpdatedTo == "" || s.UpdatedTo != core.AppVersion {
		return
	}
	title, text := app.tr("update_complete"), app.trVars("now_using", v1("version", core.AppVersion))
	if s.HideIcon {
		app.loop.Alert(title, text)
	} else {
		// Queued behind refreshTray's Invoke, which adds the icon a notification needs.
		app.loop.Invoke(func() { app.tray.Balloon(title, text) })
	}
	s2 := app.snapshotSettings()
	s2.UpdatedTo = ""
	if err := app.persistSettings(s2); err != nil {
		app.log("Could not save settings: " + err.Error())
	}
}

func (app *App) postTestNotification() {
	app.notifyUpdateAvailable(core.NextPatch(core.AppVersion))
}

// installsAutomatically is Install automatically where it can work: only the installed copy
// updates itself.
func (app *App) installsAutomatically() bool {
	return app.snapshotSettings().InstallUpdates && updater.WhichCopy(app.self) == updater.Installed
}

func (app *App) autoCheckUpdate() {
	cur := app.snapshotSettings()
	if !core.IsDue(cur.UpdateEvery, cur.LastUpdateCheck, time.Now()) {
		return
	}
	go func() {
		rel, err := app.fetchRelease(context.Background())
		app.loop.Invoke(func() {
			s := app.snapshotSettings()
			s.LastUpdateCheck = time.Now()
			if err == nil {
				s.AvailableVersion = rel.Version
				app.mu.Lock()
				app.lastRelease = rel
				app.mu.Unlock()
			}
			if err := app.persistSettings(s); err != nil {
				app.log("Could not save settings: " + err.Error())
			}
			if err != nil {
				return
			}
			if core.IsNewer(rel.Version, core.AppVersion) {
				if app.installsAutomatically() {
					app.install(rel, false)
				} else {
					app.notifyOnce(rel.Version)
				}
			}
			app.refreshTray()
		})
	}()
}

func (app *App) notifyOnce(version string) {
	s := app.snapshotSettings()
	if s.NotifiedVersion == version {
		return
	}
	s.NotifiedVersion = version
	if err := app.persistSettings(s); err != nil {
		app.log("Could not save settings: " + err.Error())
	}
	app.notifyUpdateAvailable(version)
}

func (app *App) notifyUpdateAvailable(version string) {
	if app.snapshotSettings().HideIcon {
		choice := app.loop.Ask(
			app.trVars("available", v1("version", version)),
			app.trVars("update_question", v1("version", core.AppVersion)),
			[]string{app.tr("update_now"), app.tr("later")},
		)
		if choice == 0 {
			app.startUpdateNow()
		}
		return
	}
	app.tray.Balloon(
		app.trVars("available", v1("version", version)),
		app.trVars("click_to_update", v1("version", core.AppVersion)),
	)
}

// onBalloonClick updates, as the notification of a new version says a click does. The one after
// an update has nothing to do.
func (app *App) onBalloonClick() {
	if v := app.snapshotSettings().AvailableVersion; v != "" && core.IsNewer(v, core.AppVersion) {
		app.startUpdateNow()
	}
}

func (app *App) manualCheckUpdate() {
	app.mu.Lock()
	if app.checking {
		app.mu.Unlock()
		return
	}
	app.checking = true
	app.mu.Unlock()

	go func() {
		rel, err := app.fetchRelease(context.Background())
		app.loop.Invoke(func() {
			app.mu.Lock()
			app.checking = false
			app.mu.Unlock()

			s := app.snapshotSettings()
			s.LastUpdateCheck = time.Now()
			if err == nil {
				s.AvailableVersion = rel.Version
				app.mu.Lock()
				app.lastRelease = rel
				app.mu.Unlock()
			}
			if err := app.persistSettings(s); err != nil {
				app.log("Could not save settings: " + err.Error())
			}

			if err != nil {
				app.loop.Alert(app.tr("check_failed"), app.tr("check_connection"))
				return
			}
			if !core.IsNewer(rel.Version, core.AppVersion) {
				app.loop.Alert(app.tr("up_to_date"), app.trVars("newest", v1("version", core.AppVersion)))
				return
			}
			if app.installsAutomatically() {
				app.install(rel, true)
				return
			}
			choice := app.loop.Ask(
				app.trVars("available", v1("version", rel.Version)),
				app.trVars("update_question", v1("version", core.AppVersion)),
				[]string{app.tr("update_now"), app.tr("later")},
			)
			if choice == 0 {
				app.startUpdateNow()
			}
		})
	}()
}

func (app *App) startUpdateNow() {
	switch updater.WhichCopy(app.self) {
	case updater.Other:
		choice := app.loop.Ask(app.tr("update_failed"), app.tr("installed_only"), []string{app.tr("download"), app.tr("cancel")})
		if choice == 0 {
			app.openUpdatePageURL()
		}
	default:
		version := app.snapshotSettings().AvailableVersion
		app.mu.Lock()
		rel := app.lastRelease
		app.mu.Unlock()
		if rel.Version != version {
			rel = updater.Release{Version: version}
		}
		app.install(rel, true)
	}
}

func (app *App) openUpdatePageURL() {
	v := app.snapshotSettings().AvailableVersion
	if v == "" {
		return
	}
	openURL(updater.PageURL(v))
}

// install puts the update in place and restarts WeeJ into it. With a window it shows how far it
// got and asks what to do if it fails; without one, as Install automatically runs it, a failure
// only notifies, which leaves the update a click away.
func (app *App) install(rel updater.Release, withWindow bool) {
	app.mu.Lock()
	if app.installing {
		app.mu.Unlock()
		return
	}
	pending := app.restartPending
	if !pending {
		app.installing = true
		app.installStep = updater.Downloading
	}
	app.mu.Unlock()
	if pending {
		if withWindow {
			app.restartIntoUpdate()
		}
		return
	}

	if withWindow {
		app.openUpdateWindow()
		app.pushUpdateProgress(rel.Version, updater.Downloading, false)
	}

	go func() {
		ctx := context.Background()
		var err error
		// After a restart WeeJ remembers the version it found but not its checksum.
		if rel.SHA256 == "" {
			var fetched updater.Release
			if fetched, err = app.fetchRelease(ctx); err == nil {
				rel = fetched
			}
		}
		if err == nil {
			err = updater.Install(ctx, app.updateSite(), rel, app.self, func(step updater.Step) {
				app.loop.Invoke(func() {
					app.mu.Lock()
					app.installStep = step
					app.mu.Unlock()
					app.pushUpdateProgress(rel.Version, step, step == updater.Done)
				})
			})
		}
		app.loop.Invoke(func() { app.installDone(rel.Version, err, withWindow) })
	}()
}

func (app *App) installDone(version string, err error, withWindow bool) {
	app.mu.Lock()
	app.installing = false
	app.mu.Unlock()

	if err != nil {
		app.log("Could not install the update: " + err.Error())
		if !withWindow {
			app.notifyOnce(version)
			app.refreshTray()
			return
		}
		app.closeUpdateWindow()
		choice := app.loop.Ask(app.tr("update_failed"), translateUpdaterError(app, err), []string{app.tr("download"), app.tr("cancel")})
		if choice == 0 {
			app.openUpdatePageURL()
		}
		return
	}

	s := app.snapshotSettings()
	s.UpdatedTo = version
	if err := app.persistSettings(s); err != nil {
		app.log("Could not save settings: " + err.Error())
	}
	// Settings may hold edits not applied yet, so an update nobody asked for waits for it to close.
	if !withWindow && app.settingsWin != nil {
		app.mu.Lock()
		app.restartPending = true
		app.mu.Unlock()
		app.refreshTray()
		return
	}
	app.restartIntoUpdate()
}

func (app *App) restartIntoUpdate() {
	if err := updater.Reopen(app.self, app.launchArgs); err != nil {
		app.closeUpdateWindow()
		app.loop.Alert(core.AppName, app.trVars("reopen_failed", v1("error", err.Error())))
		return
	}
	app.shutdown()
	app.loop.Quit(0)
}

func translateUpdaterError(app *App, err error) string {
	if uerr, ok := err.(*updater.Error); ok {
		return app.tr(uerr.Key)
	}
	return app.tr("download_failed")
}

func (app *App) openUpdateWindow() {
	win, err := web.Open(app.loop.Invoke, "update", web.Options{
		Title: core.AppName, Width: 420, Height: 160, NoClose: true,
		OnClose: func() {
			app.mu.Lock()
			app.updateWin = nil
			app.mu.Unlock()
		},
	}, app.onUpdateMessage)
	if err != nil {
		app.log("Could not open the update window: " + err.Error())
		return
	}
	app.mu.Lock()
	app.updateWin = win
	app.mu.Unlock()
}

func (app *App) closeUpdateWindow() {
	if win := app.updateWin; win != nil {
		win.Close()
	}
}

func (app *App) pushUpdateProgress(version string, step updater.Step, done bool) {
	win := app.updateWin
	if win == nil {
		return
	}
	win.Send(map[string]any{
		"type":    "update",
		"heading": app.trVars("updating_to", v1("version", version)),
		"status":  app.updateStepText(step, version),
		"done":    done,
	})
}

func (app *App) onUpdateMessage(data []byte) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return
	}
	if probe.Type == "ready" {
		if win := app.updateWin; win != nil {
			payload := app.baseInitFields()
			payload["icon"] = appIconDataURL(64)
			win.Send(payload)
		}
	}
}
