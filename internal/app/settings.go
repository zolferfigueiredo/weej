//go:build windows

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/lang"
	"github.com/zolferfigueiredo/weej/internal/platform/midiport"
	"github.com/zolferfigueiredo/weej/internal/platform/serialport"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
	"github.com/zolferfigueiredo/weej/internal/ui/web"
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
)

const (
	websiteURL = "https://weej.zolfer.com"
	madeByURL  = "https://zolfer.com"
	deejURL    = "https://github.com/omriharel/deej"
	theejURL   = "https://theej.zolfer.com"
)

func (app *App) openSettings(tab string) {
	app.mu.Lock()
	win := app.settingsWin
	app.mu.Unlock()
	if win != nil {
		win.Focus()
		if tab != "" {
			win.Send(map[string]any{"type": "tab", "tab": tab})
		}
		return
	}
	if tab == "" {
		tab = "general"
	}
	app.mu.Lock()
	app.settingsPendingTab = tab
	app.mu.Unlock()

	w, err := web.Open(app.loop.Invoke, "settings", web.Options{
		Title: app.tr("settings"), Width: 1000, Height: 800, Resizable: true,
		OnClose: func() {
			app.mu.Lock()
			app.settingsWin = nil
			app.mu.Unlock()
			// Closing mid-recording would otherwise leave the saved hotkeys switched off.
			app.registerHotkeys(app.snapshotSettings())
		},
	}, app.onSettingsMessage)
	if err != nil {
		app.log("Could not open Settings: " + err.Error())
		return
	}
	app.mu.Lock()
	app.settingsWin = w
	app.mu.Unlock()
}

func (app *App) closeSettings() {
	if win := app.settingsWin; win != nil {
		win.Close()
	}
}

// messageProbe is what every page message has: its type, and the board it is about. setDevice
// sends the whole board under "device" rather than its id, so a string field there would fail
// to decode and drop the message.
type messageProbe struct {
	Type   string
	Device string
}

