package core

import (
	"slices"
	"strconv"
	"strings"
)

const midiPortPrefix = "midi:"

func MidiPort(device string) string { return midiPortPrefix + device }

func IsMidiPort(port string) bool { return strings.HasPrefix(port, midiPortPrefix) }

func MidiDevice(port string) string { return strings.TrimPrefix(port, midiPortPrefix) }

// A mixer frame has one column per CC number, so a control's column is its CC and any number of
// controls fits. On the M-VAVE SMC-Mixer in CC mode, read off a real unit, faders 1-8 send CC
// 40-47 and the rotary knobs CC 30-37, both absolute 0..127, on channel 1.
const MixerColumns = 128

var defaultMixerControls = []int{40, 41, 42, 43, 44, 45, 46, 47, 30, 31, 32, 33, 34, 35, 36, 37}

// DefaultMixerButtonOrder is every button CC the SMC-Mixer sends in CC mode, in the order
// Settings lists them until Calibrate sets an order. CC 50 never fired on a real unit.
var DefaultMixerButtonOrder = []int{
	20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 38, 39, 48, 49,
	51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62,
}

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

// ForMixer is the setup a mixer frame is handled with: Columns come from MixerColumns, so the
// mixer has its own knob count, and a fader's top is already its highest value, so nothing is
// flipped. A mixer never calibrated reads its faders, then its knobs.
func (s Setup) ForMixer() Setup {
	if s.MixerColumns == nil {
		cols := make([]int, len(s.Columns))
		for i := range cols {
			cols[i] = -1
			if i < len(defaultMixerControls) {
				cols[i] = defaultMixerControls[i]
			}
		}
		s.Columns = cols
	} else {
		s.Columns = append([]int{}, s.MixerColumns...)
	}
	s.Invert = true
	return s
}

// MixerButtonOrder is the order Settings lists the mixer's buttons in: Calibrate's, or the default.
func (s Setup) MixerButtonOrder() []int {
	if s.ButtonOrder != nil {
		return s.ButtonOrder
	}
	return DefaultMixerButtonOrder
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

	if slices.Contains(DefaultMixerButtonOrder, cc) {
		if value > 0 {
			return false, cc
		}
		return false, -1
	}
	scaled := (value*1023 + 63) / 127
	if m.values[cc] == scaled {
		return false, -1
	}
	m.values[cc] = scaled
	return true, -1
}

func (m *MixerState) Values() []int { return append([]int(nil), m.values...) }

// MoveWatcher tells which columns moved far enough to be a hand on a control rather than a pot's
// jitter. A column that reads -1 (a mixer control not heard from yet) counts as moved once it
// reports.
type MoveWatcher struct {
	anchor []int
}

const moveThreshold = 12

func (w *MoveWatcher) Moved(values []int) []int {
	if len(w.anchor) != len(values) {
		w.anchor = append([]int{}, values...)
		return nil
	}
	var moved []int
	for col, v := range values {
		a := w.anchor[col]
		if v < 0 || (a >= 0 && absInt(v-a) < moveThreshold) {
			continue
		}
		w.anchor[col] = v
		moved = append(moved, col)
	}
	return moved
}
