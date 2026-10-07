//go:build windows

package app

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/audio"
	"github.com/zolferfigueiredo/weej/internal/platform/midiport"
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
)

func (app *App) sourcePort() string {
	if app.forcedPort != "" {
		return app.forcedPort
	}
	return app.snapshotSettings().Port
}

// isSMC tells whether the mixer is an SMC-Mixer, whose controls are fixed.
func (app *App) isSMC() bool { return app.snapshotSettings().MixerIsSMC() }

// deviceColumns is the board's calibration, or the mixer's.
func deviceColumns(s core.Setup, mixer bool) []int {
	if mixer {
		return s.ForMixer().Columns
	}
	return s.Columns
}

// deviceName is how Settings tells the two apart.
func deviceName(mixer bool) string {
	if mixer {
		return "mixer"
	}
	return "board"
}

func (app *App) onMixerValues(values []int) {
	app.handleValues(app.mixerEngine, values, app.snapshotSettings().ForMixer(), true)
	app.showValues(true, &app.mixerLive, values)
}

// liveFrames keeps a device's last frame for the controls Settings draws.
type liveFrames struct {
	mu    sync.Mutex
	last  []int
	timer *time.Timer
}

func (l *liveFrames) frame() []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last
}

// showValues hands Settings a device's frames at most 20 times a second, the last always among
// them.
func (app *App) showValues(mixer bool, live *liveFrames, values []int) {
	live.mu.Lock()
	defer live.mu.Unlock()
	live.last = values
	if live.timer == nil && app.settingsWin != nil {
		live.timer = time.AfterFunc(50*time.Millisecond, func() {
			live.mu.Lock()
			values := live.last
			live.timer = nil
			live.mu.Unlock()
			if win := app.settingsWin; win != nil {
				win.Send(map[string]any{"type": "values", "device": deviceName(mixer), "values": values})
			}
		})
	}
}

// pointOutMovedKnobs lights up a knob's row in Settings while its control moves, so it is easy
// to tell which knob is which.
func (app *App) pointOutMovedKnobs(mixer bool, values []int, setup core.Setup) {
	win := app.settingsWin
	app.movesMu.Lock()
	watcher := &app.moves
	if mixer {
		watcher = &app.mixerMoves
	}
	moved := watcher.Moved(values)
	app.movesMu.Unlock()
	if win == nil {
		return
	}
	for _, col := range moved {
		for knob, c := range setup.Columns {
			if c == col {
				win.Send(map[string]any{"type": "knobMoved", "device": deviceName(mixer), "knob": knob})
			}
		}
	}
}

// unmuteAll runs before the profile changes: the next profile gives the knobs other jobs, so a
// muted one could no longer be unmuted from its button.
func (app *App) unmuteAll() {
	setup := app.snapshotSettings().Setup
	app.engine.UnmuteAll(setup)
	if app.mixerEngine.UnmuteAll(setup.ForMixer()) == 0 {
		return
	}
	for id, actions := range setup.ActiveButtons() {
		for _, a := range actions {
			if _, ok := a.MuteKnob(); ok {
				app.midiPort().SetLED(id, false)
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
	if cal := app.currentCalibrator(); cal != nil && cal.Mixer() {
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
		app.runButtonAction(action, app.mixerEngine, setup.ForMixer(), func(on bool) { app.midiPort().SetLED(id, on) })
	}

	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "mixerButton", "id": id})
	}
}

func (app *App) onBoardButton(knob int, setup core.Setup) {
	actions := setup.ActiveBoardButtons()[knob]
	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = string(a)
	}
	app.log(fmt.Sprintf("Board button %s pressed: %s", core.Letter(knob), strings.Join(names, ", ")))
	for _, action := range actions {
		app.runButtonAction(action, app.engine, setup, nil)
	}
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "boardButton", "knob": knob})
	}
}

// runButtonAction does one of a button's actions. A mute acts on the engine and setup of the
// device the button is on, and led shows the mute on the button if it has a light.
func (app *App) runButtonAction(action core.ButtonAction, engine *core.Engine, device core.Setup, led func(on bool)) {
	if knob, ok := action.MuteKnob(); ok {
		if knob < len(device.Columns) && device.Columns[knob] >= 0 {
			muted := engine.ToggleMute(device.Columns[knob], device)
			if led != nil {
				led(muted)
			}
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
	case core.ActionNextLights:
		app.changeLights(func(s *core.Setup) { s.MixerLights = core.NextLightPattern(s.MixerLights) })
	case core.ActionPreviousLights:
		app.changeLights(func(s *core.Setup) { s.MixerLights = core.PreviousLightPattern(s.MixerLights) })
	case core.ActionLightsOn:
		app.changeLights(func(s *core.Setup) { s.MixerLights = "on" })
	case core.ActionLightsOff:
		app.changeLights(func(s *core.Setup) { s.MixerLights = "" })
	}
}

func (app *App) midiPort() *midiport.Port {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.midi
}

// applyLights runs the mixer's button light pattern, listening to the speakers only for the EQ.
func (app *App) applyLights(s core.Setup) {
	app.midiPort().SetLights(s.MixerLights)
	eq := s.MixerLights == "eq" && s.MixerIsSMC()
	app.mu.Lock()
	if app.spectrum == nil {
		app.spectrum = core.NewSpectrum()
		midiport.SetSpectrum(app.spectrum)
	}
	stop := app.loopback
	if eq && stop == nil {
		app.loopback = audio.StartLoopback(app.spectrum, app.log)
	}
	if eq {
		stop = nil
	} else {
		app.loopback = nil
	}
	app.mu.Unlock()
	if stop != nil {
		stop.Stop()
	}
}

// changeLights changes the button lights from a button and saves them, as Settings would.
func (app *App) changeLights(change func(*core.Setup)) {
	app.loop.Invoke(func() {
		s := app.snapshotSettings()
		change(&s.Setup)
		if err := app.persistSettings(s); err != nil {
			return
		}
		app.applyLights(s.Setup)
		if win := app.settingsWin; win != nil {
			win.Send(map[string]any{"type": "mixerLights", "pattern": s.MixerLights})
		}
	})
}
