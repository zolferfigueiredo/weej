package core

import (
	"slices"
	"strings"
	"sync/atomic"
)

// The M-VAVE SMC-Mixer has fixed control ids, which device.js draws by and profiles save under:
//   - fader i is knob i, read from column 40+i; its rotary knob is knob 8+i, read from column 30+i.
//   - a button's id is 128 plus the note it sends in DAW mode: R 128+i, S 136+i, M 144+i,
//     Square 152+i, and the bottom row 222, 221, 223, 219, 220, 174, 175, 224, 225, 226, 227.
//
// In CC mode M, S and the bottom row send CCs (SMCButtonID); R and Square send nothing.

var smcColumns = []int{40, 41, 42, 43, 44, 45, 46, 47, 30, 31, 32, 33, 34, 35, 36, 37}

// smcBottomNotes is the bottom row left to right: play, stop, record, rewind, fast-forward,
// bank left, bank right, up, down, left, right. In CC mode they send CC 52 to 62 in that order.
var smcBottomNotes = []int{94, 93, 95, 91, 92, 46, 47, 96, 97, 98, 99}

// S i sends these CCs in CC mode. A real unit's S7 sent 51, the same as its S8.
var smcSCCs = []int{28, 29, 38, 39, 48, 49, 50, 51}

// IsSMCName matches the mixer by USB, Bluetooth and MIDI 2.0 name alike.
func IsSMCName(device string) bool {
	return strings.Contains(strings.ToLower(device), "smc-mixer")
}

func (s Setup) MixerIsSMC() bool { return IsSMCName(s.MixerPort) }

// SMCButtonID is the id of the button that sends cc in CC mode.
func SMCButtonID(cc int) (int, bool) {
	switch {
	case cc >= 20 && cc <= 27:
		return MixerNoteButton(16 + cc - 20), true
	case cc >= 52 && cc <= 62:
		return MixerNoteButton(smcBottomNotes[cc-52]), true
	}
	if i := slices.Index(smcSCCs, cc); i >= 0 {
		return MixerNoteButton(8 + i), true
	}
	return 0, false
}

// SMCButtonCC is the CC a button sends in CC mode, if it sends one.
func SMCButtonCC(id int) (int, bool) {
	note, ok := MixerButtonNote(id)
	if !ok {
		return 0, false
	}
	switch {
	case note >= 16 && note < 24:
		return 20 + note - 16, true
	case note >= 8 && note < 16:
		return smcSCCs[note-8], true
	}
	if i := slices.Index(smcBottomNotes, note); i >= 0 {
		return 52 + i, true
	}
	return 0, false
}

// SMCButtonOrder lists every button by id: M, S, R and Square for each strip, then the bottom row.
func SMCButtonOrder() []int {
	var ids []int
	for strip := 0; strip < 8; strip++ {
		for _, first := range []int{16, 8, 0, 24} {
			ids = append(ids, MixerNoteButton(first+strip))
		}
	}
	for _, note := range smcBottomNotes {
		ids = append(ids, MixerNoteButton(note))
	}
	return ids
}

// SMCStripOf is the strip a fader or knob column belongs to.
func SMCStripOf(column int) (int, bool) {
	switch {
	case column >= 40 && column < 48:
		return column - 40, true
	case column >= 30 && column < 38:
		return column - 30, true
	}
	return 0, false
}

// SMCButtonLED lights or clears a button's LED: by its note in DAW mode, or by its CC in CC mode,
// which R and Square don't have.
func SMCButtonLED(id int, on, daw bool) (uint32, bool) {
	value := uint32(0)
	if on {
		value = 127
	}
	if !daw {
		cc, ok := SMCButtonCC(id)
		if !ok {
			return 0, false
		}
		return 0xB0 | uint32(cc)<<8 | value<<16, true
	}
	note, ok := MixerButtonNote(id)
	if !ok {
		return 0, false
	}
	return 0x90 | uint32(note)<<8 | value<<16, true
}

// SMCMode tells which mode the mixer is in from what it sends: notes, pitch bend and knob steps
// only come in DAW mode, other CCs only in CC mode.
type SMCMode struct{ daw atomic.Bool }

func (m *SMCMode) DAW() bool { return m.daw.Load() }

