//go:build windows

package app

import (
	"fmt"
	"strings"
	"time"

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

// isSMC tells whether the mixer read is an SMC-Mixer, whose controls are fixed.
func (app *App) isSMC() bool {
	port := app.sourcePort()
	return core.IsMidiPort(port) && core.IsSMCName(core.MidiDevice(port))
}

// activeColumns is the calibration of whatever is connected: the board's or the mixer's.
func (app *App) activeColumns(s core.Setup) []int {
	if app.usesMixer() {
		return s.ForMixer().Columns
	}
	return s.Columns
}

func (app *App) onMixerValues(values []int) {
	setup := app.snapshotSettings().ForMixer()
	app.handleValues(values, setup)
	app.showControls(values, setup.Columns)
}

// showControls keeps where each knob is for the mixer Settings draws, and hands it over at most
// 20 times a second, the last position always among them.
func (app *App) showControls(values, columns []int) {
	controls := make([]int, len(columns))
	for i, col := range columns {
		controls[i] = -1
		if col >= 0 && col < len(values) {
			controls[i] = values[col]
		}
	}
	app.controlsMu.Lock()
	defer app.controlsMu.Unlock()
	app.controls = controls
	if app.controlsTimer == nil && app.settingsWin != nil {
		app.controlsTimer = time.AfterFunc(50*time.Millisecond, func() {
			app.controlsMu.Lock()
			controls := app.controls
			app.controlsTimer = nil
			app.controlsMu.Unlock()
			if win := app.settingsWin; win != nil {
				win.Send(map[string]any{"type": "controls", "values": controls})
			}
		})
	}
}

func (app *App) lastControls() []int {
	app.controlsMu.Lock()
	defer app.controlsMu.Unlock()
	return app.controls
}

func (app *App) onStripLight(strip int, on bool) {
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "stripLight", "strip": strip, "on": on})
	}
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

func (app *App) onMixerButton(id int) {
	if app.isCalibrating() {
		app.log(fmt.Sprintf("Mixer button %s pressed while calibrating", mixerButtonName(id)))
		app.loop.Invoke(func() {
			cal := app.currentCalibrator()
			if cal == nil {
				return
			}
			cal.PressButton(id)
			app.refreshCalibration()
		})
		return
	}
	setup := app.snapshotSettings().Setup
	actions := setup.ActiveButtons()[id]
	if len(actions) == 0 {
		app.log(fmt.Sprintf("Mixer button %s pressed: empty", mixerButtonName(id)))
	} else {
		names := make([]string, len(actions))
		for i, a := range actions {
			names[i] = string(a)
		}
		app.log(fmt.Sprintf("Mixer button %s pressed: %s", mixerButtonName(id), strings.Join(names, ", ")))
	}
	for _, action := range actions {
		app.runButtonAction(id, action, setup)
	}

	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "mixerButton", "id": id})
	}
}

func (app *App) runButtonAction(id int, action core.ButtonAction, setup core.Setup) {
	if knob, ok := action.MuteKnob(); ok {
		mixer := setup.ForMixer()
		if knob < len(mixer.Columns) && mixer.Columns[knob] >= 0 {
			muted := app.engine.ToggleMute(mixer.Columns[knob], mixer)
			midiport.SetLED(id, muted)
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
