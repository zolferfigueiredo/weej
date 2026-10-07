//go:build windows

package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/audio"
	"github.com/zolferfigueiredo/weej/internal/platform/midiport"
	"github.com/zolferfigueiredo/weej/internal/platform/serialport"
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
)

// runner reads one board: its loop, engine, connection and what Settings is shown of it.
type runner struct {
	id        string
	key       runnerKey
	engine    *core.Engine
	cancel    context.CancelFunc
	done      chan struct{}
	reconnect chan struct{}

	mu        sync.Mutex
	midi      *midiport.Port
	connected bool
	busy      bool
	port      string
	moves     core.MoveWatcher
	buttons   core.ButtonWatcher
	live      liveFrames
}

// runnerKey is what a runner was started with: a change to any of it starts the board again.
type runnerKey struct {
	typ  core.DeviceType
	port string
	baud int
}

func (r *runner) status() (connected, busy bool, port string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.connected, r.busy, r.port
}

func (r *runner) midiPort() *midiport.Port {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.midi
}

func (app *App) now() float64 { return time.Since(app.startTime).Seconds() }

// device is the board with that ID as last saved.
func (app *App) device(id string) (core.Device, bool) {
	for _, d := range app.snapshotSettings().Devices {
		if d.ID == id {
			return d, true
		}
	}
	return core.Device{}, false
}

func deviceIndex(s core.Settings, id string) int {
	for i, d := range s.Devices {
		if d.ID == id {
			return i
		}
	}
	return -1
}

func (app *App) runnerFor(id string) *runner {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.runners[id]
}

func (app *App) allRunners() []*runner {
	app.mu.Lock()
	defer app.mu.Unlock()
	out := make([]*runner, 0, len(app.runners))
	for _, r := range app.runners {
		out = append(out, r)
	}
	return out
}

// syncRunners starts every board that is on and not running, starts again one whose type, port
// or speed changed, and stops the rest. A port given on the command line is the first DIY board's.
func (app *App) syncRunners(devices []core.Device) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.runners == nil {
		app.runners = map[string]*runner{}
	}
	keep := map[string]bool{}
	firstDIY := true
	for _, d := range devices {
		port := d.Port
		if d.Type == core.DeviceDIY {
			if firstDIY && app.forcedPort != "" {
				port = app.forcedPort
			}
			firstDIY = false
		}
		if !d.Enabled {
			continue
		}
		keep[d.ID] = true
		key := runnerKey{typ: d.Type, port: port, baud: d.BaudRate()}
		prev := app.runners[d.ID]
		if prev != nil && prev.key == key {
			continue
		}
		app.runners[d.ID] = app.startRunner(d.ID, key, prev)
	}
	for id, r := range app.runners {
		if !keep[id] {
			r.cancel()
			delete(app.runners, id)
			go func(id string, r *runner) {
				<-r.done
				app.onRunnerStatus(r, id, false, false, "")
			}(id, r)
		}
	}
}

// startRunner starts a board's loop once the loop it replaces has let go of the port.
func (app *App) startRunner(id string, key runnerKey, prev *runner) *runner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &runner{
		id: id, key: key, engine: core.NewEngine(&applier{app: app}),
		cancel: cancel, done: make(chan struct{}), reconnect: make(chan struct{}, 1),
	}
	go func() {
		defer close(r.done)
		if prev != nil {
			prev.cancel()
			<-prev.done
		}
		status := func(connected, busy bool, port string) { app.onRunnerStatus(r, id, connected, busy, port) }
		if key.typ == core.DeviceDIY {
			serialport.Run(ctx, serialport.Config{
				Owner:      id,
				Exclude:    func() []string { return app.portsOfOtherBoards(id) },
				ForcedPort: key.port,
				Baud:       key.baud,
				OnLine:     func(values []int) { app.onRawFrame(r, values) },
				OnStatus:   func(s serialport.Status) { status(s.Connected, s.Busy, s.Port) },
				Log:        app.log,
			}, r.reconnect)
			return
		}
		port := midiport.New(midiport.Config{
			Device:   key.port,
			ID:       id,
			OnValues: func(values []int) { app.onRawFrame(r, values) },
			OnButton: func(button int) { app.onMIDIButton(r, button) },
			OnStatus: func(connected, busy bool) { status(connected, busy, key.port) },
			Lights:   key.typ == core.DeviceSMC,
			Log:      app.log,
		})
		if d, ok := app.device(id); ok {
			port.SetLights(d.Lights)
		}
		r.mu.Lock()
		r.midi = port
		r.mu.Unlock()
		port.Run(ctx, r.reconnect)
	}()
	return r
}

