package core

import (
	"encoding/json"
	"slices"
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
			Profiles:     []Profile{{Name: defaultProfileName, Buttons: DefaultMixerButtons()}},
			ShowProfiles: true,
			Icon:         IconMixer,
			Speed:        SpeedSlow,
		},
		ShowDataInMenu: true,
		UpdateEvery:    604800,
	}
}

// The on-disk shape, key order matching the spec exactly so EncodeSettings is deterministic.
type settingsJSON struct {
	Columns          []*int     `json:"columns"`
	Profiles         []Profile  `json:"profiles"`
	Profile          int        `json:"profile"`
	NextProfile      *Shortcut  `json:"nextProfile"`
	PreviousProfile  *Shortcut  `json:"previousProfile"`
	InvertKnobs      bool       `json:"invertKnobs"`
	InvertMixer      bool       `json:"invertMixer"`
	HideTrayIcon     bool       `json:"hideTrayIcon"`
	ShowProfileList  bool       `json:"showProfileList"`
	TrayIcon         string     `json:"trayIcon"`
	Speed            string     `json:"speed"`
	Port             string     `json:"port"`
	MixerPort        string     `json:"mixerPort"`
	BaudRate         int        `json:"baudRate"`
	BoardKinds       []string   `json:"boardKinds,omitempty"`
	BoardLayout      [][]int    `json:"boardLayout,omitempty"`
	MixerColumns     []*int     `json:"mixerColumns"`
	MixerButtonOrder []int      `json:"mixerButtonOrder"`
	Language         string     `json:"language"`
	ShowDataInMenu   bool       `json:"showDataInMenu"`
	UpdateEvery      int        `json:"updateEvery"`
	LastUpdateCheck  *time.Time `json:"lastUpdateCheck,omitempty"`
	AvailableVersion string     `json:"availableVersion"`
	NotifiedVersion  string     `json:"notifiedVersion"`
	UpdatedTo        string     `json:"updatedTo"`
	TrayTipShown     bool       `json:"trayTipShown"`
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

func EncodeKinds(kinds []ControlKind) []string {
	if kinds == nil {
		return nil
	}
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

func DecodeKinds(kinds []string) []ControlKind {
	if kinds == nil {
		return nil
	}
	out := make([]ControlKind, len(kinds))
	for i, k := range kinds {
		out[i] = ParseControlKind(k)
	}
	return out
}

// ButtonMap is a profile's button functions by button id; a button may do several things at once.
// Saved as an object of id to list of actions. A bad entry in a saved file is dropped on its own
// instead of failing the whole profile list, and a single action, as files before lists held,
// still reads.
type ButtonMap map[int][]ButtonAction

func (m ButtonMap) MarshalJSON() ([]byte, error) {
	out := map[string][]ButtonAction{}
	for id, actions := range m {
		if len(actions) > 0 {
			out[strconv.Itoa(id)] = actions
		}
	}
	return json.Marshal(out)
}

func (m *ButtonMap) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		*m = ButtonMap{}
		return nil
	}
	out := ButtonMap{}
	for k, v := range raw {
		var list []ButtonAction
		if err := json.Unmarshal(v, &list); err != nil {
			var one ButtonAction
			if json.Unmarshal(v, &one) != nil {
				continue
			}
			list = []ButtonAction{one}
		}
		out.set(k, list)
	}
	*m = out
	return nil
}

func (m ButtonMap) set(key string, actions []ButtonAction) {
	id, err := strconv.Atoi(key)
	if err != nil || id < 0 || id > 255 {
		return
	}
	var kept []ButtonAction
	for _, a := range actions {
		if a != ActionNone && a.Valid() && !slices.Contains(kept, a) {
			kept = append(kept, a)
		}
	}
	if len(kept) > 0 {
		m[id] = kept
	}
}

// DecodeButtons reads the one shared set of button functions that came before lists and profiles.
func DecodeButtons(raw map[string]string) ButtonMap {
	out := ButtonMap{}
	for k, v := range raw {
		out.set(k, []ButtonAction{ButtonAction(v)})
	}
	return out
}

func normalizeProfile(p Profile) Profile {
	p.Jobs = normalizeJobs(p.Jobs)
	p.MixerJobs = normalizeJobs(p.MixerJobs)
	return p
}

func normalizeJobs(jobs [][]Job) [][]Job {
	if jobs == nil {
		jobs = [][]Job{}
	}
	for i, row := range jobs {
		if row == nil {
			jobs[i] = []Job{}
		}
	}
	return jobs
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
		InvertMixer:      s.MixerInvert,
		HideTrayIcon:     s.HideIcon,
		ShowProfileList:  s.ShowProfiles,
		TrayIcon:         string(s.Icon),
		Speed:            string(s.Speed),
		Port:             s.Port,
		MixerPort:        s.MixerPort,
		BaudRate:         s.BaudRate(),
		BoardKinds:       EncodeKinds(s.BoardKinds),
		BoardLayout:      s.BoardLayout,
		MixerColumns:     EncodeMixerColumns(s.MixerColumns),
		MixerButtonOrder: s.ButtonOrder,
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
	if v, ok := take[bool](raw, "invertMixer"); ok {
		s.MixerInvert = v
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
	if v, ok := take[string](raw, "mixerPort"); ok {
		s.MixerPort = v
	}
	// The mixer was read in the board's place before both could run at once.
	if IsMidiPort(s.Port) {
		s.MixerPort, s.Port = MidiDevice(s.Port), PortOff
	}
	if v, ok := take[int](raw, "baudRate"); ok && v > 0 {
		s.Baud = v
	}
	if v, ok := take[[]string](raw, "boardKinds"); ok {
		s.BoardKinds = DecodeKinds(v)
	}
	if v, ok := take[[][]int](raw, "boardLayout"); ok {
		s.BoardLayout = CleanLayout(v, len(s.Columns))
	}
	if v, ok := take[[]*int](raw, "mixerColumns"); ok && v != nil {
		s.MixerColumns = columnsFromJSON(v)
	}
	if v, ok := take[[]int](raw, "mixerButtonOrder"); ok && v != nil {
		s.ButtonOrder = v
	}
	// Button functions were one set for every profile before they moved into the profile, and
	// both devices shared the knob jobs before each got its own.
	legacy, hasLegacy := take[map[string]string](raw, "mixerButtons")
	for i := range s.Profiles {
		if s.Profiles[i].MixerJobs == nil {
			s.Profiles[i].MixerJobs = make([][]Job, len(s.Profiles[i].Jobs))
			for k, row := range s.Profiles[i].Jobs {
				s.Profiles[i].MixerJobs[k] = append([]Job{}, row...)
			}
		}
		switch {
		case s.Profiles[i].Buttons != nil:
		case hasLegacy:
			s.Profiles[i].Buttons = DecodeButtons(legacy)
		default:
			s.Profiles[i].Buttons = DefaultMixerButtons()
		}
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
	MigrateSMC(&s.Setup)

	return s, nil
}
