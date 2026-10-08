//go:build windows

package app

import (
	"github.com/zolferfigueiredo/weej/internal/platform/sys"

	"github.com/zolferfigueiredo/weej/internal/core"
)

func (app *App) snapshotSettings() core.Settings {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.settings
}

func (app *App) replaceSettings(s core.Settings) {
	app.mu.Lock()
	app.settings = s
	app.mu.Unlock()
}

func (app *App) persistSettings(s core.Settings) error {
	data, err := core.EncodeSettings(s)
	if err != nil {
		return err
	}
	if err := sys.WriteFileAtomic(sys.SettingsPath(), data); err != nil {
		app.log("Could not save settings: " + err.Error())
		return err
	}
	app.replaceSettings(s)
	return nil
}
