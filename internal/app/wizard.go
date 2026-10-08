//go:build windows

package app

import (
	"sync"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"
)

// wizardRun is a board being calibrated: its frames and presses go to the wizard instead of its
// jobs and buttons, and Settings shows each step.
type wizardRun struct {
	mu     sync.Mutex
	device string
	cal    *core.Calibration
	stop   chan struct{}
}

func (app *App) wizardFor(id string) *wizardRun {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.wizard != nil && app.wizard.device == id {
		return app.wizard
	}
	return nil
}

// startWizard calibrates the given controls of a board, every one when none are given; another
// board being calibrated stops first.
func (app *App) startWizard(id string, controls []int) {
	d, ok := app.device(id)
	if !ok || d.Type == core.DeviceSMC {
		return
	}
	if len(controls) == 0 {
		controls = make([]int, len(d.Controls))
		for i := range controls {
			controls[i] = i
		}
	}
	app.endWizard(false)
	w := &wizardRun{device: id, cal: core.NewCalibration(d, controls), stop: make(chan struct{})}
	app.mu.Lock()
	app.wizard = w
	app.mu.Unlock()
	go func() {
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-tick.C:
				app.pushWizard()
			}
		}
	}()
	app.pushWizard()
}

func (w *wizardRun) feed(raw []int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cal.Feed(raw)
}

func (w *wizardRun) press(id int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cal.Press(id)
}

// wizardPayload is the step Settings shows, or nil when no board is being calibrated.
func (app *App) wizardPayload() map[string]any {
	app.mu.Lock()
	w := app.wizard
	app.mu.Unlock()
	if w == nil {
		return nil
	}
	w.mu.Lock()
	s, done := w.cal.State(), w.cal.Done()
	w.mu.Unlock()
	return map[string]any{
		"device": w.device, "control": s.Control, "kind": s.Kind, "stage": s.Stage, "count": s.Count,
		"need": s.Need, "warning": s.Warning, "other": s.Other, "index": s.Index,
		"total": s.Total, "done": done,
	}
}

func (app *App) pushWizard() {
	win := app.settingsWin
	if win == nil {
		return
	}
	if p := app.wizardPayload(); p != nil {
		p["type"] = "wizard"
		win.Send(p)
	}
}

// wizardOp is a button in the wizard: next, skip, redo, cancel or finish.
func (app *App) wizardOp(op string) {
	app.mu.Lock()
	w := app.wizard
	app.mu.Unlock()
	if w == nil {
		return
	}
	switch op {
	case "next":
		w.mu.Lock()
		w.cal.Next()
		w.mu.Unlock()
		app.pushWizard()
	case "skip":
		w.mu.Lock()
		w.cal.Skip()
		w.mu.Unlock()
		app.pushWizard()
	case "redo":
		w.mu.Lock()
		w.cal.Redo()
		w.mu.Unlock()
		app.pushWizard()
	case "finish":
		app.endWizard(true)
	default:
		app.endWizard(false)
	}
}

// endWizard stops calibrating, keeping what was found when apply is set.
func (app *App) endWizard(apply bool) {
	app.mu.Lock()
	w := app.wizard
	app.wizard = nil
	app.mu.Unlock()
	if w == nil {
		return
	}
	close(w.stop)
	if apply {
		w.mu.Lock()
		controls := w.cal.Result()
		w.mu.Unlock()
		s := app.snapshotSettings()
		if i := deviceIndex(s, w.device); i >= 0 {
			s.Devices[i].Controls = controls
			app.applySettings(s)
			app.sendDevice(s.Devices[i])
		}
	}
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "wizard", "device": w.device, "end": true})
	}
}
