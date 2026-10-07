//go:build windows

package midiport

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

// The faders' last positions are kept between runs, by device: a fader's LED can only be stopped
// by telling the mixer where the fader is, and a fader doesn't move while WeeJ is closed.
func fadersPath() string { return filepath.Join(sys.LocalDir(), "mixer-faders.json") }

type savedFaders map[string][8]*[2]int

func readFaders() savedFaders {
	saved := savedFaders{}
	if data, err := os.ReadFile(fadersPath()); err == nil {
		_ = json.Unmarshal(data, &saved)
	}
	return saved
}

func loadFaders(device string, state *core.MixerState) {
	for strip, p := range readFaders()[device] {
		if p != nil {
			state.SetPitch(strip, p[0], p[1])
		}
	}
}

func saveFaders(device string, state *core.MixerState) {
	saved := readFaders()
	var faders [8]*[2]int
	for strip := range faders {
		if lsb, msb, ok := state.Pitch(strip); ok {
			faders[strip] = &[2]int{lsb, msb}
		}
	}
	saved[device] = faders
	if data, err := json.Marshal(saved); err == nil {
		_ = sys.WriteFileAtomic(fadersPath(), data)
	}
}
