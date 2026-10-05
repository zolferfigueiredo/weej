//go:build windows

package app

import (
	"encoding/json"
	"math"
	"time"

	"github.com/zolferfigueiredo/weej/internal/ui/web"
)

// The job menu is its own popup window so it can hang past the edge of Settings. Settings
// owns the list (the catalog and the knob's unsaved jobs live in that page), so it sends the
// whole menu here, and every tick in the popup goes straight back to it.
//
// The popup is made once, hidden, as soon as Settings has loaded, and only hides between
// opens: building a WebView on every click took too long.

// prepareJobMenu runs on the main loop once the Settings page is ready.
func (app *App) prepareJobMenu() {
	app.mu.Lock()
	owner, existing := app.settingsWin, app.jobMenuWin
	app.mu.Unlock()
	if owner == nil || existing != nil {
		return
	}

	var w *web.Window
	w, err := web.Open(app.loop.Invoke, "jobs", web.Options{
		Title: "WeeJ", Width: 280, Height: 440, MaxHeight: 440,
		Owner: owner,
		OnHide: func() {
			app.mu.Lock()
			app.jobMenuShown = false
			app.jobMenuHiddenAt = time.Now()
			app.mu.Unlock()
		},
		OnClose: func() {
			app.mu.Lock()
			if app.jobMenuWin == w {
				app.jobMenuWin = nil
				app.jobMenuShown = false
			}
			app.mu.Unlock()
		},
	}, app.onJobMenuMessage)
	if err != nil {
		app.log("Could not open the job menu: " + err.Error())
		return
	}
	app.mu.Lock()
	app.jobMenuWin = w
	app.mu.Unlock()
}

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

	app.prepareJobMenu()
	app.mu.Lock()
	menu, shown, lastKnob, hiddenAt := app.jobMenuWin, app.jobMenuShown, app.jobMenuKnob, app.jobMenuHiddenAt
	app.mu.Unlock()
	if menu == nil {
		return
	}
	// Clicking the knob whose menu is open closes it, as a dropdown's own button does. The
	// click usually hid it already, by taking focus from it just before landing here.
	if shown {
		menu.Hide()
		if lastKnob == msg.Knob {
			return
		}
	} else if lastKnob == msg.Knob && time.Since(hiddenAt) < 300*time.Millisecond {
		return
	}

	app.mu.Lock()
	app.jobMenuKnob = msg.Knob
	app.jobMenuAnchor = msg.Anchor
	app.mu.Unlock()
	// The page draws the list, then answers menuReady with its height, and only then does
	// the popup show, so it never flashes the previous knob's list.
	menu.Send(map[string]any{"type": "menu", "knob": msg.Knob, "model": msg.Model})
}

func (app *App) onJobMenuMessage(data []byte) {
	var msg struct {
		Type    string          `json:"type"`
		Job     json.RawMessage `json:"job"`
		Checked bool            `json:"checked"`
		Height  float64         `json:"height"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	app.mu.Lock()
	menu, settings := app.jobMenuWin, app.settingsWin
	knob, anchor := app.jobMenuKnob, app.jobMenuAnchor
	app.mu.Unlock()
	if menu == nil {
		return
	}

	switch msg.Type {
	case "ready":
		menu.Send(app.baseInitFields())
	case "menuReady":
		app.mu.Lock()
		app.jobMenuShown = true
		app.mu.Unlock()
		menu.ShowAt(anchor, int32(math.Ceil(msg.Height)))
	case "toggle":
		if settings != nil {
			settings.Send(map[string]any{"type": "jobMenuToggle", "knob": knob, "job": msg.Job, "checked": msg.Checked})
		}
	case "clear":
		if settings != nil {
			settings.Send(map[string]any{"type": "jobMenuClear", "knob": knob})
		}
	case "pickApp":
		menu.Hide()
		app.loop.Invoke(func() { app.handleSettingsPickApp(data) })
	case "close":
		menu.Hide()
	}
}
