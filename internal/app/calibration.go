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
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
)

// calibrationTurnSeconds mirrors core's own private turnSeconds (20): it is
// only ever used for display text ("gets {n} seconds of turning"), never for
// timing logic, which always reads the calibrator's own Left()/Paused().
const calibrationTurnSeconds = "20"

func (app *App) startCalibration(onlyNew bool) {
	app.mu.Lock()
	if app.calibWin != nil {
		win := app.calibWin
		app.mu.Unlock()
		win.Focus()
		return
	}
	saved := app.snapshotSettings().Columns
	app.calibrator = core.NewCalibrator(saved, onlyNew)
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
			before := cal.StepKey()
			cal.Skip()
			app.soundIfStepChanged(before, cal.StepKey())
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
	result := cal.Result()
	saved := app.snapshotSettings().Columns
	changed := !intSliceEqual(result, saved)
	if changed {
		cur := app.snapshotSettings()
		cur.Columns = result
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
	app.openSettings("general")
	if wasOpen {
		if win := app.settingsWin; win != nil {
			win.Send(map[string]any{"type": "columns", "columns": result})
		}
	}
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
		before := cal.StepKey()
		cal.Feed(values, now)
		app.soundIfStepChanged(before, cal.StepKey())
		app.refreshCalibration()
	})
}

func (app *App) soundIfStepChanged(before, after core.StepKey) {
	if before != after {
		winui.StepSound()
	}
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

	if cal.Full() {
		button = "finish"
	} else if cal.CanSkip() {
		button = "skip"
	} else {
		button = "finish"
	}

	letter := core.Letter(cal.Knob())

	switch {
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
