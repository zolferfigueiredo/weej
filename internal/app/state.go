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

func (app *App) setConnection(connected, busy bool, port string) {
	app.mu.Lock()
	app.connected, app.busy, app.currentPort = connected, busy, port
	app.mu.Unlock()
}

func (app *App) connectionStatus() (connected, busy bool, port string) {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.connected, app.busy, app.currentPort
}

func (app *App) setMixerConnection(connected, busy bool, port string) {
	app.mu.Lock()
	app.mixerConnected, app.mixerBusy, app.mixerPortName = connected, busy, port
	app.mu.Unlock()
}

func (app *App) mixerStatus() (connected, busy bool, port string) {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.mixerConnected, app.mixerBusy, app.mixerPortName
}

func (app *App) deviceConnected(mixer bool) bool {
	if mixer {
		connected, _, _ := app.mixerStatus()
		return connected
	}
	connected, _, _ := app.connectionStatus()
	return connected
}

func (app *App) setCalibrating(v bool) {
	app.mu.Lock()
	app.calibrating = v
	win := app.settingsWin
	app.mu.Unlock()
	if win != nil {
		win.Send(map[string]any{"type": "calibrating", "on": v})
	}
}

func (app *App) isCalibrating() bool {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.calibrating
}

func (app *App) currentCalibrator() *core.Calibrator {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.calibrator
}
