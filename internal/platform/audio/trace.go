//go:build windows

package audio

import (
	"math"

	wca "github.com/moutend/go-wca/pkg/wca"
)

// Tracer is the debug log: while On, the worker writes down every level it sets and every one it
// finds changed by something else.
type Tracer interface {
	On() bool
	Trace(tag, format string, args ...any)
}

func (a *Audio) tracef(format string, args ...any) {
	if a.tracer != nil {
		a.tracer.Trace("audio", format, args...)
	}
}

func sessionName(info sessionInfo) string {
	switch {
	case info.isSystemSounds:
		return "System sounds"
	case info.exe != "":
		return info.exe
	}
	return "an app"
}

// watchOutside reads every level and logs the ones that differ from what WeeJ last set or saw: a
// change made by Windows, a game or another app, since WeeJ writes down each of its own.
func (a *Audio) watchOutside() {
	if a.tracer == nil || !a.tracer.On() {
		return
	}
	for _, ep := range []struct {
		flow uint32
		name string
	}{{wca.ERender, "output"}, {wca.ECapture, "input"}} {
		vol := a.endpointVolume(ep.flow)
		if vol == nil {
			continue
		}
		var level float32
		var muted bool
		if vol.GetMasterVolumeLevelScalar(&level) == nil {
			a.compare(ep.name, ep.name, level)
		}
		if vol.GetMute(&muted) == nil {
			a.compare(ep.name+" mute", ep.name+" mute", boolLevel(muted))
		}
		vol.Release()
	}
	a.rangeActiveRenderEndpoints(func(endpoint *wca.IMMDevice) {
		a.eachRenderSession(endpoint, func(info sessionInfo, vol *wca.ISimpleAudioVolume) {
			defer vol.Release()
			name := sessionName(info) + " (session " + info.key + ")"
			var level float32
			var muted bool
			if vol.GetMasterVolume(&level) == nil {
				a.compare("session "+info.key, name, level)
			}
			if vol.GetMute(&muted) == nil {
				a.compare("session mute "+info.key, name+" mute", boolLevel(muted))
			}
		})
	})
}

func (a *Audio) compare(key, name string, level float32) {
	was, ok := a.seen[key]
	a.seen[key] = level
	switch {
	case !ok:
		a.tracer.Trace("now", "%s is %.3f", name, level)
	case math.Abs(float64(level-was)) > 0.005:
		a.tracer.Trace("outside", "%s went from %.3f to %.3f, not by WeeJ", name, was, level)
	}
}

func boolLevel(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
