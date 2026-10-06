//go:build windows

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
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

type setupJSON struct {
	Columns          []*int         `json:"columns"`
	Profiles         []core.Profile `json:"profiles"`
	Profile          int            `json:"profile"`
	NextProfile      *core.Shortcut `json:"nextProfile"`
	PreviousProfile  *core.Shortcut `json:"previousProfile"`
	InvertKnobs      bool           `json:"invertKnobs"`
	HideTrayIcon     bool           `json:"hideTrayIcon"`
	ShowProfileList  bool           `json:"showProfileList"`
	TrayIcon         string         `json:"trayIcon"`
	Speed            string         `json:"speed"`
	Port             string         `json:"port"`
	BaudRate         int            `json:"baudRate"`
	MixerColumns     []*int         `json:"mixerColumns"`
	MixerButtonOrder []int          `json:"mixerButtonOrder"`
	Language         string         `json:"language"`
}

func columnsToJSON(cols []int) []*int {
	out := make([]*int, len(cols))
	for i, c := range cols {
		if c == -1 {
			continue
		}
		v := c
		out[i] = &v
	}
	return out
}

func columnsFromJSON(ptrs []*int) []int {
	out := make([]int, len(ptrs))
	for i, p := range ptrs {
		if p == nil {
			out[i] = -1
		} else {
			out[i] = *p
		}
	}
	return out
}

func mixerColumnsFromJSON(ptrs []*int) []int {
	if ptrs == nil {
		return nil
	}
	return columnsFromJSON(ptrs)
}

func setupToJSON(s core.Setup) setupJSON {
	columns := s.Columns
	if columns == nil {
		columns = []int{}
	}
	// One jobs row per knob, for the board's knobs and the mixer's: the page adds and removes
	// knobs by index.
	knobs := max(len(columns), len(s.ForMixer().Columns))
	profiles := make([]core.Profile, len(s.Profiles))
	for i, p := range s.Profiles {
		rows := max(len(p.Jobs), knobs)
		jobs := make([][]core.Job, rows)
		for j := range jobs {
			jobs[j] = []core.Job{}
			if j < len(p.Jobs) && p.Jobs[j] != nil {
				jobs[j] = p.Jobs[j]
			}
		}
		p.Jobs = jobs
		profiles[i] = p
	}
	return setupJSON{
		Columns:          columnsToJSON(columns),
		Profiles:         profiles,
		Profile:          s.Active,
		NextProfile:      s.Next,
		PreviousProfile:  s.Previous,
		InvertKnobs:      s.Invert,
		HideTrayIcon:     s.HideIcon,
		ShowProfileList:  s.ShowProfiles,
		TrayIcon:         string(s.Icon),
		Speed:            string(s.Speed),
		Port:             s.Port,
		BaudRate:         s.BaudRate(),
		MixerColumns:     core.EncodeMixerColumns(s.MixerColumns),
		MixerButtonOrder: s.ButtonOrder,
	}
}

func setupToJSONWithLanguage(s core.Settings) setupJSON {
	j := setupToJSON(s.Setup)
	j.Language = s.Language
	return j
}

func setupFromJSON(j setupJSON) core.Setup {
	return core.Setup{
		Columns:      columnsFromJSON(j.Columns),
		Profiles:     j.Profiles,
		Active:       j.Profile,
		Next:         j.NextProfile,
		Previous:     j.PreviousProfile,
		Invert:       j.InvertKnobs,
		HideIcon:     j.HideTrayIcon,
		ShowProfiles: j.ShowProfileList,
		Icon:         core.ParseIconStyle(j.TrayIcon),
		Speed:        core.ParseSpeed(j.Speed),
		Port:         j.Port,
		Baud:         j.BaudRate,
		MixerColumns: mixerColumnsFromJSON(j.MixerColumns),
		ButtonOrder:  j.MixerButtonOrder,
	}
}

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
		Title: app.tr("settings"), Width: 900, Height: 560,
		OnClose: func() {
			app.mu.Lock()
			app.settingsWin = nil
			app.mu.Unlock()
			// Closing mid-recording would otherwise leave the saved hotkeys switched off.
			app.registerHotkeys(app.snapshotSettings().Setup)
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

func (app *App) onSettingsMessage(data []byte) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
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
	case "calibrate":
		app.loop.Invoke(func() { app.startCalibration(false) })
	case "openJobMenu":
		app.loop.Invoke(func() { app.openJobMenu(data) })
	case "pickApp":
		app.loop.Invoke(func() { app.handleSettingsPickApp(data) })
	case "importDeej":
		app.loop.Invoke(app.handleSettingsImportDeej)
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
		app.requestReconnect()
	case "checkUpdates":
		app.manualCheckUpdate()
	case "openUrl":
		app.handleSettingsOpenURL(data)
	case "close":
		app.closeSettings()
	}
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
	payload["setup"] = setupToJSONWithLanguage(s)
	payload["catalog"] = app.buildCatalog(s.Setup)
	payload["iconPreviews"] = app.iconPreviews()
	payload["labels"] = app.shortcutLabels(s.Setup)
	payload["nightLightExperimental"] = true
	payload["connection"] = app.connectionPayload()
	payload["calibrating"] = app.isCalibrating()
	payload["forcedPort"] = app.forcedPort
	payload["baudRates"] = core.BaudRates
	payload["mixerButtonDefaults"] = core.DefaultMixerButtonOrder
	win.Send(payload)
}

