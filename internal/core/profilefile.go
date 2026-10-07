package core

import (
	"encoding/json"
	"fmt"
)

// profileFileKind marks a file Export wrote, so Import can tell it from a deej config.
const profileFileKind = "weej.profile"

type profileFileJSON struct {
	Kind    string      `json:"kind"`
	Version int         `json:"version"`
	Type    string      `json:"type"`
	Profile profileJSON `json:"profile"`
}

// EncodeProfileFile is a profile of a board of type t as Export saves it. Its shortcut stays
// behind: it belongs to the board it was set on.
func EncodeProfileFile(t DeviceType, p DeviceProfile) ([]byte, error) {
	buttons := p.Buttons
	if buttons == nil {
		buttons = ButtonMap{}
	}
	return json.MarshalIndent(profileFileJSON{
		Kind: profileFileKind, Version: 1, Type: string(t),
		Profile: profileJSON{Name: p.Name, Jobs: normalizeJobs(p.Jobs), Buttons: buttons},
	}, "", "  ")
}

// DecodeProfileFile reads a file Export wrote as a profile of board d. Jobs go by control; the
// buttons only come along from a board of the same type, since each type keys them its own way.
func DecodeProfileFile(data []byte, d Device) (DeviceProfile, bool) {
	var f profileFileJSON
	if err := json.Unmarshal(data, &f); err != nil || f.Kind != profileFileKind {
		return DeviceProfile{}, false
	}
	return profileFor(f.Profile, d, f.Type == string(d.Type)), true
}

// EncodeProfile is one profile in the shape Settings edits it.
func EncodeProfile(p DeviceProfile) ([]byte, error) {
	buttons := p.Buttons
	if buttons == nil {
		buttons = ButtonMap{}
	}
	return json.Marshal(profileJSON{Name: p.Name, Shortcut: p.Shortcut, Jobs: normalizeJobs(p.Jobs), Buttons: buttons})
}

// DecodeProfile reads one profile in the shape Settings edits it, as a profile of board d.
func DecodeProfile(data []byte, d Device) (DeviceProfile, bool) {
	var pj profileJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return DeviceProfile{}, false
	}
	return profileFor(pj, d, true), true
}

func profileFor(pj profileJSON, d Device, buttons bool) DeviceProfile {
	p := d.NewProfile(pj.Name)
	for i := range p.Jobs {
		if i < len(pj.Jobs) && pj.Jobs[i] != nil {
			p.Jobs[i] = pj.Jobs[i]
		}
	}
	if buttons && pj.Buttons != nil {
		p.Buttons = pj.Buttons
	}
	return p
}

// DeejProfile is a deej config as a profile of board d. Each slider's jobs go to the knob or
// fader on that input of a DIY board, or to the one at that place on any other; the sliders with
// no such control are added to what was skipped.
func DeejProfile(imp Import, d Device) (DeviceProfile, []string) {
	p := d.NewProfile(imp.Name)
	skipped := append([]string(nil), imp.Skipped...)
	for i, col := range imp.Columns {
		k := -1
		for c, ctl := range d.Controls {
			if ctl.Kind == KindButton {
				continue
			}
			if d.Type == DeviceDIY && ctl.Input == col || d.Type != DeviceDIY && c == col {
				k = c
				break
			}
		}
		if k < 0 {
			skipped = append(skipped, fmt.Sprintf("slider %d", col))
			continue
		}
		p.Jobs[k] = append(p.Jobs[k], imp.Jobs[i]...)
	}
	return p, skipped
}
