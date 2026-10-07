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

// Profile is a board's jobs by column, as the Engine reads them (Device.EngineSetup).
type Profile struct {
	Name string
	Jobs [][]Job
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

// Letter names knob i as on the box: A to Z, then A2 to Z2, A3 and so on, with no limit.
func Letter(i int) string {
	s := string(rune('A' + i%26))
	if i >= 26 {
		s += strconv.Itoa(i/26 + 1)
	}
	return s
}

// Setup is one board as the Engine reads it (Device.EngineSetup): a column per control, -1 for
// one with no jobs to run, and Invert the Engine's own, which flips unless set.
type Setup struct {
	Columns  []int
	Profiles []Profile
	Active   int
	Speed    Speed
	Invert   bool
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

func (s Setup) Mapping() map[int][]Job { return mapping(s.Columns, s.activeJobs()) }

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