func probeMessage(data []byte) (messageProbe, bool) {
	var raw struct {
		Type   string          `json:"type"`
		Device json.RawMessage `json:"device"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return messageProbe{}, false
	}
	probe := messageProbe{Type: raw.Type}
	_ = json.Unmarshal(raw.Device, &probe.Device)
	return probe, true
}

func (app *App) onSettingsMessage(data []byte) {
	probe, ok := probeMessage(data)
	if !ok {
		return
	}
	// Page messages arrive inside a WebView2 callback, so anything that opens a window or a
	// modal dialog is deferred to the main loop instead of nesting a message loop here.
	switch probe.Type {
	case "ready":
		app.sendSettingsInit()
		app.loop.Invoke(app.prepareJobMenu)
	case "save":
		app.handleSettingsSave(data)
	case "setDevice":
		app.handleSetDevice(data)
	case "addDevice":
		app.handleAddDevice(data)
	case "removeDevice":
		app.handleRemoveDevice(probe.Device)
	case "calibrate":
		var msg struct {
			Controls []int `json:"controls"`
		}
		_ = json.Unmarshal(data, &msg)
		app.loop.Invoke(func() { app.startWizard(probe.Device, msg.Controls) })
	case "wizard":
		var msg struct {
			Op string `json:"op"`
		}
		_ = json.Unmarshal(data, &msg)
		app.loop.Invoke(func() { app.wizardOp(msg.Op) })
	case "openJobMenu":
		app.loop.Invoke(func() { app.openJobMenu(data) })
	case "pickApp":
		app.loop.Invoke(func() { app.handleSettingsPickApp(data) })
	case "importProfile":
		app.loop.Invoke(func() { app.handleImportProfile(probe.Device) })
	case "exportProfile":
		app.loop.Invoke(func() { app.handleExportProfile(probe.Device, data) })
	case "setLanguage":
		app.handleSettingsSetLanguage(data)
	case "record":
		app.handleSettingsRecord()
	case "key":
		app.handleSettingsKey(data)
	case "stopRecording":
		app.handleSettingsStopRecording()
	case "listPorts":
		app.sendPorts()
	case "reconnect":
		app.requestReconnect(probe.Device)
	case "checkUpdates":
		app.manualCheckUpdate()
	case "openUrl":
		app.handleSettingsOpenURL(data)
	case "close":
		app.closeSettings()
	}
}

// settingsJSON is the settings as the page edits them: the file's own shape.
func settingsJSON(s core.Settings) json.RawMessage {
	data, err := core.EncodeSettings(s)
	if err != nil {
		return json.RawMessage("null")
	}
	return data
}

func deviceJSON(d core.Device) json.RawMessage {
	data, err := core.EncodeDevice(d)
	if err != nil {
		return json.RawMessage("null")
	}
	return data
}

func (app *App) sendSettingsInit() {
	win := app.settingsWin
	if win == nil {
		return
	}
	s := app.snapshotSettings()
	app.mu.Lock()
	tab := app.settingsPendingTab
	app.mu.Unlock()
	if tab == "" {
		tab = "general"
	}
	values := map[string][]int{}
	status := map[string]any{}
	for _, d := range s.Devices {
		status[d.ID] = app.statusPayload(d.ID)
		if r := app.runnerFor(d.ID); r != nil {
			values[d.ID] = r.live.frame()
		}
	}

	payload := app.baseInitFields()
	payload["tab"] = tab
	payload["version"] = core.AppVersion
	payload["website"] = websiteURL
	payload["madeBy"] = madeByURL
	payload["deej"] = deejURL
	payload["theej"] = theejURL
	payload["icon"] = appIconDataURL(64)
	payload["languages"] = languagesPayload()
	payload["language"] = s.Language
	payload["settings"] = settingsJSON(s)
	payload["catalog"] = app.buildCatalog(s.Devices)
	payload["actionIcons"] = app.actionIcons()
	payload["labels"] = app.shortcutLabels(s)
	payload["nightLightExperimental"] = true
	payload["status"] = status
	payload["values"] = values
	payload["wizard"] = app.wizardPayload()
	payload["forcedPort"] = app.forcedPort
	payload["baudRates"] = core.BaudRates
	payload["smcButtons"] = core.SMCButtonOrder()
	payload["lightPatterns"] = core.LightPatterns
	payload["ctrlName"] = app.ctrlLabelName()
	win.Send(payload)
}

// applySettings saves new settings and makes everything follow them: the boards that run, their
// lights, the shortcuts and the tray. A board whose profile changed gets its mutes back first.
func (app *App) applySettings(next core.Settings) {
	cur := app.snapshotSettings()
	for _, d := range cur.Devices {
		if i := deviceIndex(next, d.ID); i < 0 || next.Devices[i].Active != d.Active {
			app.unmuteDevice(d)
		}
	}
	for i := range next.Devices {
		d := &next.Devices[i]
		for j := range d.Profiles {
			if strings.TrimSpace(d.Profiles[j].Name) == "" {
				d.Profiles[j].Name = app.trVars("profile_n", v1("n", strconv.Itoa(j+1)))
			}
		}
		if strings.TrimSpace(d.Name) == "" {
			d.Name = app.tr("device.type." + string(d.Type))
		}
	}
	if err := app.persistSettings(next); err != nil {
		app.log("Could not save settings: " + err.Error())
		return
	}
	app.syncRunners(next.Devices)
	app.applyLights(next.Devices)
	app.registerHotkeys(next)
	app.refreshTray()
}

func (app *App) profileName() string { return app.tr("default_profile") }

func (app *App) handleSettingsSave(data []byte) {
	var msg struct {
		Settings json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	page := core.DecodeSettings(msg.Settings, app.profileName())
	next := app.snapshotSettings()
	next.Devices = page.Devices
	next.HideIcon, next.ShowProfiles = page.HideIcon, page.ShowProfiles
	app.applySettings(next)
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "saved", "settings": settingsJSON(app.snapshotSettings())})
	}
}

// handleSetDevice saves one board at once, as the gear's dialog, the on and off switch, the
// Draw or List choice and calibrating do, leaving the rest of the page's edits for Apply.
func (app *App) handleSetDevice(data []byte) {
	var msg struct {
		Device json.RawMessage `json:"device"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	d, ok := core.DecodeDevice(msg.Device, app.profileName())
	if !ok {
		return
	}
	next := app.snapshotSettings()
	i := deviceIndex(next, d.ID)
	if i < 0 {
		return
	}
	next.Devices[i] = d
	app.applySettings(next)
	app.sendDevice(app.snapshotSettings().Devices[i])
}

func (app *App) sendDevice(d core.Device) {
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "deviceSaved", "device": deviceJSON(d)})
	}
}

