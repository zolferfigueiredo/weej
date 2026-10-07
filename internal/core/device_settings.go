package core

import (
	"encoding/json"
	"fmt"
	"time"
)

// Settings is everything WeeJ saves: its boards, and its own settings.
type Settings struct {
	Devices []Device
	// Added counts every board ever added, so NextDeviceID never hands out an ID again.
	Added int

	HideIcon         bool
	ShowProfiles     bool
	Language         string
	ShowDataInMenu   bool
	UpdateEvery      int
	LastUpdateCheck  time.Time
	AvailableVersion string
	NotifiedVersion  string
	UpdatedTo        string
	TrayTipShown     bool
}

// settingsVersion marks the file as one of boards; a file without it is from before boards and
// is not read at all.
const settingsVersion = 2

func DefaultSettings() Settings {
	return Settings{ShowProfiles: true, ShowDataInMenu: true, UpdateEvery: 604800}
}

// The on-disk shape, key order fixed so EncodeSettings is deterministic.
type settingsJSON struct {
	Version          int          `json:"version"`
	Devices          []deviceJSON `json:"devices"`
	Added            int          `json:"added"`
	HideTrayIcon     bool         `json:"hideTrayIcon"`
	ShowProfileList  bool         `json:"showProfileList"`
	Language         string       `json:"language"`
	ShowDataInMenu   bool         `json:"showDataInMenu"`
	UpdateEvery      int          `json:"updateEvery"`
	LastUpdateCheck  *time.Time   `json:"lastUpdateCheck,omitempty"`
	AvailableVersion string       `json:"availableVersion"`
	NotifiedVersion  string       `json:"notifiedVersion"`
	UpdatedTo        string       `json:"updatedTo"`
	TrayTipShown     bool         `json:"trayTipShown"`
}

type deviceJSON struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Type            string        `json:"type"`
	Enabled         bool          `json:"enabled"`
	Port            string        `json:"port"`
	BaudRate        int           `json:"baudRate,omitempty"`
	Speed           string        `json:"speed"`
	Controls        []controlJSON `json:"controls"`
	Layout          [][]int       `json:"layout,omitempty"`
	View            string        `json:"view"`
	Profiles        []profileJSON `json:"profiles"`
	Profile         int           `json:"profile"`
	NextProfile     *Shortcut     `json:"nextProfile,omitempty"`
	PreviousProfile *Shortcut     `json:"previousProfile,omitempty"`
	Lights          string        `json:"lights,omitempty"`
}

type controlJSON struct {
	Kind    string `json:"kind"`
	Input   *int   `json:"input"`
	Reverse bool   `json:"reverse,omitempty"`
	Min     int    `json:"min"`
	Max     int    `json:"max"`
}

type profileJSON struct {
	Name     string    `json:"name"`
	Shortcut *Shortcut `json:"shortcut,omitempty"`
	Jobs     [][]Job   `json:"jobs"`
	Buttons  ButtonMap `json:"buttons"`
}

func EncodeSettings(s Settings) ([]byte, error) {
	out := settingsJSON{
		Version:          settingsVersion,
		Devices:          make([]deviceJSON, len(s.Devices)),
		Added:            s.Added,
		HideTrayIcon:     s.HideIcon,
		ShowProfileList:  s.ShowProfiles,
		Language:         s.Language,
		ShowDataInMenu:   s.ShowDataInMenu,
		UpdateEvery:      s.UpdateEvery,
		AvailableVersion: s.AvailableVersion,
		NotifiedVersion:  s.NotifiedVersion,
		UpdatedTo:        s.UpdatedTo,
		TrayTipShown:     s.TrayTipShown,
	}
	for i, d := range s.Devices {
		out.Devices[i] = encodeDevice(d)
	}
	if !s.LastUpdateCheck.IsZero() {
		out.LastUpdateCheck = &s.LastUpdateCheck
	}
	return json.MarshalIndent(out, "", "  ")
}

func encodeDevice(d Device) deviceJSON {
	out := deviceJSON{
		ID:              d.ID,
		Name:            d.Name,
		Type:            string(d.Type),
		Enabled:         d.Enabled,
		Port:            d.Port,
		BaudRate:        d.Baud,
		Speed:           string(d.Speed),
		Controls:        make([]controlJSON, len(d.Controls)),
		Layout:          d.Layout,
		View:            d.View,
		Profiles:        make([]profileJSON, len(d.Profiles)),
		Profile:         d.Active,
		NextProfile:     d.Next,
		PreviousProfile: d.Previous,
		Lights:          d.Lights,
	}
	for i, c := range d.Controls {
		out.Controls[i] = controlJSON{Kind: string(c.Kind), Reverse: c.Reverse, Min: c.Min, Max: c.Max}
		if c.Input >= 0 {
			input := c.Input
			out.Controls[i].Input = &input
		}
	}
	for i, p := range d.Profiles {
		buttons := p.Buttons
		if buttons == nil {
			buttons = ButtonMap{}
		}
		out.Profiles[i] = profileJSON{Name: p.Name, Shortcut: p.Shortcut, Jobs: normalizeJobs(p.Jobs), Buttons: buttons}
	}
	return out
}

