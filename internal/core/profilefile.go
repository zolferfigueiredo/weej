package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// profileFileKind marks a file Export wrote, so Import can tell it from a deej config.
const profileFileKind = "weej.profile"

// profileFilePlatform is the system a file was exported on; TheeJ's say "macos". A file from
// before either said so is WeeJ's.
const profileFilePlatform = "windows"

type profileFileJSON struct {
	Kind     string      `json:"kind"`
	Version  int         `json:"version"`
	Type     string      `json:"type"`
	Platform string      `json:"platform"`
	Profile  profileJSON `json:"profile"`
}

// EncodeProfileFile is a profile of a board of type t as Export saves it. Its shortcut stays
// behind: it belongs to the board it was set on.
func EncodeProfileFile(t DeviceType, p DeviceProfile) ([]byte, error) {
	buttons := p.Buttons
	if buttons == nil {
		buttons = ButtonMap{}
	}
	return json.MarshalIndent(profileFileJSON{
		Kind: profileFileKind, Version: 1, Type: string(t), Platform: profileFilePlatform,
		Profile: profileJSON{Name: p.Name, Jobs: normalizeJobs(p.Jobs), Buttons: buttons},
	}, "", "  ")
}

// DecodeProfileFile reads a file Export wrote as a profile of board d. Jobs go by control; the
// buttons only come along from a board of the same type, since each type keys them its own way.
// A file made on another system leaves out what only works there: the jobs TheeJ keeps under
// "theej" for itself, named in skipped, and the actions that open or close an app or press keys,
// which name that system's apps and keys, by button in dropped.
func DecodeProfileFile(data []byte, d Device, tr Translate) (p DeviceProfile, skipped []string, dropped ButtonMap, ok bool) {
	var f profileFileJSON
	if err := json.Unmarshal(data, &f); err != nil || f.Kind != profileFileKind {
		return DeviceProfile{}, nil, nil, false
	}
	sameType := f.Type == string(d.Type)
	p = profileFor(f.Profile, d, sameType)
	if f.Platform == "" || f.Platform == profileFilePlatform {
		return p, nil, nil, true
	}
	// Read apart from the profile: a Mac's keys don't fit a Windows shortcut, so ButtonMap drops
	// them before they could be named.
	var other struct {
		Profile struct {
			Buttons map[string][]ButtonAction `json:"buttons"`
		} `json:"profile"`
		TheeJ struct {
			Jobs [][]map[string]json.RawMessage `json:"jobs"`
		} `json:"theej"`
	}
	_ = json.Unmarshal(data, &other)
	for _, row := range other.TheeJ.Jobs {
		for _, job := range row {
			var app struct {
				ID string `json:"_0"`
			}
			name := ""
			switch {
			case job["app"] != nil && json.Unmarshal(job["app"], &app) == nil:
				name = app.ID
			case job["builtinContrast"] != nil:
				name = tr("job.builtin_contrast", nil)
			case job["builtinKeyboard"] != nil:
				name = tr("job.builtin_keyboard", nil)
			}
			if name != "" && !slices.Contains(skipped, name) {
				skipped = append(skipped, name)
			}
		}
	}
	if !sameType {
		return p, skipped, nil, true
	}
	dropped = ButtonMap{}
	for key, actions := range other.Profile.Buttons {
		id, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		for _, a := range actions {
			if a.systemBound() {
				dropped[id] = append(dropped[id], a)
			}
		}
		if kept := slices.DeleteFunc(p.Buttons[id], ButtonAction.systemBound); len(kept) > 0 {
			p.Buttons[id] = kept
		} else {
			delete(p.Buttons, id)
		}
	}
	return p, skipped, dropped, true
}

// systemBound is an action that names an app or keys the way one system does, so on another it
// would open the wrong app or press the wrong keys.
func (a ButtonAction) systemBound() bool {
	for _, prefix := range []string{openActionPrefix, closeActionPrefix, keysActionPrefix} {
		if strings.HasPrefix(string(a), prefix) {
			return true
		}
	}
	return false
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
