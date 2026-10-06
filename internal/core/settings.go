package core

import (
	"encoding/json"
	"strconv"
	"time"
)

type Settings struct {
	Setup

	Language         string
	ShowDataInMenu   bool
	UpdateEvery      int
	LastUpdateCheck  time.Time
	AvailableVersion string
	NotifiedVersion  string
	UpdatedTo        string
	TrayTipShown     bool
}

func DefaultSettings(defaultProfileName string) Settings {
	return Settings{
		Setup: Setup{
			Profiles:     []Profile{{Name: defaultProfileName}},
			ShowProfiles: true,
			Icon:         IconMixer,
			Speed:        SpeedSlow,
			Buttons:      DefaultMixerButtons(),
		},
		ShowDataInMenu: true,
		UpdateEvery:    604800,
	}
}

// The on-disk shape, key order matching the spec exactly so EncodeSettings is deterministic.
type settingsJSON struct {
	Columns          []*int            `json:"columns"`
	Profiles         []Profile         `json:"profiles"`
	Profile          int               `json:"profile"`
	NextProfile      *Shortcut         `json:"nextProfile"`
	PreviousProfile  *Shortcut         `json:"previousProfile"`
	InvertKnobs      bool              `json:"invertKnobs"`
	HideTrayIcon     bool              `json:"hideTrayIcon"`
	ShowProfileList  bool              `json:"showProfileList"`
	TrayIcon         string            `json:"trayIcon"`
	Speed            string            `json:"speed"`
	Port             string            `json:"port"`
	BaudRate         int               `json:"baudRate"`
	MixerColumns     []*int            `json:"mixerColumns"`
	MixerButtonOrder []int             `json:"mixerButtonOrder"`
	MixerButtons     map[string]string `json:"mixerButtons"`
	Language         string            `json:"language"`
	ShowDataInMenu   bool              `json:"showDataInMenu"`
	UpdateEvery      int               `json:"updateEvery"`
	LastUpdateCheck  *time.Time        `json:"lastUpdateCheck,omitempty"`
	AvailableVersion string            `json:"availableVersion"`
	NotifiedVersion  string            `json:"notifiedVersion"`
	UpdatedTo        string            `json:"updatedTo"`
	TrayTipShown     bool              `json:"trayTipShown"`
}

func columnsToJSON(cols []int) []*int {
	out := make([]*int, len(cols))
	for i, c := range cols {
		if c == -1 {
			continue
		}
		v := c
		out[i] = &v
	}
	return out
}

func columnsFromJSON(ptrs []*int) []int {
	out := make([]int, len(ptrs))
	for i, p := range ptrs {
		if p == nil {
			out[i] = -1
		} else {
			out[i] = *p
		}
	}
	return out
}

// EncodeMixerColumns keeps a never-calibrated mixer as null, apart from an empty calibration.
func EncodeMixerColumns(cols []int) []*int {
	if cols == nil {
		return nil
	}
	return columnsToJSON(cols)
}

func EncodeButtons(buttons map[int]ButtonAction) map[string]string {
	out := map[string]string{}
	for cc, a := range buttons {
		if a != ActionNone {
			out[strconv.Itoa(cc)] = string(a)
		}
	}
	return out
}

// A saved map replaces the defaults whole, so a button set to Nothing stays that way.
func DecodeButtons(raw map[string]string) map[int]ButtonAction {
	out := map[int]ButtonAction{}
	for k, v := range raw {
		cc, err := strconv.Atoi(k)
		a := ButtonAction(v)
		if err != nil || cc < 0 || cc > 255 || a == ActionNone || !a.Valid() {
			continue
		}
		out[cc] = a
	}
	return out
}

func normalizeProfile(p Profile) Profile {
	jobs := p.Jobs
	if jobs == nil {
		jobs = [][]Job{}
	}
	for i, row := range jobs {
		if row == nil {
			jobs[i] = []Job{}
		}
	}
	p.Jobs = jobs
	return p
}

