//go:build windows

package app

import "github.com/zolferfigueiredo/weej/internal/core"

const (
	nextProfileHotkeyID     = 0xBFFF
	previousProfileHotkeyID = 0xBFFE
)

func (app *App) registerHotkeys(setup core.Setup) {
	hk := app.loop.Hotkeys()
	app.loop.Invoke(func() {
		hk.UnregisterAll()
		for i, p := range setup.Profiles {
			if p.Shortcut != nil {
				hk.Register(i, p.Shortcut.Mods, p.Shortcut.VK)
			}
		}
		if setup.Next != nil {
			hk.Register(nextProfileHotkeyID, setup.Next.Mods, setup.Next.VK)
		}
		if setup.Previous != nil {
			hk.Register(previousProfileHotkeyID, setup.Previous.Mods, setup.Previous.VK)
		}
	})
}

func (app *App) onHotkey(id int) {
	s := app.snapshotSettings()
	switch id {
	case nextProfileHotkeyID:
		s.Active = s.Stepped(1)
	case previousProfileHotkeyID:
		s.Active = s.Stepped(-1)
	default:
		if id < 0 || id >= len(s.Profiles) {
			return
		}
		s.Active = id
	}
	app.unmuteAll()
	if err := app.persistSettings(s); err != nil {
		app.log("Could not save settings: " + err.Error())
	}
	app.refreshTray()
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "profile", "profile": s.Active})
	}
	app.showProfileHUD(s)
}
