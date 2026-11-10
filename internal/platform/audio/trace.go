//go:build windows

package audio

import (
	"math"

	wca "github.com/moutend/go-wca/pkg/wca"
)

// ChangeLog is where the worker writes down each level that changes, and what changed it, while
// On.
type ChangeLog interface {
	On() bool
	Note(format string, args ...any)
}

func (a *Audio) noting() bool { return a.changes != nil && a.changes.On() }

func sessionName(info sessionInfo) string {
	switch {
	case info.isSystemSounds:
		return "System sounds"
	case info.exe != "":
		return info.exe
	}
	return "An app"
}

func pct(v float32) int { return int(math.Round(float64(v) * 100)) }

// wrote writes down a level WeeJ set, from what it read just before, and keeps it as seen.
func (a *Audio) wrote(key, name string, from, to float32, why string) {
	a.seen[key] = to
	if a.noting() && pct(from) != pct(to) {
		a.changes.Note("%s: %d%% to %d%%, by WeeJ: %s", name, pct(from), pct(to), why)
	}
}

// watchOutside reads every level and writes down the ones that changed since WeeJ last set or saw
// them: a change made by Windows, a game or another app.
func (a *Audio) watchOutside() {
	if !a.noting() {
		return
	}
	for _, ep := range []struct {
		flow uint32
		name string
	}{{wca.ERender, "Speaker volume"}, {wca.ECapture, "Microphone volume"}} {
		vol := a.endpointVolume(ep.flow)
		if vol == nil {
			continue
		}
		var level float32
		var muted bool
		if vol.GetMasterVolumeLevelScalar(&level) == nil {
			a.compare(ep.name, ep.name, level, false)
		}
		if vol.GetMute(&muted) == nil {
			a.compare(ep.name+" mute", ep.name, boolLevel(muted), true)
		}
		vol.Release()
	}
	a.rangeActiveRenderEndpoints(func(endpoint *wca.IMMDevice) {
		a.eachRenderSession(endpoint, func(info sessionInfo, vol *wca.ISimpleAudioVolume) {
			defer vol.Release()
			name := sessionName(info) + " volume"
			var level float32
			var muted bool
			if vol.GetMasterVolume(&level) == nil {
				a.compare(info.key, name, level, false)
			}
			if vol.GetMute(&muted) == nil {
				a.compare(info.key+" mute", name, boolLevel(muted), true)
			}
		})
	})
}

func (a *Audio) compare(key, name string, level float32, mute bool) {
	was, ok := a.seen[key]
	a.seen[key] = level
	switch {
	case !ok || pct(was) == pct(level):
	case mute && level > 0:
		a.changes.Note("%s: muted, by another app or Windows", name)
	case mute:
		a.changes.Note("%s: unmuted, by another app or Windows", name)
	default:
		a.changes.Note("%s: %d%% to %d%%, by another app or Windows", name, pct(was), pct(level))
	}
}

func boolLevel(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
