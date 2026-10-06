package core

import (
	"reflect"
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
		{cc(40, 0), 0, 0},
		{cc(40, 127), 0, 1023},
		{cc(47, 64), 7, 516},
		{cc(30, 127), 8, 1023},
		{cc(37, 1), 15, 8},
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
	if got := m.Values()[1]; got != -1 {
		t.Errorf("untouched fader 2 = %d, want -1", got)
	}
}

func TestMixerButtonsPressOnlyOnTheWayDown(t *testing.T) {
	m := NewMixerState()
	if changed, pressed := m.Feed(cc(20, 127)); changed || pressed != 20 {
		t.Errorf("press = %v, %d, want no change and CC 20", changed, pressed)
	}
	if _, pressed := m.Feed(cc(20, 0)); pressed != -1 {
		t.Errorf("release pressed %d, want -1", pressed)
	}
	if changed, pressed := m.Feed(0x90 | 40<<8 | 100<<16); changed || pressed != -1 {
		t.Errorf("note on = %v, %d, want ignored", changed, pressed)
	}
}

func TestEveryMixerButtonIsListedOnce(t *testing.T) {
	seen := map[int]bool{}
	for _, b := range DefaultMixerButtonOrder {
		if seen[b] {
			t.Errorf("CC %d listed twice", b)
		}
		seen[b] = true
		for _, c := range append(append([]int{}, mixerFaderCCs...), mixerKnobCCs...) {
			if b == c {
				t.Errorf("button CC %d is also a fader or knob", b)
			}
		}
	}
	for cc := range DefaultMixerButtons() {
		if !seen[cc] {
			t.Errorf("default action on CC %d, which is not a listed button", cc)
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

func TestForMixerReadsColumnIAndNeverFlips(t *testing.T) {
	s := Setup{Columns: []int{0, 3, -1}, Invert: false}
	m := s.ForMixer()
	if !reflect.DeepEqual(m.Columns, []int{0, 1, 2}) || !m.Invert {
		t.Errorf("ForMixer = %v invert %v, want [0 1 2] and no flip", m.Columns, m.Invert)
	}
	if !reflect.DeepEqual(s.Columns, []int{0, 3, -1}) {
		t.Errorf("ForMixer changed the saved columns to %v", s.Columns)
	}
}

func TestMidiPorts(t *testing.T) {
	p := MidiPort("SMC-Mixer-bt")
	if !IsMidiPort(p) || MidiDevice(p) != "SMC-Mixer-bt" || IsMidiPort("COM6") {
		t.Errorf("MidiPort round trip failed: %q", p)
	}
}

func TestMixerButtonsSurviveSaving(t *testing.T) {
	fresh, _ := DecodeSettings([]byte(`{"profiles":[{"name":"Default","jobs":[]}]}`), "Default")
	if !reflect.DeepEqual(fresh.Buttons, DefaultMixerButtons()) {
		t.Errorf("old file buttons = %v, want the defaults", fresh.Buttons)
	}

	s := DefaultSettings("Default")
	s.Buttons = map[int]ButtonAction{20: ActionNone, 52: ActionPlayPause, 21: MuteAction(9)}
	data, err := EncodeSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	back, _ := DecodeSettings(data, "Default")
	want := map[int]ButtonAction{52: ActionPlayPause, 21: MuteAction(9)}
	if !reflect.DeepEqual(back.Buttons, want) {
		t.Errorf("round trip = %v, want %v (CC 20 set to nothing stays nothing)", back.Buttons, want)
	}

	bad, _ := DecodeSettings([]byte(`{"mixerButtons":{"52":"bogus","x":"settings","61":"settings"}}`), "Default")
	if !reflect.DeepEqual(bad.Buttons, map[int]ButtonAction{61: ActionOpenSettings}) {
		t.Errorf("bad entries = %v, want only CC 61", bad.Buttons)
	}
}

func TestEngineSkipsUnknownMixerValues(t *testing.T) {
	a := &fakeApplier{}
	e := NewEngine(a)
	setup := Setup{
		Columns:  []int{0, 1},
		Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}, {{Kind: JobMicrophone}}}}},
		Speed:    SpeedSuperFast,
	}.ForMixer()
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
	}.ForMixer()

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
	setup := Setup{Columns: []int{0}, Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}}}}}.ForMixer()
	e.ToggleMute(0, setup)
	e.Reset()
	if !e.ToggleMute(0, setup) {
		t.Error("after Reset the next toggle should mute again")
	}
}

