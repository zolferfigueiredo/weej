//go:build windows

package app

import (
	"encoding/json"
	"time"

	"github.com/zolferfigueiredo/weej/internal/ui/web"
)

// The job menu is its own popup window so it can hang past the edge of Settings. Settings
// owns the list (the catalog and the knob's unsaved jobs live in that page), so it sends the
// whole menu here, and every tick in the popup goes straight back to it.

// openJobMenu runs on the main loop, deferred from the Settings page's message.
func (app *App) openJobMenu(data []byte) {
	var msg struct {
		Knob   int             `json:"knob"`
		Anchor web.Rect        `json:"anchor"`
		Model  json.RawMessage `json:"model"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	app.mu.Lock()
	owner, open, openKnob := app.settingsWin, app.jobMenuWin, app.jobMenuKnob
	closedAt, closedKnob := app.jobMenuClosedAt, app.jobMenuClosedKnob
	app.mu.Unlock()
	if owner == nil {
		return
	}
	// Clicking the knob whose menu is open closes it, as a dropdown's own button does. The
	// click usually closed it already, by taking focus from it just before landing here.
	if open != nil {
		open.Close()
		if openKnob == msg.Knob {
			return
		}
	} else if closedKnob == msg.Knob && time.Since(closedAt) < 300*time.Millisecond {
		return
	}

	var w *web.Window
	w, err := web.Open(app.loop.Invoke, "jobs", web.Options{
		Title: "WeeJ", Width: 320, Height: 480,
		Owner: owner, Anchor: &msg.Anchor,
		OnClose: func() {
			app.mu.Lock()
			if app.jobMenuWin == w {
				app.jobMenuWin = nil
			}
			app.jobMenuClosedAt = time.Now()
			app.jobMenuClosedKnob = msg.Knob
			app.mu.Unlock()
		},
	}, func(data []byte) { app.onJobMenuMessage(msg.Knob, msg.Model, data) })
	if err != nil {
		app.log("Could not open the job menu: " + err.Error())
		return
	}
	app.mu.Lock()
	app.jobMenuWin = w
	app.jobMenuKnob = msg.Knob
	app.mu.Unlock()
}

func (app *App) closeJobMenu() {
	app.mu.Lock()
	w := app.jobMenuWin
	app.mu.Unlock()
	if w != nil {
		w.Close()
	}
}

func (app *App) onJobMenuMessage(knob int, model json.RawMessage, data []byte) {
	var msg struct {
		Type    string          `json:"type"`
		Job     json.RawMessage `json:"job"`
		Checked bool            `json:"checked"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	app.mu.Lock()
	menu, settings := app.jobMenuWin, app.settingsWin
	app.mu.Unlock()

	switch msg.Type {
	case "ready":
		if menu != nil {
			payload := app.baseInitFields()
			payload["knob"] = knob
			payload["model"] = model
			menu.Send(payload)
		}
	case "toggle":
		if settings != nil {
			settings.Send(map[string]any{"type": "jobMenuToggle", "knob": knob, "job": msg.Job, "checked": msg.Checked})
		}
	case "clear":
		if settings != nil {
			settings.Send(map[string]any{"type": "jobMenuClear", "knob": knob})
		}
	case "pickApp":
		app.closeJobMenu()
		app.loop.Invoke(func() { app.handleSettingsPickApp(data) })
	case "close":
		app.closeJobMenu()
	}
}
