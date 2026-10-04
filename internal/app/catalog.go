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
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
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

func (app *App) screenCount(setup core.Setup) int {
	externals := 0
	for _, m := range display.Monitors() {
		if m.Screen+1 > externals {
			externals = m.Screen + 1
		}
	}
	highest := -1
	for _, p := range setup.Profiles {
		for _, row := range p.Jobs {
			for _, j := range row {
				if (j.Kind == core.JobBrightness || j.Kind == core.JobContrast) && j.Screen > highest {
					highest = j.Screen
				}
			}
		}
	}
	n := 2
	if externals > n {
		n = externals
	}
	if highest+1 > n {
		n = highest + 1
	}
	return n
}

func assignedAnywhere(setup core.Setup, kind core.JobKind) bool {
	for _, p := range setup.Profiles {
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

func (app *App) buildCatalog(setup core.Setup) []catalogEntry {
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

	if display.HasBuiltinBrightness() || assignedAnywhere(setup, core.JobBuiltinBrightness) {
		add(core.Job{Kind: core.JobBuiltinBrightness}, draw.GlyphSun)
	}
	n := app.screenCount(setup)
	for i := 0; i < n; i++ {
		add(core.Job{Kind: core.JobBrightness, Screen: i}, draw.GlyphSun)
	}
	for i := 0; i < n; i++ {
		add(core.Job{Kind: core.JobContrast, Screen: i}, draw.GlyphContrast)
	}
	// Night light has no backend on Windows yet; offer it only where an old profile still has it.
	if assignedAnywhere(setup, core.JobNightLight) {
		add(core.Job{Kind: core.JobNightLight}, draw.GlyphMoon)
	}
	add(core.Job{Kind: core.JobExternalKeyboard}, draw.GlyphKeyboard)
	add(core.Job{Kind: core.JobZoom}, draw.GlyphZoom)

	add(core.Job{Kind: core.JobFocusedApp}, draw.GlyphApp)
	add(core.Job{Kind: core.JobOtherApps}, draw.GlyphApp)

	type named struct{ exe, name string }
	var apps []named
	for _, exe := range app.discoverApps(setup) {
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

func (app *App) discoverApps(setup core.Setup) []string {
	set := map[string]struct{}{}
	for _, exe := range setup.Apps() {
		set[exe] = struct{}{}
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

func (app *App) iconPreviews() map[string]string {
	light := sys.TaskbarLight()
	out := map[string]string{}
	for _, style := range []core.IconStyle{core.IconMixer, core.IconDial, core.IconApp} {
		out[string(style)] = pngDataURL(draw.TrayIcon(draw.IconStyle(style), true, 32, light))
	}
	return out
}

func (app *App) shortcutLabels(setup core.Setup) map[string]string {
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
	add(setup.Next)
	add(setup.Previous)
	for _, p := range setup.Profiles {
		add(p.Shortcut)
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
