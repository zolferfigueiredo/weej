//go:build windows

package app

import (
	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/audio"
	"github.com/zolferfigueiredo/weej/internal/platform/display"
)

type applier struct{ app *App }

func (ap *applier) Apply(job core.Job, s float64) { ap.app.applyJob(job, s) }
func (ap *applier) HUD(job core.Job, s float64)   { ap.app.showHUD(job, s) }

func (ap *applier) Level(job core.Job) (float64, bool) { return ap.app.jobLevel(job) }

func (app *App) jobLevel(job core.Job) (float64, bool) {
	if app.audio == nil {
		return 0, false
	}
	return app.audio.Level(job)
}

// Display is none for Focused app with no app in front to turn, WeeJ's own Settings included.
func (ap *applier) Display(job core.Job) string {
	if job.Kind == core.JobFocusedApp {
		if exe, _ := audio.ForegroundApp(); exe == "" {
			return ""
		}
	}
	if m, ok := ap.app.monitorFor(job); ok {
		return monitorKey(m)
	}
	return ""
}

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
			app.audio.SetApp(job.Exe, s)
		}
	case core.JobOtherApps:
		if app.audio != nil {
			apps, _, _ := core.ActiveAudioJobs(app.snapshotSettings().Devices)
			app.audio.SetOtherApps(apps, s)
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

// keepAppVolumes tells the audio thread which apps the profiles in use turn, so Other apps and new
// sessions follow a profile switch or Apply.
func (app *App) keepAppVolumes(s core.Settings) {
	if app.audio != nil {
		app.audio.Keep(core.ActiveAudioJobs(s.Devices))
	}
}