// portsOfOtherBoards lists the COM ports other DIY boards are set to, which finding a board
// automatically leaves alone.
func (app *App) portsOfOtherBoards(id string) []string {
	var out []string
	for _, d := range app.snapshotSettings().Devices {
		if d.ID != id && d.Type == core.DeviceDIY && d.Enabled && d.Port != "" {
			out = append(out, d.Port)
		}
	}
	return out
}

func (app *App) onRunnerStatus(r *runner, id string, connected, busy bool, port string) {
	r.mu.Lock()
	r.connected, r.busy, r.port = connected, busy, port
	r.mu.Unlock()
	if !connected {
		r.engine.Reset()
		r.mu.Lock()
		r.buttons.Reset()
		r.mu.Unlock()
	}
	app.loop.Invoke(func() {
		app.refreshTrayNow()
		app.pushStatus(id)
	})
}

// onRawFrame runs a board's frame through its engine, unless the board is being calibrated.
func (app *App) onRawFrame(r *runner, raw []int) {
	d, ok := app.device(r.id)
	if !ok {
		return
	}
	now := app.now()
	calibrating := false
	if w := app.wizardFor(r.id); w != nil {
		calibrating = true
		w.feed(raw, now)
	}
	norm := d.Normalize(raw)
	r.engine.Handle(norm, d.EngineSetup(), calibrating)
	app.showValues(r, d.Shown(raw))
	if calibrating {
		return
	}
	r.mu.Lock()
	moved := r.moves.Moved(norm)
	var pressed []int
	if d.Type == core.DeviceDIY {
		pressed = r.buttons.Pressed(raw, d.ButtonInputs(), now)
	}
	r.mu.Unlock()
	if win := app.settingsWin; win != nil {
		for _, k := range moved {
			win.Send(map[string]any{"type": "moved", "device": r.id, "control": k})
		}
	}
	for _, k := range pressed {
		app.onButton(r, d, k)
	}
}

func (app *App) onMIDIButton(r *runner, id int) {
	if w := app.wizardFor(r.id); w != nil {
		w.press(id)
		return
	}
	if d, ok := app.device(r.id); ok {
		app.onButton(r, d, id)
	}
}

// onButton runs a button's actions; key is how the board's profiles know it (core.DeviceProfile).
func (app *App) onButton(r *runner, d core.Device, key int) {
	actions := d.ActiveButtons()[key]
	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = string(a)
	}
	if len(names) == 0 {
		names = []string{"empty"}
	}
	app.log(fmt.Sprintf("%s button %d pressed: %s", d.Name, key, strings.Join(names, ", ")))
	for _, action := range actions {
		app.runButtonAction(action, r, d, key)
	}
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "pressed", "device": d.ID, "key": key})
	}
}

// liveFrames keeps a board's last values for the controls Settings draws.
type liveFrames struct {
	mu    sync.Mutex
	last  []int
	timer *time.Timer
}

func (l *liveFrames) frame() []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last
}

