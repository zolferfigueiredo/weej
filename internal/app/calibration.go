//go:build windows

package app

import (
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/lang"
	"github.com/zolferfigueiredo/weej/internal/ui/web"
)

// calibrationTurnSeconds mirrors core's own private turnSeconds (20): it is
// only ever used for display text ("gets {n} seconds of turning"), never for
// timing logic, which always reads the calibrator's own Left()/Paused().
const calibrationTurnSeconds = "20"

func (app *App) startCalibration(onlyNew bool) {
	// Read before taking app.mu: snapshotSettings, under both of these, locks it too.
	saved := app.activeColumns(app.snapshotSettings().Setup)
	mixer := app.usesMixer()
	app.mu.Lock()
	if app.calibWin != nil {
		win := app.calibWin
		app.mu.Unlock()
		win.Focus()
		return
	}
	if mixer {
		// A full mixer calibration starts empty, so the mixer has as many knobs as get calibrated.
		if !onlyNew {
			saved = nil
		}
		app.calibrator = core.NewMixerCalibrator(saved, onlyNew)
	} else {
		app.calibrator = core.NewCalibrator(saved, onlyNew)
	}
	app.calibOnlyNew = onlyNew
	app.mu.Unlock()
	app.setCalibrating(true)

	win, err := web.Open(app.loop.Invoke, "calibration", web.Options{
		Title: app.tr("calibration_title"), Width: 440, Height: 320,
		OnClose: app.onCalibrationClosed,
	}, app.onCalibrationMessage)
	if err != nil {
		app.log("Could not open Calibration: " + err.Error())
		app.mu.Lock()
		app.calibrator = nil
		app.mu.Unlock()
		app.setCalibrating(false)
		return
	}

	app.mu.Lock()
	app.calibWin = win
	app.calibTickStop = app.loop.Every(250*time.Millisecond, app.refreshCalibration)
	app.mu.Unlock()
	app.refreshCalibration()
}

func (app *App) onCalibrationMessage(data []byte) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return
	}
	switch probe.Type {
	case "ready":
		if win := app.calibWin; win != nil {
			win.Send(app.baseInitFields())
		}
		app.refreshCalibration()
	case "skip":
		if cal := app.currentCalibrator(); cal != nil {
			cal.Skip()
		}
		app.refreshCalibration()
	case "finish":
		app.finishCalibration()
	case "cancel":
		app.cancelCalibration()
	}
}

func (app *App) onCalibrationClosed() {
	app.mu.Lock()
	if app.calibTickStop != nil {
		app.calibTickStop()
		app.calibTickStop = nil
	}
	app.calibWin = nil
	app.calibrator = nil
	app.mu.Unlock()
	app.setCalibrating(false)
}

func (app *App) cancelCalibration() {
	if win := app.calibWin; win != nil {
		win.Close()
	}
}

func (app *App) finishCalibration() {
	cal := app.currentCalibrator()
	if cal == nil {
		return
	}
	if cal.Mixer() && !cal.ButtonStage() {
		cal.StartButtons()
		app.refreshCalibration()
		return
	}
	result := cal.Result()
	mixer := cal.Mixer()
	cur := app.snapshotSettings()
	saved := app.activeColumns(cur.Setup)
	changed := !intSliceEqual(result, saved)
	if buttons := cal.Buttons(); mixer && len(buttons) > 0 && !intSliceEqual(buttons, cur.ButtonOrder) {
		cur.ButtonOrder = buttons
		profiles := make([]core.Profile, len(cur.Profiles))
		for i, p := range cur.Profiles {
			kept := core.ButtonMap{}
			for _, cc := range buttons {
				if a, ok := p.Buttons[cc]; ok {
					kept[cc] = a
				}
			}
			p.Buttons = kept
			profiles[i] = p
		}
		cur.Profiles = profiles
		changed = true
	}
	if changed {
		if mixer {
			cur.MixerColumns = result
		} else {
			cur.Columns = result
		}
		if err := app.persistSettings(cur); err != nil {
			app.log("Could not save settings: " + err.Error())
		}
	}

	app.mu.Lock()
	wasOpen := app.settingsWin != nil
	app.mu.Unlock()

	if win := app.calibWin; win != nil {
		win.Close()
	}
	if !changed {
		return
	}
	// Finish arrives inside a WebView2 callback; opening a window there would nest message loops.
	app.loop.Invoke(func() {
		app.openSettings("general")
		if wasOpen {
			if win := app.settingsWin; win != nil {
				buttons := make([]core.ButtonMap, len(cur.Profiles))
				for i, p := range cur.Profiles {
					buttons[i] = p.Buttons
				}
				win.Send(map[string]any{
					"type": "columns", "mixer": mixer, "columns": columnsToJSON(result),
					"buttonOrder": cur.ButtonOrder, "profileButtons": buttons,
				})
			}
		}
	})
}

func intSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (app *App) feedCalibrator(values []int, now float64) {
	app.loop.Invoke(func() {
		cal := app.currentCalibrator()
		if cal == nil {
			return
		}
		cal.Feed(values, now)
		app.refreshCalibration()
	})
}

func (app *App) refreshCalibration() {
	win := app.calibWin
	cal := app.currentCalibrator()
	if win == nil || cal == nil {
		return
	}
	code := app.currentLanguage()
	connected, _, _ := app.connectionStatus()
	app.mu.Lock()
	onlyNew := app.calibOnlyNew
	app.mu.Unlock()

	title, body, progress, warning, button := composeCalibrationTexts(cal, code, onlyNew, connected)
	win.Send(map[string]any{
		"type": "step", "title": title, "body": body, "progress": progress,
		"warning": warning, "button": button,
	})
}

func composeCalibrationTexts(cal *core.Calibrator, code string, onlyNew, connected bool) (title, body, progress string, warning bool, button string) {
	tr := func(key string, vars map[string]string) string { return lang.T(code, key, vars) }

	if cal.ButtonStage() {
		return composeButtonTexts(cal, code)
	}

	if cal.Full() {
		button = "finish"
	} else if cal.CanSkip() {
		button = "skip"
	} else {
		button = "finish"
	}
	if cal.Mixer() && button == "finish" {
		button = "continue"
	}

	letter := core.Letter(cal.Knob())

	switch {
	case cal.Full() && cal.Mixer():
		title = tr("cal.all_found", nil)
		body = tr("cal.all_found_mixer", map[string]string{"n": strconv.Itoa(len(cal.Found()))})
	case cal.Full():
		title = tr("cal.all_found", nil)
		body = tr("cal.all_found_text", map[string]string{"n": strconv.Itoa(len(cal.Found()))})
	default:
		title = tr("knob", map[string]string{"letter": letter})
		body = composeCalibrationBody(cal, code, onlyNew, letter)
	}

	switch {
	case cal.Full():
		progress = lang.Plural(code, "knobs_found", len(cal.Found()), nil)
	case cal.WrongKnob() >= 0:
		warning = true
		progress = tr("cal.wrong", map[string]string{"wrong": core.Letter(cal.WrongKnob()), "letter": letter})
	case cal.Phase() == 0:
		if connected {
			progress = tr("cal.waiting_knob", map[string]string{"letter": letter})
		} else {
			progress = tr("cal.waiting_board", nil)
		}
	default: // phase 1
		left := int(math.Ceil(cal.Left()))
		if left < 0 {
			left = 0
		}
		leftPhrase := lang.Plural(code, "seconds_left", left, nil)
		if cal.Paused() {
			warning = true
			progress = tr("cal.paused", map[string]string{"letter": letter, "left": leftPhrase})
		} else {
			progress = leftPhrase
		}
	}
	return title, body, progress, warning, button
}

func composeCalibrationBody(cal *core.Calibrator, code string, onlyNew bool, letter string) string {
	tr := func(key string, vars map[string]string) string { return lang.T(code, key, vars) }
	var parts []string

	if cal.Phase() == 0 {
		if cal.Knob() == cal.First() {
			parts = append(parts, tr("cal.move_first", map[string]string{"letter": letter, "n": calibrationTurnSeconds}))
		} else {
			parts = append(parts, tr("cal.move", map[string]string{"letter": letter}))
		}
		switch {
		case !cal.CanSkip() && cal.Mixer():
			parts = append(parts, tr("cal.no_knob_mixer", map[string]string{"letter": letter}))
		case !cal.CanSkip():
			parts = append(parts, tr("cal.no_knob", map[string]string{"letter": letter}))
		case onlyNew:
			parts = append(parts, tr("cal.skip_keeps", nil))
		default:
			parts = append(parts, tr("cal.finish_later", nil))
		}
		if cal.Knob() == cal.First() {
			parts = append(parts, tr("cal.hold", nil))
		}
	} else {
		parts = append(parts, tr("cal.turn", map[string]string{"n": calibrationTurnSeconds}))
	}
	return lang.Join(code, parts...)
}

func composeButtonTexts(cal *core.Calibrator, code string) (title, body, progress string, warning bool, button string) {
	tr := func(key string, vars map[string]string) string { return lang.T(code, key, vars) }
	buttons := cal.Buttons()
	n := map[string]string{"n": strconv.Itoa(len(buttons) + 1)}

	title = tr("mixer.button", n)
	body = tr("cal.press_button", n)
	switch {
	case cal.RepeatedButton() >= 0:
		warning = true
		progress = tr("cal.button_again", map[string]string{"n": strconv.Itoa(cal.RepeatedButton() + 1)})
	case len(buttons) == 0:
		progress = tr("cal.waiting_button", nil)
	default:
		progress = tr("cal.buttons_so_far", map[string]string{"n": strconv.Itoa(len(buttons))})
	}
	return title, body, progress, warning, "finish"
}