func (m *SMCMode) Seen(msg uint32) {
	status := msg & 0xF0
	cc := int(msg>>8) & 0x7F
	value := int(msg>>16) & 0x7F
	switch {
	case status == 0x80, status == 0x90, status == 0xE0, status == 0xB0 && isVPotStep(cc, value):
		m.daw.Store(true)
	case status == 0xB0:
		m.daw.Store(false)
	}
}

// StripLights times the strip LEDs: a strip lights when it moves and goes out stripLightTail
// seconds after it last moved, so a hand pausing mid-move doesn't make it flicker.
type StripLights struct {
	lit  [8]bool
	last [8]float64
}

const stripLightTail = 0.3

// Move lights the strip, or keeps it lit, from now.
func (l *StripLights) Move(strip int, now float64) {
	l.last[strip] = now
	l.lit[strip] = true
}

// Due puts out the strips that have been still for the whole tail, and lists them.
func (l *StripLights) Due(now float64) []int {
	var off []int
	for s := range l.lit {
		if l.lit[s] && now-l.last[s] >= stripLightTail {
			l.lit[s] = false
			off = append(off, s)
		}
	}
	return off
}

func (l *StripLights) Any() bool { return slices.Contains(l.lit[:], true) }

// AllOff puts out every lit strip and lists them.
func (l *StripLights) AllOff() []int {
	var off []int
	for s := range l.lit {
		if l.lit[s] {
			l.lit[s] = false
			off = append(off, s)
		}
	}
	return off
}

// MigrateSMC moves mixer data saved before WeeJ knew the SMC-Mixer onto its fixed ids. Buttons
// saved under their CC-mode CC move to their ids. An SMC's own calibration goes: each knob's jobs,
// and the mutes pointing at it, move to the control the calibration had found for it. Running it
// again changes nothing.
func MigrateSMC(s *Setup) {
	for i := range s.Profiles {
		s.Profiles[i].Buttons = smcButtonKeys(s.Profiles[i].Buttons)
	}
	if !s.MixerIsSMC() {
		if s.ButtonOrder != nil {
			order := []int{}
			for _, id := range s.ButtonOrder {
				if to, ok := SMCButtonID(id); ok {
					id = to
				}
				if !slices.Contains(order, id) {
					order = append(order, id)
				}
			}
			s.ButtonOrder = order
		}
		return
	}
	if s.MixerColumns != nil {
		for i := range s.Profiles {
			remapSMCKnobs(&s.Profiles[i], s.MixerColumns)
		}
	}
	s.MixerColumns = nil
	s.ButtonOrder = nil
}

// Canonical keys go first, so a CC-mode key's actions follow the same button's own.
func smcButtonKeys(m ButtonMap) ButtonMap {
	if m == nil {
		return nil
	}
	out := ButtonMap{}
	for id, actions := range m {
		if _, ok := SMCButtonID(id); !ok {
			out[id] = slices.Clone(actions)
		}
	}
	for cc, actions := range m {
		id, ok := SMCButtonID(cc)
		if !ok {
			continue
		}
		for _, a := range actions {
			if !slices.Contains(out[id], a) {
				out[id] = append(out[id], a)
			}
		}
	}
	return out
}

func remapSMCKnobs(p *Profile, calibrated []int) {
	to := func(knob int) (int, bool) {
		if knob < 0 || knob >= len(calibrated) {
			return 0, false
		}
		i := slices.Index(smcColumns, calibrated[knob])
		return i, i >= 0
	}
	jobs := make([][]Job, len(smcColumns))
	for knob, row := range p.MixerJobs {
		if i, ok := to(knob); ok && jobs[i] == nil {
			jobs[i] = row
		}
	}
	for i, row := range jobs {
		if row == nil {
			jobs[i] = []Job{}
		}
	}
	p.MixerJobs = jobs

	if p.Buttons == nil {
		return
	}
	buttons := ButtonMap{}
	for id, actions := range p.Buttons {
		var kept []ButtonAction
		for _, a := range actions {
			if knob, ok := a.MuteKnob(); ok {
				i, ok := to(knob)
				if !ok {
					continue
				}
				a = MuteAction(i)
			}
			if !slices.Contains(kept, a) {
				kept = append(kept, a)
			}
		}
		if len(kept) > 0 {
			buttons[id] = kept
		}
	}
	p.Buttons = buttons
}
