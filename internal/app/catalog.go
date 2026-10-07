//go:build windows

package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"sort"
	"strings"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/draw"
	"github.com/zolferfigueiredo/weej/internal/platform/display"
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
)

type catalogEntry struct {
	Section string   `json:"section"`
	Job     core.Job `json:"job"`
	Title   string   `json:"title"`
	Short   string   `json:"short"`
	Icon    string   `json:"icon"`
}

func sectionName(s core.Section) string {
	switch s {
	case core.SectionVolume:
		return "volume"
	case core.SectionBrightness:
		return "brightness"
	case core.SectionContrast:
		return "contrast"
	case core.SectionNightLight:
		return "nightLight"
	case core.SectionKeyboardBacklight:
		return "keyboard"
	case core.SectionZoom:
		return "zoom"
	case core.SectionApps:
		return "apps"
	default:
		return ""
	}
}

func (app *App) screenCount(devices []core.Device) int {
	externals := 0
	for _, m := range display.Monitors() {
		if m.Screen+1 > externals {
			externals = m.Screen + 1
		}
	}
	return core.ScreenCount(externals, devices)
}

func assignedAnywhere(devices []core.Device, kind core.JobKind) bool {
	for _, p := range allProfiles(devices) {
		for _, row := range p.Jobs {
			for _, j := range row {
				if j.Kind == kind {
					return true
				}
			}
		}
	}
	return false
}

// actionIcons is each button action's icon, by the action, or by the prefix of one that takes a
// setting (pickers.js actionIcon).
func (app *App) actionIcons() map[string]string {
	ink := app.glyphInk()
	icon := func(kind draw.GlyphKind) string { return pngDataURL(draw.Glyph(kind, 32, ink)) }
	return map[string]string{
		string(core.ActionPlayPause):       icon(draw.GlyphPlayPause),
		string(core.ActionPlay):            icon(draw.GlyphPlay),
		string(core.ActionPause):           icon(draw.GlyphPause),
		string(core.ActionStop):            icon(draw.GlyphStop),
		string(core.ActionPreviousTrack):   icon(draw.GlyphPreviousTrack),
		string(core.ActionNextTrack):       icon(draw.GlyphNextTrack),
		string(core.ActionVolumeUp):        icon(draw.GlyphVolumeUp),
		string(core.ActionVolumeDown):      icon(draw.GlyphVolumeDown),
		string(core.ActionMuteAll):         icon(draw.GlyphSpeakerMuted),
		string(core.ActionMuteMic):         icon(draw.GlyphMicMuted),
		string(core.ActionNightLight):      icon(draw.GlyphMoon),
		string(core.ActionScreensOff):      icon(draw.GlyphScreen),
		string(core.ActionLockPC):          icon(draw.GlyphLock),
		string(core.ActionSleepPC):         icon(draw.GlyphPower),
		string(core.ActionPreviousProfile): icon(draw.GlyphArrowLeft),
		string(core.ActionNextProfile):     icon(draw.GlyphArrowRight),
		string(core.ActionOpenSettings):    icon(draw.GlyphGear),
		string(core.ActionNextLights):      icon(draw.GlyphBulb),
		string(core.ActionPreviousLights):  icon(draw.GlyphBulb),
		string(core.ActionLightsOn):        icon(draw.GlyphBulbOn),
		string(core.ActionLightsOff):       icon(draw.GlyphBulbOff),
		"open:":                            icon(draw.GlyphApp),
		"close:":                           icon(draw.GlyphCloseApp),
		"url:":                             icon(draw.GlyphGlobe),
		"keys:":                            icon(draw.GlyphKeyboard),
		"profile:":                         icon(draw.GlyphList),
		"mute:":                            icon(draw.GlyphSpeakerMuted),
	}
}

func (app *App) buildCatalog(devices []core.Device) []catalogEntry {
	tr := app.trFunc()
	ink := app.glyphInk()
	glyphIcon := func(kind draw.GlyphKind) string { return pngDataURL(draw.Glyph(kind, 32, ink)) }

	var out []catalogEntry
	add := func(job core.Job, glyph draw.GlyphKind) {
		out = append(out, catalogEntry{
			Section: sectionName(job.Section()),
			Job:     job,
			Title:   job.Title(tr, app.appDisplayName),
			Short:   job.ShortTitle(tr, app.appDisplayName),
			Icon:    glyphIcon(glyph),
		})
	}

	add(core.Job{Kind: core.JobMaster}, draw.GlyphSpeaker)
	add(core.Job{Kind: core.JobMicrophone}, draw.GlyphMic)
	add(core.Job{Kind: core.JobSystemSounds}, draw.GlyphSpeaker)

	if display.HasBuiltinBrightness() || assignedAnywhere(devices, core.JobBuiltinBrightness) {
		add(core.Job{Kind: core.JobBuiltinBrightness}, draw.GlyphSun)
	}
	n := app.screenCount(devices)
	for i := 0; i < n; i++ {
		add(core.Job{Kind: core.JobBrightness, Screen: i}, draw.GlyphSun)
	}
	for i := 0; i < n; i++ {
		add(core.Job{Kind: core.JobContrast, Screen: i}, draw.GlyphContrast)
	}
	add(core.Job{Kind: core.JobNightLight}, draw.GlyphMoon)
	add(core.Job{Kind: core.JobExternalKeyboard}, draw.GlyphKeyboard)
	add(core.Job{Kind: core.JobZoom}, draw.GlyphZoom)

	add(core.Job{Kind: core.JobFocusedApp}, draw.GlyphApp)
	add(core.Job{Kind: core.JobOtherApps}, draw.GlyphApp)

	type named struct{ exe, name string }
	var apps []named
	for _, exe := range app.discoverApps(devices) {
		apps = append(apps, named{exe: exe, name: app.resolveApp(exe)})
	}
	sort.Slice(apps, func(i, j int) bool { return strings.ToLower(apps[i].name) < strings.ToLower(apps[j].name) })
	for _, a := range apps {
		job := core.Job{Kind: core.JobApp, Exe: a.exe}
		out = append(out, catalogEntry{
			Section: sectionName(job.Section()),
			Job:     job,
			Title:   a.name,
			Short:   a.name,
			Icon:    app.appIconDataURLFor(a.exe),
		})
	}
	return out
}

