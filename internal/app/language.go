//go:build windows

package app

import (
	"encoding/json"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/lang"
	"github.com/zolferfigueiredo/weej/internal/ui/web"
)

func (app *App) showLanguagePrompt(preselect string) string {
	chosen := preselect
	var win *web.Window

	onMessage := func(data []byte) {
		var probe struct {
			Type string `json:"type"`
			Code string `json:"code"`
		}
		if err := json.Unmarshal(data, &probe); err != nil {
			return
		}
		switch probe.Type {
		case "ready":
			if win == nil {
				return
			}
			win.Send(map[string]any{
				"type":      "init",
				"languages": languagesPayload(),
				"selected":  preselect,
				"icon":      appIconDataURL(64),
			})
		case "preview":
			if !lang.Valid(probe.Code) || win == nil {
				return
			}
			win.Send(map[string]any{"type": "strings", "lang": probe.Code, "strings": lang.Catalog(probe.Code)})
		case "continue":
			if lang.Valid(probe.Code) {
				chosen = probe.Code
			}
			if win != nil {
				win.Close()
			}
		}
	}

	_, err := web.Open(app.loop.Invoke, "language", web.Options{
		Title: core.AppName, Width: 420, Height: 280, Modal: true,
		OnOpen: func(w *web.Window) { win = w },
	}, onMessage)
	if err != nil {
		app.log("Could not open the language prompt: " + err.Error())
	}
	return chosen
}
