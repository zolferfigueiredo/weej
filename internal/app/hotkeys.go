//go:build windows

package app

import "github.com/zolferfigueiredo/weej/internal/core"

// registerHotkeys registers each key combination the boards use once, by its place in
// app.bindings, so a shortcut several boards share switches all of them.
func (app *App) registerHotkeys(s core.Settings) {
	hk := app.loop.Hotkeys()
	bindings := core.HotkeyBindings(s.Devices)
	app.loop.Invoke(func() {
		hk.UnregisterAll()
		app.mu.Lock()
		app.bindings = bindings
		app.mu.Unlock()
		for i, b := range bindings {
			hk.Register(i, b.Shortcut.Mods, b.Shortcut.VK)
		}
	})
}

func (app *App) onHotkey(id int) {
	app.mu.Lock()
	var targets []core.HotkeyTarget
	if id >= 0 && id < len(app.bindings) {
		targets = app.bindings[id].Targets
	}
	app.mu.Unlock()
	app.setProfiles(targets)
}