func EncodeSettings(s Settings) ([]byte, error) {
	profiles := make([]Profile, len(s.Profiles))
	for i, p := range s.Profiles {
		profiles[i] = normalizeProfile(p)
	}
	columns := s.Columns
	if columns == nil {
		columns = []int{}
	}

	out := settingsJSON{
		Columns:          columnsToJSON(columns),
		Profiles:         profiles,
		Profile:          s.Active,
		NextProfile:      s.Next,
		PreviousProfile:  s.Previous,
		InvertKnobs:      s.Invert,
		HideTrayIcon:     s.HideIcon,
		ShowProfileList:  s.ShowProfiles,
		TrayIcon:         string(s.Icon),
		Speed:            string(s.Speed),
		Port:             s.Port,
		BaudRate:         s.BaudRate(),
		MixerColumns:     EncodeMixerColumns(s.MixerColumns),
		MixerButtonOrder: s.ButtonOrder,
		MixerButtons:     EncodeButtons(s.Buttons),
		Language:         s.Language,
		ShowDataInMenu:   s.ShowDataInMenu,
		UpdateEvery:      s.UpdateEvery,
		AvailableVersion: s.AvailableVersion,
		NotifiedVersion:  s.NotifiedVersion,
		UpdatedTo:        s.UpdatedTo,
		TrayTipShown:     s.TrayTipShown,
	}
	if !s.LastUpdateCheck.IsZero() {
		out.LastUpdateCheck = &s.LastUpdateCheck
	}
	return json.MarshalIndent(out, "", "  ")
}

func take[T any](raw map[string]json.RawMessage, key string) (T, bool) {
	var zero T
	v, ok := raw[key]
	if !ok {
		return zero, false
	}
	var out T
	if err := json.Unmarshal(v, &out); err != nil {
		return zero, false
	}
	return out, true
}

// Never fails outright: a missing key keeps its default and a bad value falls back to its
// default too, the same tolerance TheeJ's own optional-decode gave UserDefaults.
func DecodeSettings(data []byte, defaultProfileName string) (Settings, error) {
	s := DefaultSettings(defaultProfileName)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return s, nil
	}

	if profiles, ok := take[[]Profile](raw, "profiles"); ok && len(profiles) > 0 {
		s.Profiles = profiles
		if cols, ok := take[[]*int](raw, "columns"); ok {
			s.Columns = columnsFromJSON(cols)
		}
	}

	if active, ok := take[int](raw, "profile"); ok {
		s.Active = active
	}
	if s.Active < 0 {
		s.Active = 0
	}
	if s.Active >= len(s.Profiles) {
		s.Active = len(s.Profiles) - 1
	}

	if next, ok := take[*Shortcut](raw, "nextProfile"); ok {
		s.Next = next
	}
	if previous, ok := take[*Shortcut](raw, "previousProfile"); ok {
		s.Previous = previous
	}
	if v, ok := take[bool](raw, "invertKnobs"); ok {
		s.Invert = v
	}
	if v, ok := take[bool](raw, "hideTrayIcon"); ok {
		s.HideIcon = v
	}
	if v, ok := take[bool](raw, "showProfileList"); ok {
		s.ShowProfiles = v
	}
	if v, ok := take[string](raw, "trayIcon"); ok {
		s.Icon = ParseIconStyle(v)
	}
	if v, ok := take[string](raw, "speed"); ok {
		s.Speed = ParseSpeed(v)
	}
	if v, ok := take[string](raw, "port"); ok {
		s.Port = v
	}
	if v, ok := take[int](raw, "baudRate"); ok && v > 0 {
		s.Baud = v
	}
	if v, ok := take[[]*int](raw, "mixerColumns"); ok && v != nil {
		s.MixerColumns = columnsFromJSON(v)
	}
	if v, ok := take[[]int](raw, "mixerButtonOrder"); ok && v != nil {
		s.ButtonOrder = v
	}
	if v, ok := take[map[string]string](raw, "mixerButtons"); ok {
		s.Buttons = DecodeButtons(v)
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

	return s, nil
}
