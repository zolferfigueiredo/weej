package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type JobKind string

const (
	JobMaster            JobKind = "master"
	JobMicrophone        JobKind = "microphone"
	JobSystemSounds      JobKind = "systemSounds"
	JobBuiltinBrightness JobKind = "builtinBrightness"
	JobBrightness        JobKind = "brightness"
	JobContrast          JobKind = "contrast"
	JobNightLight        JobKind = "nightLight"
	JobExternalKeyboard  JobKind = "externalKeyboard"
	JobZoom              JobKind = "zoom"
	JobApp               JobKind = "app"
	JobFocusedApp        JobKind = "focusedApp"
	JobOtherApps         JobKind = "otherApps"
)

type Job struct {
	Kind   JobKind
	Screen int
	Exe    string
}

func (j Job) Immediate() bool {
	switch j.Kind {
	case JobMaster, JobMicrophone, JobSystemSounds, JobApp, JobFocusedApp, JobOtherApps:
		return true
	default:
		return false
	}
}

// Mirrors TheeJ's rank(): 100 per section, screens ordered within theirs. Apps sit in their own
// band after focusedApp and otherApps so every job kind keeps a distinct rank.
func (j Job) Rank() int {
	switch j.Kind {
	case JobMaster:
		return 0
	case JobMicrophone:
		return 1
	case JobSystemSounds:
		return 2
	case JobBuiltinBrightness:
		return 100
	case JobBrightness:
		return 101 + j.Screen
	case JobContrast:
		return 200 + j.Screen
	case JobNightLight:
		return 300
	case JobExternalKeyboard:
		return 400
	case JobZoom:
		return 500
	case JobFocusedApp:
		return 600
	case JobOtherApps:
		return 601
	case JobApp:
		return 602
	default:
		return -1
	}
}

func (j Job) Section() Section { return Section(j.Rank() / 100) }

type Section int

const (
	SectionVolume Section = iota
	SectionBrightness
	SectionContrast
	SectionNightLight
	SectionKeyboardBacklight
	SectionZoom
	SectionApps
)

func (s Section) Key() string {
	switch s {
	case SectionVolume:
		return "section.volume"
	case SectionBrightness:
		return "section.brightness"
	case SectionContrast:
		return "section.contrast"
	case SectionNightLight:
		return "section.night_light"
	case SectionKeyboardBacklight:
		return "section.keyboard"
	case SectionZoom:
		return "section.zoom"
	case SectionApps:
		return "section.apps"
	default:
		return ""
	}
}

type Translate func(key string, vars map[string]string) string

// For an app job the title is the caller's display name, falling back to the exe itself: core
// never imports internal/lang, so it has no app names or catalog of its own.
func (j Job) Title(tr Translate, appName func(exe string) string) string {
	switch j.Kind {
	case JobMaster:
		return tr("job.master", nil)
	case JobMicrophone:
		return tr("job.microphone", nil)
	case JobSystemSounds:
		return tr("job.system_sounds", nil)
	case JobBuiltinBrightness:
		return tr("job.builtin_brightness", nil)
	case JobBrightness:
		return tr("job.brightness", map[string]string{"n": strconv.Itoa(j.Screen + 1)})
	case JobContrast:
		return tr("job.contrast", map[string]string{"n": strconv.Itoa(j.Screen + 1)})
	case JobNightLight:
		return tr("job.night_light", nil)
	case JobExternalKeyboard:
		return tr("job.external_keyboard", nil)
	case JobZoom:
		return tr("job.zoom", nil)
	case JobFocusedApp:
		return tr("job.focused_app", nil)
	case JobOtherApps:
		return tr("job.other_apps", nil)
	case JobApp:
		if appName != nil {
			if name := appName(j.Exe); name != "" {
				return name
			}
		}
		return j.Exe
	default:
		return ""
	}
}

func (j Job) ShortTitle(tr Translate, appName func(exe string) string) string {
	switch j.Kind {
	case JobBuiltinBrightness:
		return tr("short.builtin_display", nil)
	case JobBrightness, JobContrast:
		return tr("short.screen", map[string]string{"n": strconv.Itoa(j.Screen + 1)})
	case JobNightLight:
		return tr("short.warmth", nil)
	case JobExternalKeyboard:
		return tr("short.external", nil)
	default:
		return j.Title(tr, appName)
	}
}

