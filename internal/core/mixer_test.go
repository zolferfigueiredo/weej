package core

import (
	"reflect"
	"slices"
	"testing"
)

func cc(control, value int) uint32 {
	return 0xB0 | uint32(control)<<8 | uint32(value)<<16
}

func TestMixerFadersAndKnobsScaleToColumns(t *testing.T) {
	m := NewMixerState()
	for _, v := range m.Values() {
		if v != -1 {
			t.Fatalf("fresh values = %v, want all -1", m.Values())
		}
	}

	cases := []struct {
		msg  uint32
		col  int
		want int
	}{
		{cc(40, 0), 40, 0},
		{cc(40, 127), 40, 1023},
		{cc(47, 64), 47, 516},
		{cc(30, 127), 30, 1023},
		{cc(37, 1), 37, 8},
		{cc(9, 100), 9, 806},
	}
	for _, c := range cases {
		changed, pressed := m.Feed(c.msg)
		if !changed || pressed != -1 {
			t.Errorf("Feed(%#x) = %v, %d, want a change and no press", c.msg, changed, pressed)
		}
		if got := m.Values()[c.col]; got != c.want {
			t.Errorf("Feed(%#x): column %d = %d, want %d", c.msg, c.col, got, c.want)
		}
	}
	if changed, _ := m.Feed(cc(37, 1)); changed {
		t.Error("the same value again reported a change")
	}
	if got := m.Values()[41]; got != -1 {
		t.Errorf("untouched fader 2 = %d, want -1", got)
	}
}

func TestMixerButtonsPressOnlyOnTheWayDown(t *testing.T) {
	m := NewMixerState()
	if changed, pressed := m.Feed(cc(20, 127)); changed || pressed != MixerNoteButton(16) {
		t.Errorf("press = %v, %d, want no change and M1", changed, pressed)
	}
	if _, pressed := m.Feed(cc(20, 0)); pressed != -1 {
		t.Errorf("release pressed %d, want -1", pressed)
	}
	if changed, pressed := m.Feed(0x90 | 40<<8 | 100<<16); changed || pressed != MixerNoteButton(40) {
		t.Errorf("note on = %v, %d, want a DAW-mode button", changed, pressed)
	}
}

func TestEveryMixerButtonIsListedOnce(t *testing.T) {
	seen := map[int]bool{}
	for _, b := range SMCButtonOrder() {
		if seen[b] {
			t.Errorf("button %d listed twice", b)
		}
		seen[b] = true
	}
	if len(seen) != 43 {
		t.Errorf("%d buttons, want 8 strips of 4 and a bottom row of 11", len(seen))
	}
	for cc := 0; cc < 128; cc++ {
		if _, ok := SMCButtonID(cc); ok && slices.Contains(defaultMixerControls, cc) {
			t.Errorf("button CC %d is also a fader or knob", cc)
		}
	}
	for id := range DefaultMixerButtons() {
		if !seen[id] {
			t.Errorf("default action on button %d, which is not a listed button", id)
		}
	}
}

func TestButtonActionsValidate(t *testing.T) {
	valid := []ButtonAction{ActionNone, ActionPlayPause, ActionStop, ActionPreviousTrack, ActionNextTrack,
		ActionNextProfile, ActionPreviousProfile, ActionOpenSettings, MuteAction(0), MuteAction(15)}
	for _, a := range valid {
		if !a.Valid() {
			t.Errorf("%q not valid", a)
		}
	}
	for _, a := range []ButtonAction{"bogus", "mute:", "mute:-1", "mute:x", "mute:01"} {
		if a.Valid() {
			t.Errorf("%q valid, want invalid", a)
		}
	}
	if knob, ok := MuteAction(3).MuteKnob(); !ok || knob != 3 {
		t.Errorf("MuteKnob = %d, %v, want 3", knob, ok)
	}
}

func TestEngineSkipsUnknownMixerValues(t *testing.T) {
	a := &fakeApplier{}
	e := NewEngine(a)
	setup := Setup{
		Columns:  []int{0, 1},
		Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}, {{Kind: JobMicrophone}}}}},
		Speed:    SpeedSuperFast,
		Invert:   true,
	}
	e.Handle([]int{-1, 512}, setup, false)

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.applied) != 1 || a.applied[0].Kind != JobMicrophone {
		t.Errorf("applied = %v, want only the microphone (the master fader never moved)", a.applied)
	}
}

type recordingApplier struct {
	fakeApplier
	values []float64
}

func (r *recordingApplier) Apply(job Job, s float64) {
	r.fakeApplier.Apply(job, s)
	r.mu.Lock()
	r.values = append(r.values, s)
	r.mu.Unlock()
}

func TestEngineMuteHoldsAndRestores(t *testing.T) {
	a := &recordingApplier{}
	e := NewEngine(a)
	setup := Setup{
		Columns:  []int{0},
		Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}}}},
		Speed:    SpeedSuperFast,
		Invert:   true,
	}

	e.Handle([]int{1023}, setup, false)
	if !e.ToggleMute(0, setup) {
		t.Fatal("first toggle did not mute")
	}
	e.Handle([]int{0}, setup, false)
	if e.ToggleMute(0, setup) {
		t.Fatal("second toggle did not unmute")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if !reflect.DeepEqual(a.values, []float64{1, 0, 0}) {
		t.Errorf("applied values = %v, want [1 0 0]: the sync, the mute, then the fader's new spot on unmute", a.values)
	}
}

func TestEngineResetUnmutes(t *testing.T) {
	e := NewEngine(&fakeApplier{})
	setup := Setup{Columns: []int{0}, Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}}}}, Invert: true}
	e.ToggleMute(0, setup)
	e.Reset()
	if !e.ToggleMute(0, setup) {
		t.Error("after Reset the next toggle should mute again")
	}
}