func (app *App) handleSettingsSave(data []byte) {
	var msg struct {
		Setup setupJSON `json:"setup"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	newSetup := setupFromJSON(msg.Setup)
	if len(newSetup.Profiles) == 0 {
		app.log("Ignored a Settings save with no profiles")
		return
	}
	if newSetup.Active < 0 || newSetup.Active >= len(newSetup.Profiles) {
		newSetup.Active = 0
	}

	for i := range newSetup.Profiles {
		if strings.TrimSpace(newSetup.Profiles[i].Name) == "" {
			newSetup.Profiles[i].Name = app.trVars("profile_n", v1("n", strconv.Itoa(i+1)))
		}
	}

	cur := app.snapshotSettings()
	old := cur.Setup
	if old.Active != newSetup.Active {
		app.unmuteAll()
	}
	cur.Setup = newSetup
	if err := app.persistSettings(cur); err != nil {
		app.log("Could not save settings: " + err.Error())
	}
	if old.Port != newSetup.Port || old.BaudRate() != newSetup.BaudRate() {
		app.startSerial()
	}

	app.registerHotkeys(cur.Setup)
	app.refreshTray()

	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "saved", "setup": setupToJSONWithLanguage(cur)})
	}

	if hasUncalibratedColumn(app.activeColumns(cur.Setup)) {
		app.loop.Invoke(func() { app.startCalibration(true) })
	}
}

func hasUncalibratedColumn(cols []int) bool {
	for _, c := range cols {
		if c == -1 {
			return true
		}
	}
	return false
}

func (app *App) handleSettingsPickApp(data []byte) {
	var msg struct {
		Knob int `json:"knob"`
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

func (app *App) handleSettingsImportDeej() {
	initialDir := `C:\deej`
	if p := runningProcessPath("deej.exe"); p != "" {
		initialDir = filepath.Dir(p)
	}
	path, ok := app.loop.OpenFile(app.tr("import_deej"), [][2]string{{"deej config (*.yaml)", "*.yaml"}}, initialDir)
	if !ok {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		app.sendImportFailed()
		return
	}
	imp, err := core.ImportDeej(data)
	if err != nil {
		app.sendImportFailed()
		return
	}

	cur := app.snapshotSettings()
	cur.Profiles = append(cur.Profiles, core.Profile{Name: imp.Name, Jobs: imp.Jobs})
	cur.Columns = imp.Columns
	cur.Invert = imp.Invert
	if imp.Baud > 0 {
		cur.Baud = imp.Baud
	}
	cur.Active = len(cur.Profiles) - 1

	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{
			"type":    "imported",
			"setup":   setupToJSONWithLanguage(cur),
			"skipped": imp.Skipped,
		})
	}
}

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
	for _, w := range []*web.Window{app.settingsWin, app.jobMenuWin, app.calibWin, app.updateWin} {
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
	if w := app.calibWin; w != nil {
		w.SetTitle(app.tr("calibration_title"))
	}
}

func (app *App) handleSettingsRecord() {
	app.loop.Hotkeys().UnregisterAll()
}

func (app *App) handleSettingsStopRecording() {
	app.registerHotkeys(app.snapshotSettings().Setup)
}

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

	reject := func() {
		winui.Beep()
		win.Send(map[string]any{"type": "rejected", "field": msg.Field})
	}

	if !core.Valid(shortcut) {
		reject()
		return
	}
	if app.clashesWithDraft(shortcut, msg.Field, msg.Draft) {
		reject()
		return
	}

	hk := app.loop.Hotkeys()
	registered := hk.Register(shortcutProbeHotkeyID, shortcut.Mods, shortcut.VK)
	if registered {
		hk.Unregister(shortcutProbeHotkeyID)
	}
	if !registered {
		reject()
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

func (app *App) connectionPayload() map[string]any {
	connected, busy, port := app.connectionStatus()
	return map[string]any{"connected": connected, "busy": busy, "port": port}
}

func (app *App) pushConnection() {
	if win := app.settingsWin; win != nil {
		payload := app.connectionPayload()
		payload["type"] = "connection"
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
