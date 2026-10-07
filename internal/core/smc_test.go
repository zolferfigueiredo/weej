package core

import (
	"reflect"
	"testing"
)

func note(n, velocity int) uint32 { return 0x90 | uint32(n)<<8 | uint32(velocity)<<16 }

func TestSMCButtonsAreTheSameInBothModes(t *testing.T) {
	m := NewMixerState()
	press := func(msg uint32) int {
		_, pressed := m.Feed(msg)
		return pressed
	}
	for i := 0; i < 8; i++ {
		if a, b := press(cc(20+i, 127)), press(note(16+i, 127)); a != b || a != MixerNoteButton(16+i) {
			t.Errorf("M%d = %d in CC mode, %d in DAW mode, want both %d", i+1, a, b, MixerNoteButton(16+i))
		}
		if a, b := press(cc(smcSCCs[i], 127)), press(note(8+i, 127)); a != b {
			t.Errorf("S%d = %d in CC mode, %d in DAW mode", i+1, a, b)
		}
	}
	for i, n := range smcBottomNotes {
		if a, b := press(cc(52+i, 127)), press(note(n, 127)); a != b {
			t.Errorf("bottom button %d = %d in CC mode, %d in DAW mode", i+1, a, b)
		}
	}
	if got := press(cc(50, 127)); got != MixerNoteButton(14) {
		t.Errorf("CC 50 = %d, want S7", got)
	}
}

func TestSMCButtonCCsRoundTrip(t *testing.T) {
	withCC := 0
	for _, id := range SMCButtonOrder() {
		c, ok := SMCButtonCC(id)
		if !ok {
			continue
		}
		withCC++
		if back, ok := SMCButtonID(c); !ok || back != id {
			t.Errorf("button %d sends CC %d, which reads back as %d", id, c, back)
		}
	}
	if withCC != 27 {
		t.Errorf("%d buttons send a CC, want M, S and the bottom row: 27", withCC)
	}
	if _, ok := SMCButtonCC(MixerNoteButton(0)); ok {
		t.Error("R1 has a CC, want none")
	}
}

func TestSMCControlsAreFixed(t *testing.T) {
	for _, name := range []string{"SMC-Mixer", "MIDIIN2 (SMC-Mixer)", "SMC-Mixer-bt", "smc-mixer"} {
		if !IsSMCName(name) {
			t.Errorf("%q not an SMC-Mixer", name)
		}
	}
	if IsSMCName("nanoKONTROL2") {
		t.Error("another mixer read as an SMC-Mixer")
	}

	for i, c := range SMCControls() {
		if c.Input != smcColumns[i] {
			t.Errorf("control %d reads column %d, want %d", i, c.Input, smcColumns[i])
		}
	}
}

func TestSMCModeFollowsTheMixer(t *testing.T) {
	var mode SMCMode
	steps := []struct {
		msg uint32
		daw bool
	}{
		{0xE3 | 64<<16, true},
		{cc(40, 64), false},
		{note(16, 127), true},
		{cc(20, 127), false},
		{cc(20, 65), true},
		{cc(52, 0), false},
		{0x80 | 16<<8, true},
	}
	for i, s := range steps {
		mode.Seen(s.msg)
		if mode.DAW() != s.daw {
			t.Errorf("step %d (%#x): DAW = %v, want %v", i, s.msg, mode.DAW(), s.daw)
		}
	}
}

func TestSMCLEDMessages(t *testing.T) {
	if msg, ok := SMCButtonLED(MixerNoteButton(16), true, true); !ok || msg != note(16, 127) {
		t.Errorf("M1 on in DAW mode = %#x, %v", msg, ok)
	}
	if msg, ok := SMCButtonLED(MixerNoteButton(16), false, false); !ok || msg != cc(20, 0) {
		t.Errorf("M1 off in CC mode = %#x, %v", msg, ok)
	}
	if _, ok := SMCButtonLED(MixerNoteButton(24), true, false); ok {
		t.Error("Square 1 lit in CC mode, where it sends nothing")
	}
}

func TestSMCButtonReleasedInBothModes(t *testing.T) {
	cases := []struct {
		msg  uint32
		id   int
		want bool
	}{
		{note(16, 0), MixerNoteButton(16), true},
		{0x80 | 94<<8 | 64<<16, MixerNoteButton(94), true},
		{note(16, 127), 0, false},
		{cc(20, 0), MixerNoteButton(16), true},
		{cc(20, 127), 0, false},
		{cc(40, 0), 0, false},
	}
	for _, c := range cases {
		if id, ok := SMCButtonReleased(c.msg); ok != c.want || ok && id != c.id {
			t.Errorf("SMCButtonReleased(%#x) = %d, %v, want %d, %v", c.msg, id, ok, c.id, c.want)
		}
	}
}

func TestBoardLayoutsStayDrawable(t *testing.T) {
	if got := CleanLayout(nil, 3); got != nil {
		t.Errorf("no layout = %v, want nil: one row of knobs", got)
	}
	got := CleanLayout([][]int{{2, 9, 0}, {}, {0, 4}}, 5)
	if !reflect.DeepEqual(got, [][]int{{2, 0}, {4, 1, 3}}) {
		t.Errorf("layout = %v, want bad and repeated knobs gone and the missing ones on the last row", got)
	}
}

func TestButtonWatcherFindsPressesEitherWay(t *testing.T) {
	var w ButtonWatcher
	inputs := map[int]int{4: 0, 5: 1}
	press := func(a, b int, now float64) []int { return w.Pressed([]int{a, b}, inputs, now) }
	if got := press(0, 1023, 0); got != nil {
		t.Fatalf("first frame pressed %v, want it read as rest", got)
	}
	if got := press(1023, 1023, 1); !reflect.DeepEqual(got, []int{4}) {
		t.Errorf("pulled-down button pressed %v, want knob 4", got)
	}
	if got := press(0, 1023, 1.01); got != nil {
		t.Errorf("release pressed %v", got)
	}
	if got := press(1023, 1023, 1.02); got != nil {
		t.Errorf("a bounce right after pressed %v, want nothing", got)
	}
	press(0, 1023, 1.03)
	if got := press(1023, 0, 2); !reflect.DeepEqual(got, []int{4, 5}) {
		t.Errorf("pressed %v, want both, the pulled-up one going to 0", got)
	}
	if got := press(1023-150, 150, 2.1); got != nil {
		t.Errorf("still held pressed %v", got)
	}
	w.Reset()
	if got := press(1023, 0, 3); got != nil {
		t.Errorf("after Reset pressed %v, want the new rest learned", got)
	}
}
