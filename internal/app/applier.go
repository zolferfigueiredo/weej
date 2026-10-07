//go:build windows

package app

import (
	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/display"
)

type applier struct{ app *App }

func (ap *applier) Apply(job core.Job, s float64) { ap.app.applyJob(job, s) }
func (ap *applier) HUD(job core.Job, s float64)   { ap.app.showHUD(job, s) }

func (app *App) applyJob(job core.Job, s float64) {
	switch job.Kind {
	case core.JobMaster:
		if app.audio != nil {
			app.audio.SetMaster(s)
		}
	case core.JobMicrophone:
		if app.audio != nil {
			app.audio.SetMic(s)
		}
	case core.JobSystemSounds:
		if app.audio != nil {
			app.audio.SetSystemSounds(s)
		}
	case core.JobApp:
		if app.audio != nil {
			app.audio.SetApps([]string{job.Exe}, s)
		}
	case core.JobOtherApps:
		if app.audio != nil {
			app.audio.SetOtherApps(app.activeProfileAppExes(), s)
		}
	case core.JobFocusedApp:
		if app.audio != nil {
			app.audio.SetFocused(s)
		}
	case core.JobBuiltinBrightness:
		display.SetBuiltinBrightness(core.Percent(s))
	case core.JobBrightness:
		if app.ddc != nil {
			app.ddc.Set(job.Screen, 0x10, core.Percent(s))
		}
	case core.JobContrast:
		if app.ddc != nil {
			app.ddc.Set(job.Screen, 0x12, core.Percent(s))
		}
	case core.JobExternalKeyboard:
		if app.via != nil {
			app.via.Set(s)
		}
	case core.JobNightLight:
		if app.nightlight != nil {
			app.nightlight.Set(s)
		}
	case core.JobZoom:
		if app.zoom != nil {
			app.zoom.Set(s)
		}
	}
}

func (app *App) activeProfileAppExes() []string {
	s := app.snapshotSettings()
	if s.Active < 0 || s.Active >= len(s.Profiles) {
		return nil
	}
	set := map[string]struct{}{}
	for _, row := range s.Profiles[s.Active].AllJobs() {
		for _, j := range row {
			if j.Kind == core.JobApp {
				set[j.Exe] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for e := range set {
		out = append(out, e)
	}
	return out
}