func (app *App) handleAddDevice(data []byte) {
	var msg struct {
		Name     string `json:"name"`
		Type     string `json:"deviceType"`
		Port     string `json:"port"`
		BaudRate int    `json:"baudRate"`
		Knobs    int    `json:"knobs"`
		Faders   int    `json:"faders"`
		Buttons  int    `json:"buttons"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	t, ok := core.ParseDeviceType(msg.Type)
	if !ok {
		return
	}
	clamp := func(n int) int { return min(max(n, 0), 64) }
	next := app.snapshotSettings()
	d := core.NewDevice(core.NextDeviceID(next.Added), strings.TrimSpace(msg.Name), t, clamp(msg.Knobs), clamp(msg.Faders), clamp(msg.Buttons), app.profileName())
	d.Port, d.Baud = msg.Port, msg.BaudRate
	next.Added++
	next.Devices = append(next.Devices, d)
	app.applySettings(next)
	saved := app.snapshotSettings()
	if i := deviceIndex(saved, d.ID); i >= 0 {
		app.sendDevice(saved.Devices[i])
	}
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "added", "device": d.ID})
	}
}

func (app *App) handleRemoveDevice(id string) {
	next := app.snapshotSettings()
	i := deviceIndex(next, id)
	if i < 0 {
		return
	}
	if w := app.wizardFor(id); w != nil {
		app.endWizard(false)
	}
	next.Devices = append(next.Devices[:i:i], next.Devices[i+1:]...)
	app.applySettings(next)
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "deviceRemoved", "device": id})
	}
}

func (app *App) handleSettingsPickApp(data []byte) {
	var msg struct {
		Knob int `json:"knob"`
		// Button is set when a button's Open or Close an app asks; Mode says which.
		Button *int   `json:"button"`
		Mode   string `json:"mode"`
	}
	_ = json.Unmarshal(data, &msg)

	programFiles := sys.ExpandEnv("%ProgramFiles%")
	path, ok := app.loop.OpenFile(app.tr("choose"), [][2]string{{"Programs (*.exe)", "*.exe"}}, programFiles)
	if !ok {
		return
	}
	exe := core.ExeName(path)
	if exe == core.ExeName(app.self) {
		return
	}

	app.forgetApp(exe)
	app.appCacheMu.Lock()
	app.appPath[exe] = path
	app.appCacheMu.Unlock()

	name := app.resolveApp(exe)
	if msg.Button != nil {
		if win := app.settingsWin; win != nil {
			win.Send(map[string]any{"type": "buttonAppPicked", "button": *msg.Button, "mode": msg.Mode, "path": path, "exe": exe, "name": name})
		}
		return
	}
	job := core.Job{Kind: core.JobApp, Exe: exe}
	entry := catalogEntry{
		Section: sectionName(job.Section()),
		Job:     job,
		Title:   name,
		Short:   name,
		Icon:    app.appIconDataURLFor(exe),
	}
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "appPicked", "knob": msg.Knob, "entry": entry})
	}
}

// handleImportProfile reads a file Export wrote, or a deej config, as a new profile of a board.
// Settings adds it to the board as an edit, which Apply saves.
func (app *App) handleImportProfile(id string) {
	d, ok := app.device(id)
	if !ok {
		return
	}
	initialDir := ""
	if p := runningProcessPath("deej.exe"); p != "" {
		initialDir = filepath.Dir(p)
	}
	title := strings.TrimSuffix(app.tr("profile.import"), "…")
	path, ok := app.loop.OpenFile(title, [][2]string{{"WeeJ, deej", "*.json;*.yaml;*.yml"}}, initialDir)
	if !ok {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		app.sendImportFailed()
		return
	}
	p, ok := core.DecodeProfileFile(data, d)
	var skipped []string
	if !ok {
		imp, err := core.ImportDeej(data)
		if err != nil {
			app.sendImportFailed()
			return
		}
		p, skipped = core.DeejProfile(imp, d)
	}
	profile, err := core.EncodeProfile(p)
	if err != nil {
		app.sendImportFailed()
		return
	}
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "profileImported", "device": id, "profile": json.RawMessage(profile), "skipped": skipped})
	}
}

// handleExportProfile saves the profile Settings shows for a board to a file Import reads back.
func (app *App) handleExportProfile(id string, data []byte) {
	d, ok := app.device(id)
	if !ok {
		return
	}
	var msg struct {
		Profile json.RawMessage `json:"profile"`
	}
	if json.Unmarshal(data, &msg) != nil {
		return
	}
	p, ok := core.DecodeProfile(msg.Profile, d)
	if !ok {
		return
	}
	out, err := core.EncodeProfileFile(d.Type, p)
	if err != nil {
		return
	}
	title := strings.TrimSuffix(app.tr("profile.export"), "…")
	name := fileNameSafe.ReplaceAllString(d.Name+" - "+p.Name, "_") + ".json"
	path, ok := app.loop.SaveFile(title, [][2]string{{"WeeJ", "*.json"}}, name, "json")
	if !ok {
		return
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		app.log("Could not export the profile: " + err.Error())
		if win := app.settingsWin; win != nil {
			win.Send(map[string]any{"type": "exportFailed"})
		}
	}
}

// fileNameSafe matches what Windows refuses in a file name.
var fileNameSafe = regexp.MustCompile(`[\\/:*?"<>|]`)

func (app *App) sendImportFailed() {
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "importFailed"})
	}
}