func TestForMixerUsesTheMixersOwnCalibration(t *testing.T) {
	s := Setup{Columns: []int{0, 3, 2}, MixerColumns: []int{9, -1}}
	if got := s.ForMixer().Columns; !reflect.DeepEqual(got, []int{9, -1}) {
		t.Errorf("ForMixer = %v, want the mixer's own [9 -1]", got)
	}
}

func TestMixerColumnsSurviveSaving(t *testing.T) {
	s := DefaultSettings("Default")
	s.Columns = []int{0, 3}
	data, _ := EncodeSettings(s)
	back, _ := DecodeSettings(data, "Default")
	if back.MixerColumns != nil {
		t.Errorf("never calibrated = %v, want nil", back.MixerColumns)
	}

	s.MixerColumns = []int{8, -1}
	data, _ = EncodeSettings(s)
	back, _ = DecodeSettings(data, "Default")
	if !reflect.DeepEqual(back.MixerColumns, []int{8, -1}) || !reflect.DeepEqual(back.Columns, []int{0, 3}) {
		t.Errorf("round trip = mixer %v board %v, want [8 -1] and [0 3]", back.MixerColumns, back.Columns)
	}
}

func TestCalibratorWaitsForAnUnknownControlToSwing(t *testing.T) {
	c := NewCalibrator(nil, false)
	c.Feed([]int{-1, -1}, 0)
	c.Feed([]int{-1, 900}, 0.1)
	if len(c.Found()) != 0 {
		t.Fatalf("found %v after a first touch, want nothing yet", c.Found())
	}
	c.Feed([]int{-1, 100}, 0.2)
	if !reflect.DeepEqual(c.Found(), []int{1}) {
		t.Errorf("found %v, want column 1 once it swung", c.Found())
	}
}

func TestMixerCalibrationSkipsCleaningThenFindsButtons(t *testing.T) {
	c := NewMixerCalibrator(nil, false)
	c.Feed([]int{-1, -1}, 0)
	c.Feed([]int{-1, 0}, 0.1)
	c.Feed([]int{-1, 1023}, 0.2)
	if c.Phase() != 0 || c.Knob() != 1 {
		t.Fatalf("after knob A: phase %d knob %d, want straight on to knob B", c.Phase(), c.Knob())
	}

	c.PressButton(20)
	if len(c.Buttons()) != 0 {
		t.Fatal("a button counted before the button stage")
	}
	c.StartButtons()
	c.Feed([]int{0, 0}, 0.3)
	c.Feed([]int{1023, 0}, 0.4)
	if len(c.Found()) != 1 {
		t.Errorf("found %v, want knobs left alone during the button stage", c.Found())
	}
	before := c.StepKey()
	c.PressButton(52)
	c.PressButton(20)
	c.PressButton(52)
	if !reflect.DeepEqual(c.Buttons(), []int{52, 20}) || c.RepeatedButton() != 0 {
		t.Errorf("buttons %v repeated %d, want [52 20] and button 1 pressed again", c.Buttons(), c.RepeatedButton())
	}
	if c.StepKey() == before {
		t.Error("finding buttons did not change the step")
	}
}

func TestForMixerHasItsOwnKnobCount(t *testing.T) {
	s := Setup{Columns: []int{0, 1}, MixerColumns: []int{8, 9, 10}}
	if got := len(s.ForMixer().Columns); got != 3 {
		t.Errorf("mixer knobs = %d, want 3 from its own calibration", got)
	}
}

func TestButtonOrderSurvivesSaving(t *testing.T) {
	s := DefaultSettings("Default")
	if !reflect.DeepEqual(s.MixerButtonOrder(), DefaultMixerButtonOrder) {
		t.Errorf("fresh order = %v, want the default", s.MixerButtonOrder())
	}
	s.ButtonOrder = []int{52, 20}
	data, _ := EncodeSettings(s)
	back, _ := DecodeSettings(data, "Default")
	if !reflect.DeepEqual(back.ButtonOrder, []int{52, 20}) {
		t.Errorf("round trip = %v, want [52 20]", back.ButtonOrder)
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

func TestMixerKnobIsFoundAfterAFewSteps(t *testing.T) {
	few := [][]int{{-1, 500}, {-1, 540}}

	board := NewCalibrator(nil, false)
	mixer := NewMixerCalibrator(nil, false)
	for i, v := range few {
		board.Feed(v, float64(i))
		mixer.Feed(v, float64(i))
	}
	if len(board.Found()) != 0 {
		t.Errorf("board found %v after a 40 step nudge, want nothing: pots jitter", board.Found())
	}
	if !reflect.DeepEqual(mixer.Found(), []int{1}) {
		t.Errorf("mixer found %v, want column 1 after a few clean steps", mixer.Found())
	}
}
