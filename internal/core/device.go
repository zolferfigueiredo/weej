package core

import (
	"slices"
	"strconv"
)

// DeviceType is what a board is, which decides how WeeJ reads it.
type DeviceType string

const (
	// DeviceDIY sends lines of values over serial, as deej's Arduino sketch does.
	DeviceDIY DeviceType = "diy"
	// DeviceSMC is an M-VAVE SMC-Mixer, whose controls are fixed (smc.go).
	DeviceSMC DeviceType = "smc"
	// DeviceMIDI is any other MIDI controller, calibrated like a DIY board.
	DeviceMIDI DeviceType = "midi"
)

func ParseDeviceType(s string) (DeviceType, bool) {
	switch DeviceType(s) {
	case DeviceDIY, DeviceSMC, DeviceMIDI:
		return DeviceType(s), true
	}
	return "", false
}

func (t DeviceType) IsMIDI() bool { return t == DeviceSMC || t == DeviceMIDI }

// Control is one knob, fader or button. Input is where calibration found it, -1 until then: a
// serial column on a DIY board, a MixerState column for a MIDI knob or fader, and the button id
// (MixerNoteButton) for a MIDI button. Min and Max are the raw ends of a pot's travel, 0 to 1023,
// and Reverse puts its 0% at Max.
type Control struct {
	Kind    ControlKind
	Input   int
	Reverse bool
	Min     int
	Max     int
}

// DeviceProfile is what a board's controls do in one of its profiles. Jobs go by control index,
// and Buttons by control index on a DIY board and by button id on a MIDI one, which is what each
// reports when a button is pressed.
type DeviceProfile struct {
	Name     string
	Shortcut *Shortcut
	Jobs     [][]Job
	Buttons  ButtonMap
}

// Device is one board with everything that belongs to it. ID never changes and is never given to
// another board, so whatever is kept by it (the faders' positions) can't land on the wrong one.
type Device struct {
	ID      string
	Name    string
	Type    DeviceType
	Enabled bool
	// Port is a COM port on a DIY board, "" to find it automatically, and the input's name on a
	// MIDI one.
	Port  string
	Baud  int
	Speed Speed
	// Invert flips every control, on top of each one's Reverse.
	Invert   bool
	Controls []Control
	// Layout is how Settings draws a DIY or Other MIDI board: rows of control indices.
	Layout   [][]int
	View     string
	Profiles []DeviceProfile
	Active   int
	Next     *Shortcut
	Previous *Shortcut
	// Lights is an SMC-Mixer's button light pattern (lights.go), "" for none.
	Lights string
}

const (
	ViewDraw = "draw"
	ViewList = "list"
)

// NewDevice makes a board with one profile and its knobs, faders and buttons still to find. An
// SMC-Mixer's controls are fixed, so it takes no counts.
func NewDevice(id, name string, t DeviceType, knobs, faders, buttons int, profileName string) Device {
	d := Device{ID: id, Name: name, Type: t, Enabled: true, Speed: SpeedSlow, View: ViewDraw}
	if t == DeviceSMC {
		d.Controls = SMCControls()
	} else {
		add := func(kind ControlKind, n int) {
			for range n {
				d.Controls = append(d.Controls, Control{Kind: kind, Input: -1, Max: 1023})
			}
		}
		add(KindKnob, knobs)
		add(KindFader, faders)
		add(KindButton, buttons)
		d.Layout = DefaultLayout(d.Controls)
	}
	d.Profiles = []DeviceProfile{d.NewProfile(profileName)}
	return d
}

// NewProfile is an empty profile for the board, with an SMC-Mixer's usual buttons.
func (d Device) NewProfile(name string) DeviceProfile {
	p := DeviceProfile{Name: name, Jobs: make([][]Job, len(d.Controls)), Buttons: ButtonMap{}}
	for i := range p.Jobs {
		p.Jobs[i] = []Job{}
	}
	if d.Type == DeviceSMC {
		p.Buttons = DefaultMixerButtons()
	}
	return p
}

// SMCControls is an SMC-Mixer's faders, then its knobs, as smcColumns reads them.
func SMCControls() []Control {
	out := make([]Control, len(smcColumns))
	for i, col := range smcColumns {
		kind := KindFader
		if i >= 8 {
			kind = KindKnob
		}
		out[i] = Control{Kind: kind, Input: col, Max: 1023}
	}
	return out
}

// DefaultLayout draws the knobs on one row, the faders on the next and the buttons on the last.
func DefaultLayout(controls []Control) [][]int {
	var layout [][]int
	for _, kind := range []ControlKind{KindKnob, KindFader, KindButton} {
		var row []int
		for i, c := range controls {
			if c.Kind == kind {
				row = append(row, i)
			}
		}
		if row != nil {
			layout = append(layout, row)
		}
	}
	return layout
}