func (app *App) handleSettingsSetLanguage(data []byte) {
	var msg struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(data, &msg)
	app.setLanguage(msg.Code)
}

func (app *App) setLanguage(code string) {
	if !lang.Valid(code) {
		return
	}
	s := app.snapshotSettings()
	s.Language = code
	if err := app.persistSettings(s); err != nil {
		app.log("Could not save settings: " + err.Error())
	}

	payload := map[string]any{"type": "strings", "lang": code, "strings": lang.Catalog(code)}
	for _, win := range app.openWebWindows() {
		win.Send(payload)
	}
	app.refreshTray()
	app.retitleWebWindows()
}

func (app *App) openWebWindows() []*web.Window {
	app.mu.Lock()
	defer app.mu.Unlock()
	var out []*web.Window
	for _, w := range []*web.Window{app.settingsWin, app.jobMenuWin, app.updateWin} {
		if w != nil {
			out = append(out, w)
		}
	}
	return out
}

func (app *App) retitleWebWindows() {
	if w := app.settingsWin; w != nil {
		w.SetTitle(app.tr("settings"))
	}
}

func (app *App) handleSettingsRecord() {
	app.loop.Hotkeys().UnregisterAll()
}

func (app *App) handleSettingsStopRecording() {
	app.registerHotkeys(app.snapshotSettings())
}

// recordDraft is one board's shortcuts as the page has them: a shortcut only clashes on its own
// board, since the same one may switch several.
type recordDraft struct {
	Profiles []*core.Shortcut `json:"profiles"`
	Next     *core.Shortcut   `json:"next"`
	Previous *core.Shortcut   `json:"previous"`
}

type keyMsg struct {
	Field string      `json:"field"`
	VK    uint16      `json:"vk"`
	Key   string      `json:"key"`
	Ctrl  bool        `json:"ctrl"`
	Alt   bool        `json:"alt"`
	Shift bool        `json:"shift"`
	Meta  bool        `json:"meta"`
	Draft recordDraft `json:"draft"`
}

const shortcutProbeHotkeyID = 0xBFF0

