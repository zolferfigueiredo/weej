package core

import (
	"fmt"
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

type ButtonAction string

const (
	ActionNone            ButtonAction = ""
	ActionPlayPause       ButtonAction = "media.playpause"
	ActionPlay            ButtonAction = "media.play"
	ActionPause           ButtonAction = "media.pause"
	ActionStop            ButtonAction = "media.stop"
	ActionPreviousTrack   ButtonAction = "media.previous"
	ActionNextTrack       ButtonAction = "media.next"
	ActionVolumeUp        ButtonAction = "volume.up"
	ActionVolumeDown      ButtonAction = "volume.down"
	ActionMuteAll         ButtonAction = "mute.all"
	ActionMuteMic         ButtonAction = "mute.mic"
	ActionNightLight      ButtonAction = "nightlight"
	ActionScreensOff      ButtonAction = "screens.off"
	ActionLockPC          ButtonAction = "pc.lock"
	ActionSleepPC         ButtonAction = "pc.sleep"
	ActionNextProfile     ButtonAction = "profile.next"
	ActionPreviousProfile ButtonAction = "profile.previous"
	ActionOpenSettings    ButtonAction = "settings"

	// These carry a setting after the prefix; settings.js keys its controls off the same prefixes.
	muteActionPrefix    = "mute:"
	openActionPrefix    = "open:"
	closeActionPrefix   = "close:"
	urlActionPrefix     = "url:"
	keysActionPrefix    = "keys:"
	profileActionPrefix = "profile:"
)

func MuteAction(knob int) ButtonAction {
	return ButtonAction(muteActionPrefix + strconv.Itoa(knob))
}

func OpenAppAction(path string) ButtonAction { return ButtonAction(openActionPrefix + path) }

func CloseAppAction(exe string) ButtonAction { return ButtonAction(closeActionPrefix + exe) }

func URLAction(url string) ButtonAction { return ButtonAction(urlActionPrefix + url) }

// KeysAction is "keys:<mods>:<vk>:<key>"; the key name goes last since it may hold a colon.
func KeysAction(s Shortcut) ButtonAction {
	return ButtonAction(fmt.Sprintf("%s%d:%d:%s", keysActionPrefix, s.Mods, s.VK, s.Key))
}

func ProfileAction(i int) ButtonAction {
	return ButtonAction(profileActionPrefix + strconv.Itoa(i))
}

func (a ButtonAction) param(prefix string) (string, bool) {
	rest, ok := strings.CutPrefix(string(a), prefix)
	return rest, ok && rest != ""
}

func index(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || strconv.Itoa(n) != s {
		return 0, false
	}
	return n, true
}

func (a ButtonAction) MuteKnob() (int, bool) {
	rest, ok := a.param(muteActionPrefix)
	if !ok {
		return 0, false
	}
	return index(rest)
}

// OpenApp is the full path of the app to start.
func (a ButtonAction) OpenApp() (string, bool) { return a.param(openActionPrefix) }

// CloseApp is the file name of the app to close, such as "spotify.exe".
func (a ButtonAction) CloseApp() (string, bool) { return a.param(closeActionPrefix) }

func (a ButtonAction) URL() (string, bool) {
	u, ok := a.param(urlActionPrefix)
	lower := strings.ToLower(u)
	return u, ok && (strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://"))
}

func (a ButtonAction) Keys() (Shortcut, bool) {
	rest, ok := a.param(keysActionPrefix)
	if !ok {
		return Shortcut{}, false
	}
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) != 3 {
		return Shortcut{}, false
	}
	mods, err1 := strconv.ParseUint(parts[0], 10, 16)
	vk, err2 := strconv.ParseUint(parts[1], 10, 16)
	if err1 != nil || err2 != nil || vk == 0 {
		return Shortcut{}, false
	}
	return Shortcut{VK: uint16(vk), Mods: uint16(mods), Key: parts[2]}, true
}

func (a ButtonAction) Profile() (int, bool) {
	rest, ok := a.param(profileActionPrefix)
	if !ok {
		return 0, false
	}
	return index(rest)
}

func (a ButtonAction) Valid() bool {
	switch a {
	case ActionNone, ActionPlayPause, ActionPlay, ActionPause, ActionStop, ActionPreviousTrack,
		ActionNextTrack, ActionVolumeUp, ActionVolumeDown, ActionMuteAll, ActionMuteMic,
		ActionNightLight, ActionScreensOff, ActionLockPC, ActionSleepPC, ActionNextProfile,
		ActionPreviousProfile, ActionOpenSettings:
		return true
	}
	_, mute := a.MuteKnob()
	_, open := a.OpenApp()
	_, closeApp := a.CloseApp()
	_, url := a.URL()
	_, keys := a.Keys()
	_, profile := a.Profile()
	return mute || open || closeApp || url || keys || profile
}

// DefaultMixerButtons has the SMC-Mixer's M buttons mute their faders, and « and » step profiles.
func DefaultMixerButtons() ButtonMap {
	m := ButtonMap{MixerNoteButton(46): {ActionPreviousProfile}, MixerNoteButton(47): {ActionNextProfile}}
	for i := 0; i < 8; i++ {
		m[MixerNoteButton(16+i)] = []ButtonAction{MuteAction(i)}
	}
	return m
}

// ForMixer is the setup a mixer frame is handled with: Columns come from MixerColumns and every
// profile's Jobs from its MixerJobs, so the mixer has its own knobs. A fader's top is already its
// highest value, so only the mixer's own MixerInvert flips it. A mixer never calibrated reads its
// faders, then its knobs, and an SMC-Mixer always does.
func (s Setup) ForMixer() Setup {
	profiles := make([]Profile, len(s.Profiles))
	for i, p := range s.Profiles {
		p.Jobs = p.MixerJobs
		profiles[i] = p
	}
	s.Profiles = profiles
	if s.MixerIsSMC() {
		s.Columns = append([]int{}, smcColumns...)
	} else if s.MixerColumns == nil {
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
	s.Invert = !s.MixerInvert
	s.BoardKinds, s.BoardLayout = nil, nil
	return s
}

// MixerButtonOrder is the order Settings lists the mixer's buttons in: Calibrate's, or the
// SMC-Mixer's own.
func (s Setup) MixerButtonOrder() []int {
	if s.ButtonOrder != nil && !s.MixerIsSMC() {
		return s.ButtonOrder
	}
	return SMCButtonOrder()
}

// MixerState turns MIDI short messages into a frame of 0..1023 values. A control that has not
// moved yet reads -1, since its position is unknown until the mixer sends it.
type MixerState struct {
	values []int
	// The last pitch bend each fader sent, as it came, which the strip LED needs to stop blinking.
	pitch   [8][2]int
	pitchOK [8]bool
	last    int
}

func NewMixerState() *MixerState {
	v := make([]int, MixerColumns)
	for i := range v {
		v[i] = -1
	}
	return &MixerState{values: v, last: -1}
}

// Pitch is the last pitch bend fader strip sent, if it has sent one.
func (m *MixerState) Pitch(strip int) (lsb, msb int, ok bool) {
	return m.pitch[strip][0], m.pitch[strip][1], m.pitchOK[strip]
}

// LastChanged is the column the last Feed changed, or -1.
func (m *MixerState) LastChanged() int { return m.last }

// A step is never 0 or 127, and a CC-mode button on the same number never sends anything else.
func isVPotStep(cc, value int) bool {
	return cc >= vpotFirstCC && cc < vpotFirstCC+vpotCount && value != 0 && value != 127
}

// Feed takes a WinMM short message (status | data1<<8 | data2<<16). It reports whether a column
// changed, and the id of a button that was just pressed, or -1: a button has the same id in
// either mode (SMCButtonID).
func (m *MixerState) Feed(msg uint32) (changed bool, pressed int) {
	m.last = -1
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
			m.pitch[channel] = [2]int{data1, data2}
			m.pitchOK[channel] = true
		}
		// The SMC-Mixer only sends the top 7 bits, so its fader tops out at 127<<7, not 16383.
		return m.set(col, min(((data1|data2<<7)*1023+8128)/16256, 1023)), -1
	case 0xB0:
	default:
		return false, -1
	}

	cc, value := data1, data2
	if isVPotStep(cc, value) {
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
	if id, ok := SMCButtonID(cc); ok {
		if value > 0 {
			return false, id
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
	m.last = col
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