func TestMoveWatcherIgnoresJitter(t *testing.T) {
	var w MoveWatcher
	if got := w.Moved([]int{500, -1}); got != nil {
		t.Errorf("first frame moved %v, want nothing", got)
	}
	if got := w.Moved([]int{505, -1}); got != nil {
		t.Errorf("jitter moved %v, want nothing", got)
	}
	if got := w.Moved([]int{520, 300}); !reflect.DeepEqual(got, []int{0, 1}) {
		t.Errorf("moved %v, want [0 1]: a real turn and a mixer control's first report", got)
	}
	if got := w.Moved([]int{528, 300}); got != nil {
		t.Errorf("moved %v, want nothing within the threshold of the new spot", got)
	}
}

func TestMixerDAWModeLandsOnTheSameColumns(t *testing.T) {
	m := NewMixerState()

	m.Feed(0xE0 | 0<<8 | 127<<16)
	m.Feed(0xE7 | 0<<8 | 64<<16)
	if v := m.Values(); v[40] != 1023 || v[47] != 516 {
		t.Errorf("faders 1 and 8 = %d, %d, want 1023 and 516 on CC 40 and 47's columns, as CC mode reads them", v[40], v[47])
	}

	m.Feed(cc(16, 1))
	m.Feed(cc(16, 1))
	m.Feed(cc(16, 65))
	if v := m.Values()[30]; v != 512+vpotStep {
		t.Errorf("knob 1 = %d, want two steps up and one down from the middle", v)
	}
	for i := 0; i < 300; i++ {
		m.Feed(cc(17, 65))
	}
	if v := m.Values()[31]; v != 0 {
		t.Errorf("knob 2 = %d, want it to stop at 0", v)
	}

	if _, pressed := m.Feed(cc(20, 127)); pressed != MixerNoteButton(16) {
		t.Errorf("CC 20 at 127 pressed %d, want M1", pressed)
	}
	if _, pressed := m.Feed(cc(20, 1)); pressed != -1 {
		t.Errorf("CC 20 stepping pressed %d, want knob 5 instead", pressed)
	}
}

func TestMixerDAWButtonsAreNotes(t *testing.T) {
	m := NewMixerState()
	if _, pressed := m.Feed(0x90 | 24<<8 | 127<<16); pressed != MixerNoteButton(24) {
		t.Errorf("Square 1 pressed %d, want %d", pressed, MixerNoteButton(24))
	}
	if _, pressed := m.Feed(0x90 | 24<<8 | 0<<16); pressed != -1 {
		t.Errorf("release pressed %d, want -1", pressed)
	}
	if note, ok := MixerButtonNote(MixerNoteButton(94)); !ok || note != 94 {
		t.Errorf("MixerButtonNote = %d, %v, want 94", note, ok)
	}
	if _, ok := MixerButtonNote(20); ok {
		t.Error("CC 20 read as a note")
	}
}

func TestEngineUnmuteAllRestoresWithoutAHUD(t *testing.T) {
	a := &recordingApplier{}
	e := NewEngine(a)
	setup := Setup{
		Columns:  []int{0},
		Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}}}},
		Speed:    SpeedSuperFast,
		Invert:   true,
	}
	e.Handle([]int{1023}, setup, false)
	e.ToggleMute(0, setup)
	huds := len(a.huds)
	if n := e.UnmuteAll(setup); n != 1 {
		t.Errorf("UnmuteAll = %d, want 1", n)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if last := a.values[len(a.values)-1]; last != 1 || len(a.huds) != huds {
		t.Errorf("last applied %v, HUDs %d -> %d, want the master back at 1 with no HUD", last, huds, len(a.huds))
	}
	if e.UnmuteAll(setup) != 0 {
		t.Error("a second UnmuteAll found mutes left")
	}
}

func TestButtonActionsWithASetting(t *testing.T) {
	cases := []struct {
		action ButtonAction
		valid  bool
	}{
		{OpenAppAction(`C:\Program Files\Spotify\Spotify.exe`), true},
		{"open:", false},
		{CloseAppAction("spotify.exe"), true},
		{URLAction("https://weej.zolfer.com"), true},
		{URLAction("HTTP://example.com"), true},
		{URLAction("file:///C:/x"), false},
		{KeysAction(Shortcut{VK: 0x4D, Mods: ModControl | ModShift, Key: "m"}), true},
		{KeysAction(Shortcut{VK: 0xBA, Key: ":"}), true},
		{"keys:2:0:x", false},
		{"keys:", false},
		{ProfileAction(2), true},
		{"profile:-1", false},
		{ActionPlay, true},
		{ActionMuteMic, true},
		{"pc.reboot", false},
	}
	for _, c := range cases {
		if got := c.action.Valid(); got != c.valid {
			t.Errorf("%q valid = %v, want %v", c.action, got, c.valid)
		}
	}

	s, ok := KeysAction(Shortcut{VK: 0xBA, Mods: ModShift, Key: ":"}).Keys()
	if !ok || s.VK != 0xBA || s.Mods != ModShift || s.Key != ":" {
		t.Errorf("Keys round trip = %+v, %v", s, ok)
	}
	if path, ok := OpenAppAction(`C:\Apps\c.exe`).OpenApp(); !ok || path != `C:\Apps\c.exe` {
		t.Errorf("OpenApp = %q, %v", path, ok)
	}
	if i, ok := ProfileAction(3).Profile(); !ok || i != 3 {
		t.Errorf("Profile = %d, %v", i, ok)
	}
	if _, ok := MuteAction(1).Profile(); ok {
		t.Error("a mute action read as a profile")
	}
}