func (app *App) discoverApps(devices []core.Device) []string {
	set := map[string]struct{}{}
	for _, d := range devices {
		for _, exe := range d.Apps() {
			set[exe] = struct{}{}
		}
	}
	for _, k := range core.KnownApps {
		if app.resolveAppPath(k.Exe) != "" {
			set[k.Exe] = struct{}{}
		}
	}
	if app.audio != nil {
		for _, exe := range app.audio.Playing() {
			set[exe] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for e := range set {
		out = append(out, e)
	}
	return out
}

func (app *App) resolveAppPath(exe string) string {
	exe = strings.ToLower(exe)
	app.appCacheMu.Lock()
	if p, ok := app.appPath[exe]; ok {
		app.appCacheMu.Unlock()
		return p
	}
	app.appCacheMu.Unlock()

	path := appPathsRegistryPath(exe)
	if path == "" {
		path = runningProcessPath(exe)
	}

	app.appCacheMu.Lock()
	app.appPath[exe] = path
	app.appCacheMu.Unlock()
	return path
}

func (app *App) lookupAppPath(exe string) string {
	app.appCacheMu.Lock()
	defer app.appCacheMu.Unlock()
	return app.appPath[strings.ToLower(exe)]
}

func (app *App) resolveApp(exe string) string {
	exe = strings.ToLower(exe)
	app.appCacheMu.Lock()
	if n, ok := app.appName[exe]; ok {
		app.appCacheMu.Unlock()
		return n
	}
	app.appCacheMu.Unlock()

	name := ""
	if path := app.resolveAppPath(exe); path != "" {
		name = fileDescription(path)
	}
	if name == "" {
		for _, k := range core.KnownApps {
			if k.Exe == exe {
				name = k.Name
				break
			}
		}
	}
	if name == "" {
		name = exe
	}

	app.appCacheMu.Lock()
	app.appName[exe] = name
	app.appCacheMu.Unlock()
	return name
}

func (app *App) appIconDataURLFor(exe string) string {
	exe = strings.ToLower(exe)
	app.appCacheMu.Lock()
	if icon, ok := app.appIcon[exe]; ok {
		app.appCacheMu.Unlock()
		return icon
	}
	app.appCacheMu.Unlock()

	var img *image.NRGBA
	if path := app.resolveAppPath(exe); path != "" {
		img = winui.ExeIcon(path, 32)
	}
	if img == nil {
		img = draw.Glyph(draw.GlyphApp, 32, app.glyphInk())
	}
	url := pngDataURL(img)

	app.appCacheMu.Lock()
	app.appIcon[exe] = url
	app.appCacheMu.Unlock()
	return url
}

func (app *App) forgetApp(exe string) {
	exe = strings.ToLower(exe)
	app.appCacheMu.Lock()
	delete(app.appPath, exe)
	delete(app.appName, exe)
	delete(app.appIcon, exe)
	app.appCacheMu.Unlock()
}

func (app *App) shortcutLabels(s core.Settings) map[string]string {
	labels := map[string]string{}
	ctrlName := app.ctrlLabelName()
	add := func(s *core.Shortcut) {
		if s == nil {
			return
		}
		data, err := json.Marshal(s)
		if err != nil {
			return
		}
		labels[string(data)] = core.Label(*s, ctrlName)
	}
	for _, d := range s.Devices {
		add(d.Next)
		add(d.Previous)
		for _, p := range d.Profiles {
			add(p.Shortcut)
			for _, actions := range p.Buttons {
				for _, a := range actions {
					if keys, ok := a.Keys(); ok {
						add(&keys)
					}
				}
			}
		}
	}
	return labels
}

func pngDataURL(img *image.NRGBA) string {
	if img == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func appIconDataURL(px int) string { return pngDataURL(draw.AppIcon(px)) }

func allProfiles(devices []core.Device) []core.DeviceProfile {
	var out []core.DeviceProfile
	for _, d := range devices {
		out = append(out, d.Profiles...)
	}
	return out
}