// showValues hands Settings a board's values at most 20 times a second, the last always among
// them.
func (app *App) showValues(r *runner, values []int) {
	live := &r.live
	live.mu.Lock()
	defer live.mu.Unlock()
	live.last = values
	if live.timer == nil && app.settingsWin != nil {
		live.timer = time.AfterFunc(50*time.Millisecond, func() {
			live.mu.Lock()
			values := live.last
			live.timer = nil
			live.mu.Unlock()
			if win := app.settingsWin; win != nil {
				win.Send(map[string]any{"type": "values", "device": r.id, "values": values})
			}
		})
	}
}

// runButtonAction does one of a button's actions. A mute, a profile step or a light pattern acts
// on the board the button is on.
func (app *App) runButtonAction(action core.ButtonAction, r *runner, d core.Device, key int) {
	if k, ok := action.MuteKnob(); ok && d.IsPot(k) && d.Controls[k].Input >= 0 {
		muted := r.engine.ToggleMute(k, d.EngineSetup())
		r.midiPort().SetLED(key, muted)
	}
	if path, ok := action.OpenApp(); ok && !winui.FocusApp(core.ExeName(path)) {
		launch(path)
	}
	if exe, ok := action.CloseApp(); ok {
		winui.CloseApp(exe)
	}
	if url, ok := action.URL(); ok {
		openURL(url)
	}
	if keys, ok := action.Keys(); ok {
		winui.SendShortcut(keys.Mods, keys.VK)
	}
	if i, ok := action.Profile(); ok {
		app.loop.Invoke(func() { app.setProfiles([]core.HotkeyTarget{{Device: d.ID, Profile: i}}) })
	}
	switch action {
	case core.ActionPlayPause:
		winui.MediaKey(winui.VKMediaPlayPause)
	case core.ActionPlay:
		winui.AppCommand(winui.AppCommandMediaPlay)
	case core.ActionPause:
		winui.AppCommand(winui.AppCommandMediaPause)
	case core.ActionVolumeUp:
		winui.MediaKey(winui.VKVolumeUp)
	case core.ActionVolumeDown:
		winui.MediaKey(winui.VKVolumeDown)
	case core.ActionMuteAll:
		winui.MediaKey(winui.VKVolumeMute)
	case core.ActionMuteMic:
		if app.audio != nil {
			app.audio.ToggleMicMute()
		}
	case core.ActionNightLight:
		app.nightlight.Toggle()
	case core.ActionScreensOff:
		winui.ScreensOff()
	case core.ActionLockPC:
		winui.LockPC()
	case core.ActionSleepPC:
		winui.SleepPC()
	case core.ActionStop:
		winui.MediaKey(winui.VKMediaStop)
	case core.ActionPreviousTrack:
		winui.MediaKey(winui.VKMediaPrevTrack)
	case core.ActionNextTrack:
		winui.MediaKey(winui.VKMediaNextTrack)
	case core.ActionNextProfile:
		app.loop.Invoke(func() { app.setProfiles([]core.HotkeyTarget{{Device: d.ID, Step: 1}}) })
	case core.ActionPreviousProfile:
		app.loop.Invoke(func() { app.setProfiles([]core.HotkeyTarget{{Device: d.ID, Step: -1}}) })
	case core.ActionOpenSettings:
		app.loop.Invoke(func() { app.openSettings("") })
	case core.ActionNextLights:
		app.changeLights(d.ID, core.NextLightPattern)
	case core.ActionPreviousLights:
		app.changeLights(d.ID, core.PreviousLightPattern)
	case core.ActionLightsOn:
		app.changeLights(d.ID, func(string) string { return "on" })
	case core.ActionLightsOff:
		app.changeLights(d.ID, func(string) string { return "" })
	}
}

