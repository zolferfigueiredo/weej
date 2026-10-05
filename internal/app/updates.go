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

func (app *App) checkUpdateComplete() {
	s := app.snapshotSettings()
	if s.UpdatedTo == "" || s.UpdatedTo != core.AppVersion {
		return
	}
	app.loop.Alert(app.tr("update_complete"), app.trVars("now_using", v1("version", core.AppVersion)))
	s2 := app.snapshotSettings()
	s2.UpdatedTo = ""
	if err := app.persistSettings(s2); err != nil {
		app.log("Could not save settings: " + err.Error())
	}
}

func (app *App) postTestNotification() {
	app.notifyUpdateAvailable(core.NextPatch(core.AppVersion))
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
			if core.IsNewer(rel.Version, core.AppVersion) && s.NotifiedVersion != rel.Version {
				s2 := app.snapshotSettings()
				s2.NotifiedVersion = rel.Version
				if err := app.persistSettings(s2); err != nil {
					app.log("Could not save settings: " + err.Error())
				}
				app.notifyUpdateAvailable(rel.Version)
			}
			app.refreshTray()
		})
	}()
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
		app.beginInstall()
	}
}

func (app *App) openUpdatePageURL() {
	v := app.snapshotSettings().AvailableVersion
	if v == "" {
		return
	}
	openURL(updater.PageURL(v))
}

func (app *App) beginInstall() {
	app.mu.Lock()
	if app.installing {
		app.mu.Unlock()
		return
	}
	app.installing = true
	app.installStep = updater.Downloading
	rel := app.lastRelease
	app.mu.Unlock()

	version := app.snapshotSettings().AvailableVersion
	if rel.Version != version {
		rel.Version = version
	}

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
		app.mu.Lock()
		app.installing = false
		app.mu.Unlock()
		return
	}
	app.mu.Lock()
	app.updateWin = win
	app.mu.Unlock()
	app.pushUpdateProgress(version, updater.Downloading, false)

	go func() {
		installErr := updater.Install(context.Background(), app.updateSite(), rel, app.self, func(step updater.Step) {
			app.loop.Invoke(func() {
				app.mu.Lock()
				app.installStep = step
				app.mu.Unlock()
				app.pushUpdateProgress(version, step, step == updater.Done)
			})
		})
		app.loop.Invoke(func() {
			app.mu.Lock()
			app.installing = false
			app.mu.Unlock()

			if installErr != nil {
				app.closeUpdateWindow()
				reason := translateUpdaterError(app, installErr)
				choice := app.loop.Ask(app.tr("update_failed"), reason, []string{app.tr("download"), app.tr("cancel")})
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
		})
	}()
}

func translateUpdaterError(app *App, err error) string {
	if uerr, ok := err.(*updater.Error); ok {
		return app.tr(uerr.Key)
	}
	return app.tr("download_failed")
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
	switch probe.Type {
	case "ready":
		if win := app.updateWin; win != nil {
			payload := app.baseInitFields()
			payload["icon"] = appIconDataURL(64)
			win.Send(payload)
		}
	case "reopen":
		app.loop.Invoke(func() {
			if err := updater.Reopen(app.self, app.launchArgs); err != nil {
				app.loop.Alert(core.AppName, app.trVars("reopen_failed", v1("error", err.Error())))
				return
			}
			app.shutdown()
			app.loop.Quit(0)
		})
	}
}
