//go:build windows

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/lang"
	"github.com/zolferfigueiredo/weej/internal/platform/audio"
	"github.com/zolferfigueiredo/weej/internal/platform/display"
	"github.com/zolferfigueiredo/weej/internal/platform/midiport"
	"github.com/zolferfigueiredo/weej/internal/platform/nightlight"
	"github.com/zolferfigueiredo/weej/internal/platform/serialport"
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
	settings, _ := core.DecodeSettings(data, lang.T(code, "default_profile", nil))
	if !lang.Valid(settings.Language) {
		settings.Language = code
	}
	app.replaceSettings(settings)
	if err := app.persistSettings(settings); err != nil {
		app.log("Could not save settings: " + err.Error())
	}

	app.printStartupSummary()

	app.engine = core.NewEngine(&applier{app: app})

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
	app.registerHotkeys(app.snapshotSettings().Setup)

	app.startSerial()

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
	s := app.snapshotSettings()
	if s.Active < 0 || s.Active >= len(s.Profiles) {
		return
	}
	if app.usesMixer() {
		s.Setup = s.ForMixer()
	}
	active := s.Profiles[s.Active]
	tr := app.trFunc()
	app.log("profile " + active.Name)
	for i, col := range s.Columns {
		letter := core.Letter(i)
		if col < 0 {
			app.log(fmt.Sprintf("knob %s, not calibrated: %s", letter, app.tr("job.empty")))
			continue
		}
		jobs := active.JobsOf(i)
		if len(jobs) == 0 {
			app.log(fmt.Sprintf("knob %s, input %d: %s", letter, col, app.tr("job.empty")))
			continue
		}
		titles := make([]string, len(jobs))
		for j, job := range jobs {
			titles[j] = job.Title(tr, app.appDisplayName)
		}
		app.log(fmt.Sprintf("knob %s, input %d: %s", letter, col, strings.Join(titles, ", ")))
	}
}

// startSerial (re)starts the input loop, serial or MIDI, with the saved port and speed. A port
// given on the command line wins over the saved one. The previous loop has closed its port
// before the new one opens it, or the board would look busy for a moment.
func (app *App) startSerial() {
	s := app.snapshotSettings()
	port := app.sourcePort()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	app.mu.Lock()
	prevCancel, prevDone := app.cancelSerial, app.serialDone
	app.cancelSerial, app.serialDone = cancel, done
	app.mu.Unlock()

	go func() {
		defer close(done)
		if prevCancel != nil {
			prevCancel()
			<-prevDone
			app.onSerialStatus(serialport.Status{})
		}
		if core.IsMidiPort(port) {
			device := core.MidiDevice(port)
			midiport.Run(ctx, midiport.Config{
				Device:   device,
				OnValues: app.onMixerValues,
				OnButton: app.onMixerButton,
				OnStatus: func(connected, busy bool) {
					app.onSerialStatus(serialport.Status{Connected: connected, Busy: busy, Port: device})
				},
				Log: app.log,
			}, app.reconnectCh)
			return
		}
		serialport.Run(ctx, serialport.Config{
			ForcedPort: port,
			Baud:       s.BaudRate(),
			OnLine:     app.onSerialLine,
			OnStatus:   app.onSerialStatus,
			Log:        app.log,
		}, app.reconnectCh)
	}()
}

func (app *App) onSerialLine(values []int) {
	app.handleValues(values, app.snapshotSettings().Setup)
}

func (app *App) handleValues(values []int, setup core.Setup) {
	calibrating := app.isCalibrating()
	if calibrating {
		app.feedCalibrator(values, time.Since(app.startTime).Seconds())
	}
	app.engine.Handle(values, setup, calibrating)
	if !calibrating {
		app.pointOutMovedKnobs(values, setup)
	}
	app.updateTerminalLine(values, setup, calibrating)
}

func (app *App) onSerialStatus(status serialport.Status) {
	app.loop.Invoke(func() {
		app.setConnection(status.Connected, status.Busy, status.Port)
		app.refreshTrayNow()
		app.pushConnection()
		if !status.Connected {
			app.engine.Reset()
			return
		}
		app.mu.Lock()
		first := !app.firstConnectDone
		app.firstConnectDone = true
		app.mu.Unlock()
		if first {
			app.calibrateIfNeeded()
		}
	})
}

func (app *App) calibrateIfNeeded() {
	cols := app.activeColumns(app.snapshotSettings().Setup)
	if len(cols) == 0 || hasUncalibratedColumn(cols) {
		app.startCalibration(true)
	}
}

func (app *App) updateTerminalLine(values []int, setup core.Setup, calibrating bool) {
	if calibrating || !sys.StdoutIsTerminal() {
		return
	}
	now := time.Now()
	if now.Sub(app.lastTerminalLine) < 500*time.Millisecond {
		return
	}
	app.lastTerminalLine = now

	mapping := setup.Mapping()
	assigned := map[int]bool{}
	for _, c := range setup.Columns {
		assigned[c] = true
	}

	segs := make([]string, 0, len(values))
	for i, raw := range values {
		if raw < 0 {
			continue
		}
		marker := byte(' ')
		if assigned[i] {
			marker = '|'
			if jobs, ok := mapping[i]; ok && len(jobs) > 0 {
				marker = '*'
			}
		}
		segs = append(segs, fmt.Sprintf("%c%d:%4d", marker, i, raw))
	}

	tr := app.trFunc()
	lines := app.engine.Lines()
	jobTexts := make([]string, len(lines))
	for i, ln := range lines {
		jobTexts[i] = fmt.Sprintf("%s %d%%", ln.Job.Title(tr, app.appDisplayName), ln.Percent)
	}

	fmt.Print("\r" + strings.Join(segs, " ") + "   " + strings.Join(jobTexts, "  "))
}

func (app *App) shutdown() {
	app.shutdownOnce.Do(func() {
		if app.zoom != nil {
			app.zoom.Off()
		}
		app.mu.Lock()
		cancel := app.cancelSerial
		app.mu.Unlock()
		if cancel != nil {
			cancel()
		}
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