// DecodeSettings never fails: a file from before boards, or one that isn't JSON, gives the
// defaults, and a bad value falls back to its default on its own. profileName names the profile
// a board without any gets.
func DecodeSettings(data []byte, profileName string) Settings {
	s := DefaultSettings()
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return s
	}
	if v, _ := take[int](raw, "version"); v != settingsVersion {
		return s
	}
	if v, ok := take[int](raw, "added"); ok && v > 0 {
		s.Added = v
	}
	devices, _ := take[[]json.RawMessage](raw, "devices")
	seen := map[string]bool{}
	for _, rd := range devices {
		d, ok := decodeDevice(rd, profileName)
		if !ok {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(d.ID, "d%d", &n); err == nil && n > s.Added {
			s.Added = n
		}
		if d.ID == "" || seen[d.ID] {
			d.ID = ""
		}
		seen[d.ID] = true
		s.Devices = append(s.Devices, d)
	}
	for i := range s.Devices {
		if s.Devices[i].ID == "" {
			s.Devices[i].ID = NextDeviceID(s.Added)
			s.Added++
		}
	}

	if v, ok := take[bool](raw, "hideTrayIcon"); ok {
		s.HideIcon = v
	}
	if v, ok := take[bool](raw, "showProfileList"); ok {
		s.ShowProfiles = v
	}
	if v, ok := take[string](raw, "language"); ok {
		s.Language = v
	}
	if v, ok := take[bool](raw, "showDataInMenu"); ok {
		s.ShowDataInMenu = v
	}
	if v, ok := take[int](raw, "updateEvery"); ok && (v == 0 || v == 86400 || v == 604800) {
		s.UpdateEvery = v
	}
	if v, ok := take[time.Time](raw, "lastUpdateCheck"); ok {
		s.LastUpdateCheck = v
	}
	if v, ok := take[string](raw, "availableVersion"); ok {
		s.AvailableVersion = v
	}
	if v, ok := take[string](raw, "notifiedVersion"); ok {
		s.NotifiedVersion = v
	}
	if v, ok := take[string](raw, "updatedTo"); ok {
		s.UpdatedTo = v
	}
	if v, ok := take[bool](raw, "trayTipShown"); ok {
		s.TrayTipShown = v
	}
	return s
}

// decodeDevice reads one board, dropping it only when its type is unknown.
func decodeDevice(data json.RawMessage, profileName string) (Device, bool) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return Device{}, false
	}
	typ, _ := take[string](raw, "type")
	t, ok := ParseDeviceType(typ)
	if !ok {
		return Device{}, false
	}
	d := Device{Type: t, Enabled: true, Speed: SpeedSlow, View: ViewDraw}
	d.ID, _ = take[string](raw, "id")
	d.Name, _ = take[string](raw, "name")
	if v, ok := take[bool](raw, "enabled"); ok {
		d.Enabled = v
	}
	d.Port, _ = take[string](raw, "port")
	if v, ok := take[int](raw, "baudRate"); ok && v > 0 {
		d.Baud = v
	}
	if v, ok := take[string](raw, "speed"); ok {
		d.Speed = ParseSpeed(v)
	}

	if t == DeviceSMC {
		d.Controls = SMCControls()
	} else if controls, ok := take[[]controlJSON](raw, "controls"); ok {
		for _, c := range controls {
			ctl := Control{Kind: ParseControlKind(c.Kind), Input: -1, Reverse: c.Reverse, Min: c.Min, Max: c.Max}
			if c.Input != nil && *c.Input >= 0 {
				ctl.Input = *c.Input
			}
			if ctl.Min < 0 || ctl.Max > 1023 || ctl.Max <= ctl.Min {
				ctl.Min, ctl.Max = 0, 1023
			}
			d.Controls = append(d.Controls, ctl)
		}
	}
	if t != DeviceSMC {
		layout, _ := take[[][]int](raw, "layout")
		d.Layout = CleanLayout(layout, len(d.Controls))
		if d.Layout == nil {
			d.Layout = DefaultLayout(d.Controls)
		}
	}
	if v, _ := take[string](raw, "view"); v == ViewList {
		d.View = ViewList
	}

	profiles, _ := take[[]profileJSON](raw, "profiles")
	for _, p := range profiles {
		dp := DeviceProfile{Name: p.Name, Shortcut: p.Shortcut, Jobs: make([][]Job, len(d.Controls)), Buttons: p.Buttons}
		for i := range dp.Jobs {
			dp.Jobs[i] = []Job{}
			if i < len(p.Jobs) && p.Jobs[i] != nil {
				dp.Jobs[i] = p.Jobs[i]
			}
		}
		if dp.Buttons == nil {
			dp.Buttons = ButtonMap{}
		}
		d.Profiles = append(d.Profiles, dp)
	}
	if len(d.Profiles) == 0 {
		d.Profiles = []DeviceProfile{d.NewProfile(profileName)}
	}
	d.Active, _ = take[int](raw, "profile")
	d.Active = min(max(d.Active, 0), len(d.Profiles)-1)
	d.Next, _ = take[*Shortcut](raw, "nextProfile")
	d.Previous, _ = take[*Shortcut](raw, "previousProfile")
	if t == DeviceSMC {
		v, _ := take[string](raw, "lights")
		d.Lights = ParseLightPattern(v)
	}
	return d, true
}

// EncodeDevice is one board in the file's shape, which Settings edits it in.
func EncodeDevice(d Device) ([]byte, error) { return json.Marshal(encodeDevice(d)) }

// DecodeDevice reads one board as the file holds it, as tolerantly as DecodeSettings.
func DecodeDevice(data []byte, profileName string) (Device, bool) {
	return decodeDevice(data, profileName)
}
