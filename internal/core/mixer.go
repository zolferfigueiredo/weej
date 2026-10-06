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
// controls fits, then one per pitch-bend channel. Read off a real M-VAVE SMC-Mixer on channel 1:
//   - CC mode: faders 1-8 send CC 40-47 and the rotary knobs CC 30-37, absolute 0..127, and
//     buttons send CC 127 on press and 0 on release.
//   - DAW (Mackie) mode: faders send pitch bend on channels 1-8, the rotary knobs CC 16-23 as
//     steps (1 up, 65 down), and buttons notes: R 0-7, S 8-15, M 16-23, Square 24-31, the
//     bottom row 46, 47 and 91-99.
//
// DAW mode's faders and knobs land on the CC-mode columns of the same control, so one
// calibration covers both modes.
const MixerColumns = 128 + 16

const (
	pitchBendColumn = 128
	vpotFirstCC     = 16
	vpotCount       = 8
	knobFirstCC     = 30
	faderFirstCC    = 40
	// A rotary knob in DAW mode moves this much per step, about 150 steps to half a turn.
	vpotStep = 4
)

var defaultMixerControls = []int{40, 41, 42, 43, 44, 45, 46, 47, 30, 31, 32, 33, 34, 35, 36, 37}

// MixerNoteButton is a DAW-mode button's id: buttons share one id space, CCs below 128 and notes
// above.
func MixerNoteButton(note int) int { return 128 + note }

// MixerButtonNote reports the note behind a DAW-mode button id.
func MixerButtonNote(id int) (int, bool) { return id - 128, id >= 128 }

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
// changed, and the id of a button that was just pressed, or -1.
func (m *MixerState) Feed(msg uint32) (changed bool, pressed int) {
	status := int(msg & 0xFF)
	data1 := int(msg>>8) & 0x7F
	data2 := int(msg>>16) & 0x7F

	switch status & 0xF0 {
	case 0x90:
		if data2 > 0 {
			return false, MixerNoteButton(data1)
		}
		return false, -1
	case 0xE0:
		channel := status & 0x0F
		col := pitchBendColumn + channel
		if channel < 8 {
			col = faderFirstCC + channel
		}
		// The SMC-Mixer only sends the top 7 bits, so its fader tops out at 127<<7, not 16383.
		return m.set(col, min(((data1|data2<<7)*1023+8128)/16256, 1023)), -1
	case 0xB0:
	default:
		return false, -1
	}

	cc, value := data1, data2
	// A step is never 0 or 127, and a CC-mode button on the same number never sends anything else.
	if cc >= vpotFirstCC && cc < vpotFirstCC+vpotCount && value != 0 && value != 127 {
		col := knobFirstCC + cc - vpotFirstCC
		pos := m.values[col]
		if pos < 0 {
			pos = 512
		}
		step := value & 0x3F
		if value&0x40 != 0 {
			step = -step
		}
		return m.set(col, min(max(pos+step*vpotStep, 0), 1023)), -1
	}
	if slices.Contains(DefaultMixerButtonOrder, cc) {
		if value > 0 {
			return false, cc
		}
		return false, -1
	}
	return m.set(cc, (value*1023+63)/127), -1
}

func (m *MixerState) set(col, value int) bool {
	if m.values[col] == value {
		return false
	}
	m.values[col] = value
	return true
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
