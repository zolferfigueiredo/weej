//go:build windows

package app

import (
	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/midiport"
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
)

func (app *App) sourcePort() string {
	if app.forcedPort != "" {
		return app.forcedPort
	}
	return app.snapshotSettings().Port
}

func (app *App) usesMixer() bool { return core.IsMidiPort(app.sourcePort()) }

// activeColumns is the calibration of whatever is connected: the board's or the mixer's.
func (app *App) activeColumns(s core.Setup) []int {
	if app.usesMixer() {
		return s.ForMixer().Columns
	}
	return s.Columns
}

func (app *App) onMixerValues(values []int) {
	app.handleValues(values, app.snapshotSettings().Setup.ForMixer())
}

// pointOutMovedKnobs lights up a knob's row in Settings while its control moves, so it is easy
// to tell which knob is which.
func (app *App) pointOutMovedKnobs(values []int, setup core.Setup) {
	win := app.settingsWin
	app.movesMu.Lock()
	moved := app.moves.Moved(values)
	app.movesMu.Unlock()
	if win == nil {
		return
	}
	for _, col := range moved {
		for knob, c := range setup.Columns {
			if c == col {
				win.Send(map[string]any{"type": "knobMoved", "knob": knob})
			}
		}
	}
}

func (app *App) onMixerButton(cc int) {
	if app.isCalibrating() {
		app.loop.Invoke(func() {
			cal := app.currentCalibrator()
			if cal == nil {
				return
			}
			before := cal.StepKey()
			cal.PressButton(cc)
			app.soundIfStepChanged(before, cal.StepKey())
			app.refreshCalibration()
		})
		return
	}
	setup := app.snapshotSettings().Setup
	action := setup.Buttons[cc]

	if knob, ok := action.MuteKnob(); ok {
		mixer := setup.ForMixer()
		if knob < len(mixer.Columns) && mixer.Columns[knob] >= 0 {
			muted := app.engine.ToggleMute(mixer.Columns[knob], mixer)
			midiport.SetLED(cc, muted)
		}
	}
	switch action {
	case core.ActionPlayPause:
		winui.MediaKey(winui.VKMediaPlayPause)
	case core.ActionStop:
		winui.MediaKey(winui.VKMediaStop)
	case core.ActionPreviousTrack:
		winui.MediaKey(winui.VKMediaPrevTrack)
	case core.ActionNextTrack:
		winui.MediaKey(winui.VKMediaNextTrack)
	case core.ActionNextProfile:
		app.loop.Invoke(func() { app.onHotkey(nextProfileHotkeyID) })
	case core.ActionPreviousProfile:
		app.loop.Invoke(func() { app.onHotkey(previousProfileHotkeyID) })
	case core.ActionOpenSettings:
		app.loop.Invoke(func() { app.openSettings("") })
	}

	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "mixerButton", "cc": cc})
	}
}
