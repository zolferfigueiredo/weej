//go:build windows

package app

import (
	"fmt"
	"strings"

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

// unmuteAll runs before the profile changes: the next profile gives the knobs other jobs, so a
// muted one could no longer be unmuted from its button.
func (app *App) unmuteAll() {
	setup := app.snapshotSettings().Setup
	if app.engine.UnmuteAll(setup.ForMixer()) == 0 || !app.usesMixer() {
		return
	}
	for id, actions := range setup.ActiveButtons() {
		for _, a := range actions {
			if _, ok := a.MuteKnob(); ok {
				midiport.SetLED(id, false)
			}
		}
	}
}

func mixerButtonName(id int) string {
	if note, ok := core.MixerButtonNote(id); ok {
		return fmt.Sprintf("note %d", note)
	}
	return fmt.Sprintf("CC %d", id)
}

func (app *App) onMixerButton(cc int) {
	if app.isCalibrating() {
		app.log(fmt.Sprintf("Mixer button %s pressed while calibrating", mixerButtonName(cc)))
		app.loop.Invoke(func() {
			cal := app.currentCalibrator()
			if cal == nil {
				return
			}
			cal.PressButton(cc)
			app.refreshCalibration()
		})
		return
	}
	setup := app.snapshotSettings().Setup
	actions := setup.ActiveButtons()[cc]
	if len(actions) == 0 {
		app.log(fmt.Sprintf("Mixer button %s pressed: empty", mixerButtonName(cc)))
	} else {
		names := make([]string, len(actions))
		for i, a := range actions {
			names[i] = string(a)
		}
		app.log(fmt.Sprintf("Mixer button %s pressed: %s", mixerButtonName(cc), strings.Join(names, ", ")))
	}
	for _, action := range actions {
		app.runButtonAction(cc, action, setup)
	}

	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "mixerButton", "cc": cc})
	}
}

func (app *App) runButtonAction(cc int, action core.ButtonAction, setup core.Setup) {
	if knob, ok := action.MuteKnob(); ok {
		mixer := setup.ForMixer()
		if knob < len(mixer.Columns) && mixer.Columns[knob] >= 0 {
			muted := app.engine.ToggleMute(mixer.Columns[knob], mixer)
			midiport.SetLED(cc, muted)
		}
	}
	if path, ok := action.OpenApp(); ok && !winui.FocusApp(core.ExeName(path)) {
		launch(path)
	}
	if exe, ok := action.CloseApp(); ok {
		winui.CloseApp(exe)
	}
	if url, ok := action.URL(); ok {
		openURL(url)
	}
	if keys, ok := action.Keys(); ok {
		winui.SendShortcut(keys.Mods, keys.VK)
	}
	if i, ok := action.Profile(); ok {
		app.loop.Invoke(func() { app.onHotkey(i) })
	}
	switch action {
	case core.ActionPlayPause:
		winui.MediaKey(winui.VKMediaPlayPause)
	case core.ActionPlay:
		winui.AppCommand(winui.AppCommandMediaPlay)
	case core.ActionPause:
		winui.AppCommand(winui.AppCommandMediaPause)
	case core.ActionVolumeUp:
		winui.MediaKey(winui.VKVolumeUp)
	case core.ActionVolumeDown:
		winui.MediaKey(winui.VKVolumeDown)
	case core.ActionMuteAll:
		winui.MediaKey(winui.VKVolumeMute)
	case core.ActionMuteMic:
		if app.audio != nil {
			app.audio.ToggleMicMute()
		}
	case core.ActionNightLight:
		app.nightlight.Toggle()
	case core.ActionScreensOff:
		winui.ScreensOff()
	case core.ActionLockPC:
		winui.LockPC()
	case core.ActionSleepPC:
		winui.SleepPC()
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
}