func (app *App) handleSettingsKey(data []byte) {
	var msg keyMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	win := app.settingsWin
	if win == nil {
		return
	}

	noMods := !msg.Ctrl && !msg.Alt && !msg.Shift && !msg.Meta
	if noMods && (msg.Key == "Delete" || msg.Key == "Backspace") {
		win.Send(map[string]any{"type": "recorded", "field": msg.Field, "shortcut": nil, "label": nil})
		return
	}

	var mods uint16
	if msg.Ctrl {
		mods |= core.ModControl
	}
	if msg.Alt {
		mods |= core.ModAlt
	}
	if msg.Shift {
		mods |= core.ModShift
	}
	if msg.Meta {
		mods |= core.ModWin
	}
	shortcut := core.Shortcut{VK: msg.VK, Mods: mods, Key: msg.Key}

	// reason tells the page why: "invalid", "clash" with another of the board's shortcuts, or
	// "taken" by another app, which Windows lets hold a combination alone.
	reject := func(reason string) {
		winui.Beep()
		win.Send(map[string]any{"type": "rejected", "field": msg.Field, "reason": reason})
	}

	// A button presses its keys rather than listening for them, so any key will do and nothing
	// has to be free to register.
	if strings.HasPrefix(msg.Field, "button:") {
		win.Send(map[string]any{"type": "recorded", "field": msg.Field, "shortcut": shortcut, "label": core.Label(shortcut, app.ctrlLabelName())})
		return
	}

	if !core.Valid(shortcut) {
		reject("invalid")
		return
	}
	if app.clashesWithDraft(shortcut, msg.Field, msg.Draft) {
		reject("clash")
		return
	}

	hk := app.loop.Hotkeys()
	registered := hk.Register(shortcutProbeHotkeyID, shortcut.Mods, shortcut.VK)
	if registered {
		hk.Unregister(shortcutProbeHotkeyID)
	}
	if !registered {
		app.log("Shortcut " + core.Label(shortcut, "Ctrl") + " is in use by another app")
		reject("taken")
		return
	}

	label := core.Label(shortcut, app.ctrlLabelName())
	win.Send(map[string]any{"type": "recorded", "field": msg.Field, "shortcut": shortcut, "label": label})
}

func (app *App) clashesWithDraft(s core.Shortcut, field string, draft recordDraft) bool {
	clashesWith := func(otherField string, other *core.Shortcut) bool {
		if otherField == field {
			return false
		}
		return core.Clash(&s, other)
	}
	if clashesWith("next", draft.Next) {
		return true
	}
	if clashesWith("previous", draft.Previous) {
		return true
	}
	for i, p := range draft.Profiles {
		if clashesWith("profile:"+strconv.Itoa(i), p) {
			return true
		}
	}
	return false
}

func (app *App) handleSettingsOpenURL(data []byte) {
	var msg struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(data, &msg)
	for _, allowed := range []string{websiteURL, madeByURL, deejURL, theejURL} {
		if msg.URL == allowed {
			openURL(msg.URL)
			return
		}
	}
}

func languagesPayload() []map[string]string {
	out := make([]map[string]string, len(lang.Languages))
	for i, l := range lang.Languages {
		out[i] = map[string]string{"code": l.Code, "name": l.Name}
	}
	return out
}

func (app *App) statusPayload(id string) map[string]any {
	out := map[string]any{"connected": false, "busy": false, "port": ""}
	if r := app.runnerFor(id); r != nil {
		connected, busy, port := r.status()
		out["connected"], out["busy"], out["port"] = connected, busy, port
	}
	return out
}

func (app *App) pushStatus(id string) {
	if win := app.settingsWin; win != nil {
		payload := app.statusPayload(id)
		payload["type"] = "status"
		payload["device"] = id
		win.Send(payload)
	}
}

// Listing ports can take a moment, so it runs off the UI thread; Send is safe from anywhere.
func (app *App) sendPorts() {
	win := app.settingsWin
	if win == nil {
		return
	}
	go func() {
		ports := serialport.List()
		if ports == nil {
			ports = []serialport.PortInfo{}
		}
		win.Send(map[string]any{"type": "ports", "ports": ports, "midi": midiport.List()})
	}()
}