// setProfiles switches boards' profiles, as shortcuts, the tray and buttons do, and saves once.
func (app *App) setProfiles(targets []core.HotkeyTarget) {
	s := app.snapshotSettings()
	var hud *core.Device
	for _, t := range targets {
		i := deviceIndex(s, t.Device)
		if i < 0 {
			continue
		}
		d := &s.Devices[i]
		next := t.Profile
		if t.Step != 0 {
			next = d.Stepped(t.Step)
		}
		if next < 0 || next >= len(d.Profiles) {
			continue
		}
		app.unmuteDevice(*d)
		d.Active = next
		if hud == nil {
			hud = d
		}
		if win := app.settingsWin; win != nil {
			win.Send(map[string]any{"type": "profile", "device": d.ID, "profile": next})
		}
	}
	if hud == nil {
		return
	}
	if err := app.persistSettings(s); err != nil {
		app.log("Could not save settings: " + err.Error())
	}
	app.refreshTray()
	app.showProfileHUD(*hud, len(s.Devices) > 1)
}

// unmuteDevice puts a board's muted controls back before its profile changes: the next profile
// gives them other jobs, so a muted one could no longer be unmuted from its button.
func (app *App) unmuteDevice(d core.Device) {
	r := app.runnerFor(d.ID)
	if r == nil || r.engine.UnmuteAll(d.EngineSetup()) == 0 {
		return
	}
	port := r.midiPort()
	for key, actions := range d.ActiveButtons() {
		for _, a := range actions {
			if _, ok := a.MuteKnob(); ok {
				port.SetLED(key, false)
			}
		}
	}
}

// applyLights runs each SMC-Mixer's button light pattern, and listens to the speakers only while
// one of them follows the sound.
func (app *App) applyLights(devices []core.Device) {
	eq := false
	for _, d := range devices {
		if d.Type != core.DeviceSMC || !d.Enabled {
			continue
		}
		eq = eq || core.IsEQ(d.Lights)
		if r := app.runnerFor(d.ID); r != nil {
			r.midiPort().SetLights(d.Lights)
		}
	}
	app.mu.Lock()
	if app.spectrum == nil {
		app.spectrum = core.NewSpectrum()
		midiport.SetSpectrum(app.spectrum)
	}
	stop := app.loopback
	switch {
	case eq && stop == nil:
		app.loopback = audio.StartLoopback(app.spectrum, app.log)
	case eq:
		stop = nil
	default:
		app.loopback = nil
	}
	app.mu.Unlock()
	if stop != nil {
		stop.Stop()
	}
}

// changeLights changes the light pattern of the board a button is on, or of every SMC-Mixer when
// that board has none, and saves it as Settings would.
func (app *App) changeLights(id string, change func(string) string) {
	app.loop.Invoke(func() {
		s := app.snapshotSettings()
		own := deviceIndex(s, id)
		smc := own >= 0 && s.Devices[own].Type == core.DeviceSMC
		var changed []int
		for i, d := range s.Devices {
			if d.Type == core.DeviceSMC && (!smc || i == own) {
				s.Devices[i].Lights = core.ParseLightPattern(change(d.Lights))
				changed = append(changed, i)
			}
		}
		if len(changed) == 0 {
			return
		}
		if err := app.persistSettings(s); err != nil {
			return
		}
		app.applyLights(s.Devices)
		if win := app.settingsWin; win != nil {
			for _, i := range changed {
				win.Send(map[string]any{"type": "lights", "device": s.Devices[i].ID, "pattern": s.Devices[i].Lights})
			}
		}
	})
}

func (app *App) requestReconnect(id string) {
	for _, r := range app.allRunners() {
		if id != "" && r.id != id {
			continue
		}
		select {
		case r.reconnect <- struct{}{}:
		default:
		}
	}
}

// stopRunners stops every board, waiting up to a second so the MIDI ones can put their lights
// out and keep their faders' positions.
func (app *App) stopRunners() {
	app.mu.Lock()
	runners := app.runners
	app.runners = nil
	app.mu.Unlock()
	for _, r := range runners {
		r.cancel()
	}
	deadline := time.After(time.Second)
	for _, r := range runners {
		select {
		case <-r.done:
		case <-deadline:
			return
		}
	}
}