func (j Job) MarshalJSON() ([]byte, error) {
	switch j.Kind {
	case JobBrightness, JobContrast:
		return json.Marshal(struct {
			Kind   JobKind `json:"kind"`
			Screen int     `json:"screen"`
		}{j.Kind, j.Screen})
	case JobApp:
		return json.Marshal(struct {
			Kind JobKind `json:"kind"`
			Exe  string  `json:"exe"`
		}{j.Kind, j.Exe})
	default:
		return json.Marshal(struct {
			Kind JobKind `json:"kind"`
		}{j.Kind})
	}
}

func (j *Job) UnmarshalJSON(data []byte) error {
	var probe struct {
		Kind   JobKind `json:"kind"`
		Screen *int    `json:"screen"`
		Exe    *string `json:"exe"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	switch probe.Kind {
	case JobMaster, JobMicrophone, JobSystemSounds, JobBuiltinBrightness, JobNightLight,
		JobExternalKeyboard, JobZoom, JobFocusedApp, JobOtherApps:
		*j = Job{Kind: probe.Kind}
	case JobBrightness, JobContrast:
		if probe.Screen == nil {
			return fmt.Errorf("core: job %q missing screen", probe.Kind)
		}
		*j = Job{Kind: probe.Kind, Screen: *probe.Screen}
	case JobApp:
		if probe.Exe == nil {
			return fmt.Errorf("core: job %q missing exe", probe.Kind)
		}
		*j = Job{Kind: probe.Kind, Exe: strings.ToLower(*probe.Exe)}
	default:
		return fmt.Errorf("core: unknown job kind %q", probe.Kind)
	}
	return nil
}

const (
	ModAlt     uint16 = 1
	ModControl uint16 = 2
	ModShift   uint16 = 4
	ModWin     uint16 = 8
)

type Shortcut struct {
	VK   uint16 `json:"vk"`
	Mods uint16 `json:"mods"`
	Key  string `json:"key"`
}

type Profile struct {
	Name string `json:"name"`
	// Jobs are the board's knobs and MixerJobs the mixer's: each device has its own knobs.
	// Nil MixerJobs means a profile saved before that split; DecodeSettings fills it in.
	Jobs      [][]Job   `json:"jobs"`
	MixerJobs [][]Job   `json:"mixerJobs"`
	Shortcut  *Shortcut `json:"shortcut,omitempty"`
	// Buttons is what each mixer button does in this profile. Nil means a profile saved before
	// buttons were per profile; DecodeSettings fills it in.
	Buttons ButtonMap `json:"buttons"`
	// BoardButtons is what each of the board's buttons does in this profile, by knob index.
	BoardButtons ButtonMap `json:"boardButtons,omitempty"`
}

// AllJobs is every job the profile uses, on either device.
func (p Profile) AllJobs() [][]Job { return append(append([][]Job{}, p.Jobs...), p.MixerJobs...) }

func (p Profile) JobsOf(knob int) []Job {
	if knob < 0 || knob >= len(p.Jobs) {
		return nil
	}
	return p.Jobs[knob]
}

type Speed string

const (
	SpeedSlow      Speed = "slow"
	SpeedMedium    Speed = "medium"
	SpeedFast      Speed = "fast"
	SpeedSuperFast Speed = "superFast"
)

// Settle must stay above the longest wiper dropout TheeJ observed (~110ms) or a dropout would
// reach the panel as a flash; an unrecognised speed falls back to slow, same as TheeJ.
func (s Speed) Settle() time.Duration {
	switch s {
	case SpeedMedium:
		return 220 * time.Millisecond
	case SpeedFast:
		return 180 * time.Millisecond
	case SpeedSuperFast:
		return 150 * time.Millisecond
	default:
		return 300 * time.Millisecond
	}
}

func ParseSpeed(s string) Speed {
	switch Speed(s) {
	case SpeedMedium, SpeedFast, SpeedSuperFast:
		return Speed(s)
	default:
		return SpeedSlow
	}
}

type IconStyle string

const (
	IconMixer IconStyle = "mixer"
	IconDial  IconStyle = "dial"
	IconApp   IconStyle = "app"
)

func ParseIconStyle(s string) IconStyle {
	switch IconStyle(s) {
	case IconDial, IconApp:
		return IconStyle(s)
	default:
		return IconMixer
	}
}

// Letter names knob i as on the box: A to Z, then A2 to Z2, A3 and so on, with no limit.
func Letter(i int) string {
	s := string(rune('A' + i%26))
	if i >= 26 {
		s += strconv.Itoa(i/26 + 1)
	}
	return s
}

type Setup struct {
	Columns  []int
	Profiles []Profile
	Active   int
	Next     *Shortcut
	Previous *Shortcut
	// Invert is the board's: its pots are often wired the other way round, so false flips them.
	// MixerInvert is the mixer's own: false leaves a fader's top at its highest value.
	Invert       bool
	MixerInvert  bool
	HideIcon     bool
	ShowProfiles bool
	Icon         IconStyle
	Speed        Speed
	// Port is a COM port or a MidiPort to use instead of finding the board automatically; empty
	// means automatic, which only ever looks for serial boards.
	Port string
	// Baud is the serial speed; 0 means DefaultBaud.
	Baud int
	// MixerColumns is the mixer's own calibration, so the board's Columns survive a switch to
	// the mixer and back. Nil means never calibrated. An SMC-Mixer has neither: its controls are
	// fixed (smc.go).
	MixerColumns []int
	// ButtonOrder is the mixer buttons Calibrate found, by id: Button 1 first. Nil means never.
	ButtonOrder []int
	// BoardKinds is what each of the board's knobs is (board.go), and BoardLayout how Settings
	// draws them: rows of knob indices, left to right. Nil draws one row of knobs.
	BoardKinds  []ControlKind
	BoardLayout [][]int
}

func (s Setup) activeJobs() [][]Job {
	if s.Active < 0 || s.Active >= len(s.Profiles) {
		if len(s.Profiles) == 0 {
			return nil
		}
		return s.Profiles[0].Jobs
	}
	return s.Profiles[s.Active].Jobs
}

// A loop, not a map literal, so a later knob on a shared column wins, same as TheeJ's targets().
func mapping(columns []int, jobs [][]Job) map[int][]Job {
	result := map[int][]Job{}
	n := len(columns)
	if len(jobs) < n {
		n = len(jobs)
	}
	for i := 0; i < n; i++ {
		if columns[i] == -1 || len(jobs[i]) == 0 {
			continue
		}
		result[columns[i]] = jobs[i]
	}
	return result
}

// Mapping leaves the board's buttons out: a button only presses, whatever jobs it kept.
func (s Setup) Mapping() map[int][]Job {
	jobs := s.activeJobs()
	if s.BoardKinds != nil {
		jobs = append([][]Job(nil), jobs...)
		for i := range jobs {
			if s.Kind(i) == KindButton {
				jobs[i] = nil
			}
		}
	}
	return mapping(s.Columns, jobs)
}

func (s Setup) ActiveButtons() ButtonMap {
	switch {
	case s.Active >= 0 && s.Active < len(s.Profiles):
		return s.Profiles[s.Active].Buttons
	case len(s.Profiles) > 0:
		return s.Profiles[0].Buttons
	}
	return nil
}

func lastIndexOf(columns []int, col int) int {
	for i := len(columns) - 1; i >= 0; i-- {
		if columns[i] == col {
			return i
		}
	}
	return 0
}

// The order status lines and Settings follow: knob order (A, B, C...), not the serial column order.
func (s Setup) MenuOrder() []int {
	m := s.Mapping()
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		return lastIndexOf(s.Columns, keys[a]) < lastIndexOf(s.Columns, keys[b])
	})
	return keys
}

func (s Setup) Apps() []string {
	set := map[string]struct{}{}
	for _, p := range s.Profiles {
		for _, row := range p.AllJobs() {
			for _, j := range row {
				if j.Kind == JobApp {
					set[j.Exe] = struct{}{}
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for e := range set {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

func (s Setup) Stepped(by int) int {
	n := len(s.Profiles)
	if n == 0 {
		return 0
	}
	return ((s.Active+by)%n + n) % n
}

func Percent(s float64) int {
	v := int(s*100 + 0.5)
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func Clipped(name string, limit int) string {
	r := []rune(name)
	if len(r) <= limit {
		return name
	}
	return strings.TrimRight(string(r[:limit-1]), " \t\n") + "…"
}

// DefaultBaud is what deej's sketch (and so most boards) passes to Serial.begin().
const DefaultBaud = 9600

// BaudRates are the speeds offered in Settings; a board's sketch has to use the same one.
var BaudRates = []int{9600, 19200, 38400, 57600, 115200}

func (s Setup) BaudRate() int {
	if s.Baud <= 0 {
		return DefaultBaud
	}
	return s.Baud
}
