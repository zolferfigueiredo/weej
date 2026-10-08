//go:build windows

package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/lang"
	"github.com/zolferfigueiredo/weej/internal/platform/audio"
	"github.com/zolferfigueiredo/weej/internal/platform/display"
	"github.com/zolferfigueiredo/weej/internal/platform/nightlight"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
	"github.com/zolferfigueiredo/weej/internal/platform/via"
	"github.com/zolferfigueiredo/weej/internal/platform/zoom"
	"github.com/zolferfigueiredo/weej/internal/ui/web"
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
)

func (app *App) setup(l *winui.Loop) {
	app.loop = l
	web.SetLogger(func(msg string) { app.log(msg) })

	webVersion, webOK := web.Available()
	if !webOK {
		app.log("WebView2 is not available; Settings and the other windows cannot open")
	} else {
		app.log("WebView2 available: " + webVersion)
	}

	data, _ := os.ReadFile(sys.SettingsPath())
	code := app.resolveStartupLanguage(data, webOK)
	settings := core.DecodeSettings(data, lang.T(code, "default_profile", nil))
	if !lang.Valid(settings.Language) {
		settings.Language = code
	}
	app.replaceSettings(settings)
	if err := app.persistSettings(settings); err != nil {
		app.log("Could not save settings: " + err.Error())
	}

	app.printStartupSummary()

	audioW, err := audio.Start(app.log)
	if err != nil {
		app.log("Could not start the audio worker: " + err.Error())
	}
	app.audio = audioW
	app.ddc = display.NewDDC(app.log)
	app.via = via.NewVIA(app.log)
	app.zoom = zoom.New(app.log)
	app.nightlight = nightlight.New(app.log)

	app.setupTray(l)
	app.checkUpdateComplete()

	l.OnMessage("OpenSettings", func() { app.openSettings("") })
	l.OnMessage("Quit", app.quit)
	l.OnEndSession(app.quit)
	l.Hotkeys().OnHotkey(app.onHotkey)
	app.registerHotkeys(settings)
	app.syncRunners(settings.Devices)
	app.applyLights(settings.Devices)
	// A first start has no boards yet, so Settings opens on setting them up.
	if len(settings.Devices) == 0 && webOK {
		l.Invoke(func() { app.openSettings("general") })
	}

	app.setupAutoUpdateChecks()

	if app.testNotifications {
		app.postTestNotification()
	}
}

func (app *App) resolveStartupLanguage(data []byte, webOK bool) string {
	if code, ok := settingsFileLanguage(data); ok {
		return code
	}
	detected := lang.Detect(sys.PreferredUILanguages())
	if webOK {
		return app.showLanguagePrompt(detected)
	}
	app.log("Using the detected language " + detected + " (no WebView2 for the language prompt)")
	return detected
}

func settingsFileLanguage(data []byte) (string, bool) {
	if len(data) == 0 {
		return "", false
	}
	var probe struct {
		Language string `json:"language"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return "", false
	}
	if !lang.Valid(probe.Language) {
		return "", false
	}
	return probe.Language, true
}

func (app *App) printStartupSummary() {
	tr := app.trFunc()
	for _, d := range app.snapshotSettings().Devices {
		state := "on"
		if !d.Enabled {
			state = "off"
		}
		port := d.Port
		if port == "" {
			port = "automatic"
		}
		p := d.ActiveProfile()
		app.log(fmt.Sprintf("board %s (%s, %s, %s), profile %s", d.Name, d.Type, port, state, p.Name))
		for i, c := range d.Controls {
			if c.Kind == core.KindButton {
				continue
			}
			var titles []string
			if i < len(p.Jobs) {
				for _, job := range p.Jobs[i] {
					titles = append(titles, job.Title(tr, app.appDisplayName))
				}
			}
			if len(titles) == 0 {
				titles = []string{app.tr("job.empty")}
			}
			app.log(fmt.Sprintf("  %s %s, input %d: %s", c.Kind, core.Letter(i), c.Input, strings.Join(titles, ", ")))
		}
	}
}

func (app *App) shutdown() {
	app.shutdownOnce.Do(func() {
		if app.zoom != nil {
			app.zoom.Off()
		}
		app.endWizard(false)
		app.mu.Lock()
		loopback := app.loopback
		app.loopback = nil
		app.mu.Unlock()
		if loopback != nil {
			loopback.Stop()
		}
		app.stopRunners()
		if app.audio != nil {
			app.audio.Close()
		}
		if app.ddc != nil {
			app.ddc.Close()
		}
		if app.via != nil {
			app.via.Close()
		}
	})
}