// NextDeviceID is the ID the next board gets, given the count of boards ever added.
func NextDeviceID(added int) string { return "d" + strconv.Itoa(added+1) }

func (d Device) BaudRate() int {
	if d.Baud <= 0 {
		return DefaultBaud
	}
	return d.Baud
}

func (d Device) ActiveProfile() DeviceProfile {
	switch {
	case d.Active >= 0 && d.Active < len(d.Profiles):
		return d.Profiles[d.Active]
	case len(d.Profiles) > 0:
		return d.Profiles[0]
	}
	return DeviceProfile{}
}

func (d Device) ActiveButtons() ButtonMap { return d.ActiveProfile().Buttons }

// Stepped is the profile by steps from the active one, round the end either way.
func (d Device) Stepped(by int) int {
	n := len(d.Profiles)
	if n == 0 {
		return 0
	}
	return ((d.Active+by)%n + n) % n
}

func (d Device) IsPot(i int) bool {
	return i >= 0 && i < len(d.Controls) && d.Controls[i].Kind != KindButton
}

// ButtonInputs is a DIY board's buttons that have an input: control index to its serial column.
func (d Device) ButtonInputs() map[int]int {
	out := map[int]int{}
	if d.Type != DeviceDIY {
		return out
	}
	for i, c := range d.Controls {
		if c.Kind == KindButton && c.Input >= 0 {
			out[i] = c.Input
		}
	}
	return out
}

// ButtonKeys lists the board's buttons as its profiles key them, in the order Settings shows them.
func (d Device) ButtonKeys() []int {
	if d.Type == DeviceSMC {
		return SMCButtonOrder()
	}
	var keys []int
	for i, c := range d.Controls {
		if c.Kind != KindButton {
			continue
		}
		switch {
		case d.Type == DeviceDIY:
			keys = append(keys, i)
		case c.Input >= 0:
			keys = append(keys, c.Input)
		}
	}
	return keys
}

// Calibrated tells whether every control has been found.
func (d Device) Calibrated() bool {
	return !slices.ContainsFunc(d.Controls, func(c Control) bool { return c.Input < 0 })
}

// potDeadZone is how much of each end of a pot's travel counts as that end, so a worn pot still
// reaches 0% and 100%; a fraction of its range, in thousandths.
const potDeadZone = 20

// Normalize turns a raw frame into one value per control, 0 for 0% to 1023 for 100%. Buttons,
// controls not found yet and inputs that haven't reported read -1. Invert is left to the Engine
// (EngineSetup), which keeps a flip from jumping the volume.
func (d Device) Normalize(raw []int) []int {
	out := make([]int, len(d.Controls))
	for i, c := range d.Controls {
		out[i] = -1
		if c.Kind == KindButton || c.Input < 0 || c.Input >= len(raw) || raw[c.Input] < 0 {
			continue
		}
		lo, hi := c.Min, c.Max
		if hi <= lo {
			lo, hi = 0, 1023
		}
		dead := (hi - lo) * potDeadZone / 1000
		lo, hi = lo+dead, hi-dead
		v := (raw[c.Input] - lo) * 1023 / max(hi-lo, 1)
		v = min(max(v, 0), 1023)
		if c.Reverse {
			v = 1023 - v
		}
		out[i] = v
	}
	return out
}

// Shown is Normalize with Invert applied, so Settings draws each control the way its volume
// moves.
func (d Device) Shown(raw []int) []int {
	out := d.Normalize(raw)
	if d.Invert {
		for i, v := range out {
			if v >= 0 {
				out[i] = 1023 - v
			}
		}
	}
	return out
}

// EngineSetup is the board as the Engine reads it: frames from Normalize, so each control is its
// own column, buttons have no jobs, and Invert is the Engine's own, which flips unless set.
func (d Device) EngineSetup() Setup {
	cols := make([]int, len(d.Controls))
	for i, c := range d.Controls {
		cols[i] = i
		if c.Kind == KindButton || c.Input < 0 {
			cols[i] = -1
		}
	}
	profiles := make([]Profile, len(d.Profiles))
	for i, p := range d.Profiles {
		profiles[i] = Profile{Name: p.Name, Jobs: p.Jobs}
	}
	return Setup{Columns: cols, Profiles: profiles, Active: d.Active, Speed: d.Speed, Invert: !d.Invert}
}

// Apps lists the apps every profile of the board turns up and down.
func (d Device) Apps() []string {
	var out []string
	for _, p := range d.Profiles {
		for _, row := range p.Jobs {
			for _, j := range row {
				if j.Kind == JobApp && !slices.Contains(out, j.Exe) {
					out = append(out, j.Exe)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}
