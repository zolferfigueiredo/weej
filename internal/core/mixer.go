package core

import (
	"strconv"
	"strings"
)

const midiPortPrefix = "midi:"

func MidiPort(device string) string { return midiPortPrefix + device }

func IsMidiPort(port string) bool { return strings.HasPrefix(port, midiPortPrefix) }

func MidiDevice(port string) string { return strings.TrimPrefix(port, midiPortPrefix) }

// The M-VAVE SMC-Mixer's CC mode defaults, read off a real unit: faders 1-8 send CC 40-47 and the
// rotary knobs CC 30-37, both absolute 0..127, on channel 1.
var (
	mixerFaderCCs = []int{40, 41, 42, 43, 44, 45, 46, 47}
	mixerKnobCCs  = []int{30, 31, 32, 33, 34, 35, 36, 37}
)

var MixerColumns = len(mixerFaderCCs) + len(mixerKnobCCs)

type MixerButton struct {
	CC int `json:"cc"`
	// settings.js names a button from Kind ("channel", "button", "up", "down") and N.
	Kind string `json:"kind"`
	N    int    `json:"n,omitempty"`
}

// MixerButtons lists every button the SMC-Mixer sends in CC mode. CC 50 never fired on a real unit.
var MixerButtons = func() []MixerButton {
	var out []MixerButton
	for i := 0; i < 8; i++ {
		out = append(out, MixerButton{CC: 20 + i, Kind: "channel", N: i + 1})
	}
	n := 0
	for _, cc := range []int{28, 29, 38, 39, 48, 49, 51, 52, 53, 54, 55, 56, 57, 58, 61, 62} {
		n++
		out = append(out, MixerButton{CC: cc, Kind: "button", N: n})
	}
	return append(out, MixerButton{CC: 59, Kind: "up"}, MixerButton{CC: 60, Kind: "down"})
}()

type ButtonAction string

const (
	ActionNone            ButtonAction = ""
	ActionPlayPause       ButtonAction = "media.playpause"
	ActionStop            ButtonAction = "media.stop"
	ActionPreviousTrack   ButtonAction = "media.previous"
	ActionNextTrack       ButtonAction = "media.next"
	ActionNextProfile     ButtonAction = "profile.next"
	ActionPreviousProfile ButtonAction = "profile.previous"
	ActionOpenSettings    ButtonAction = "settings"
	muteActionPrefix                   = "mute:"
)

func MuteAction(knob int) ButtonAction {
	return ButtonAction(muteActionPrefix + strconv.Itoa(knob))
}

func (a ButtonAction) MuteKnob() (int, bool) {
	rest, ok := strings.CutPrefix(string(a), muteActionPrefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 || strconv.Itoa(n) != rest {
		return 0, false
	}
	return n, true
}

func (a ButtonAction) Valid() bool {
	switch a {
	case ActionNone, ActionPlayPause, ActionStop, ActionPreviousTrack, ActionNextTrack,
		ActionNextProfile, ActionPreviousProfile, ActionOpenSettings:
		return true
	}
	_, ok := a.MuteKnob()
	return ok
}

func DefaultMixerButtons() map[int]ButtonAction {
	m := map[int]ButtonAction{59: ActionPreviousProfile, 60: ActionNextProfile}
	for i := 0; i < 8; i++ {
		m[20+i] = MuteAction(i)
	}
	return m
}

// ForMixer is the setup a mixer frame is handled with: knob i reads column i, and a fader's top is
// already its highest value, so nothing is flipped.
func (s Setup) ForMixer() Setup {
	cols := make([]int, len(s.Columns))
	for i := range cols {
		cols[i] = i
	}
	s.Columns = cols
	s.Invert = true
	return s
}

// MixerState turns MIDI short messages into a frame of 0..1023 values. A control that has not
// moved yet reads -1, since its position is unknown until the mixer sends it.
type MixerState struct {
	values []int
}

func NewMixerState() *MixerState {
	v := make([]int, MixerColumns)
	for i := range v {
		v[i] = -1
	}
	return &MixerState{values: v}
}

// Feed takes a WinMM short message (status | data1<<8 | data2<<16). It reports whether a column
// changed, and the CC of a button that was just pressed, or -1.
func (m *MixerState) Feed(msg uint32) (changed bool, pressed int) {
	status := msg & 0xFF
	if status&0xF0 != 0xB0 {
		return false, -1
	}
	cc := int(msg>>8) & 0x7F
	value := int(msg>>16) & 0x7F

	col := -1
	for i, c := range mixerFaderCCs {
		if c == cc {
			col = i
		}
	}
	for i, c := range mixerKnobCCs {
		if c == cc {
			col = len(mixerFaderCCs) + i
		}
	}
	if col >= 0 {
		scaled := (value*1023 + 63) / 127
		if m.values[col] == scaled {
			return false, -1
		}
		m.values[col] = scaled
		return true, -1
	}
	if value > 0 {
		return false, cc
	}
	return false, -1
}

func (m *MixerState) Values() []int { return append([]int(nil), m.values...) }
