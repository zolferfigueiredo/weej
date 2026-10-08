package core

import (
	"reflect"
	"testing"
)

// wizardRun feeds a DIY wizard frames, each from the previous one with the given changes.
type wizardRun struct {
	t     *testing.T
	c     *Calibration
	frame []int
}

func newRun(t *testing.T, d Device, controls []int, frame []int) *wizardRun {
	r := &wizardRun{t: t, c: NewCalibration(d, controls), frame: append([]int(nil), frame...)}
	r.c.Feed(r.frame)
	return r
}

// move slides col to v in ten steps.
func (r *wizardRun) move(col, v int) {
	from := r.frame[col]
	for i := 1; i <= 10; i++ {
		r.frame[col] = from + (v-from)*i/10
		r.c.Feed(r.frame)
	}
}

func diyBoard(knobs, buttons int) Device {
	return NewDevice("d1", "Desk", DeviceDIY, knobs, 0, buttons, "Default")
}

func TestWizardReadsBothEndsThenFindsEachPot(t *testing.T) {
	r := newRun(t, diyBoard(2, 0), []int{0, 1}, []int{12, 1010, 9, 500})
	if s := r.c.State(); s.Stage != CalZero || s.Total != 2 {
		t.Fatalf("start = %+v", s)
	}
	r.c.Next()
	r.move(2, 1003)
	// The second knob's 100% is its low end: it is wired the other way round.
	r.move(1, 2)
	if s := r.c.State(); s.Stage != CalFull {
		t.Fatalf("after the 0%% reading = %+v", s)
	}
	r.c.Next()
	if s := r.c.State(); s.Stage != CalFind || s.Control != 0 || s.Index != 0 {
		t.Fatalf("after the 100%% reading = %+v", s)
	}
	r.move(2, 9)
	if s := r.c.State(); s.Stage != CalFind || s.Control != 1 {
		t.Fatalf("after the first knob = %+v, want the next one", s)
	}
	r.move(1, 1010)
	if !r.c.Done() {
		t.Fatalf("not done: %+v", r.c.State())
	}
	got := r.c.Result()
	if got[0].Input != 2 || got[0].Reverse || got[0].Min != 9 || got[0].Max != 1003 {
		t.Errorf("first knob = %+v, want input 2 from 9 to 1003", got[0])
	}
	if got[1].Input != 1 || !got[1].Reverse || got[1].Min != 2 || got[1].Max != 1010 {
		t.Errorf("second knob = %+v, want input 1, reversed, from 2 to 1010", got[1])
	}
}

func TestWizardTakesNoMoveBeforeBothReadings(t *testing.T) {
	// Turning a knob to 0% before the first Next is getting ready, not the knob's turn.
	r := newRun(t, diyBoard(1, 0), []int{0}, []int{100, 600})
	r.move(1, 1020)
	r.c.Next()
	r.move(1, 30)
	r.c.Next()
	r.move(1, 1020)
	got := r.c.Result()[0]
	if !r.c.Done() || got.Input != 1 || !got.Reverse || got.Min != 30 || got.Max != 1020 {
		t.Errorf("knob = %+v, want input 1, reversed, from 30 to 1020", got)
	}
}

func TestWizardGoesBackToZeroWhenNothingMoved(t *testing.T) {
	r := newRun(t, diyBoard(1, 0), []int{0}, []int{100, 600})
	r.c.Next()
	r.c.Next()
	if s := r.c.State(); s.Stage != CalZero || s.Warning != CalNothing {
		t.Errorf("nothing moved = %+v, want the 0%% reading again with a warning", s)
	}
	r.c.Next()
	r.move(1, 0)
	r.c.Next()
	r.c.Redo()
	if s := r.c.State(); s.Stage != CalZero || s.Warning != "" {
		t.Errorf("Redo = %+v, want the 0%% reading again", s)
	}
}

func TestWizardWarnsOfTheWrongAndAnUnsweptControl(t *testing.T) {
	d := diyBoard(2, 0)
	d.Controls[0].Input = 0
	r := newRun(t, d, []int{1}, []int{20, 20, 20, 20})
	r.c.Next()
	r.move(0, 900)
	r.move(1, 900)
	r.c.Next()
	for i := range 50 {
		r.frame[1] = 900 - i%5
		r.c.Feed(r.frame)
	}
	if s := r.c.State(); s.Stage != CalFind || s.Warning != "" {
		t.Fatalf("jitter = %+v, want still looking", s)
	}
	r.move(0, 20)
	if s := r.c.State(); s.Warning != CalWrong || s.Other != 0 || s.Stage != CalFind {
		t.Errorf("moving the first knob's input = %+v, want a warning naming it", s)
	}
	r.move(3, 900)
	if s := r.c.State(); s.Warning != CalUnswept || s.Stage != CalFind {
		t.Errorf("moving an input that stayed put between the readings = %+v", s)
	}
	r.move(3, 20)
	r.move(1, 20)
	if got := r.c.Result()[1]; !r.c.Done() || got.Input != 1 || got.Min != 20 || got.Max != 900 {
		t.Errorf("second knob = %+v, want input 1 from 20 to 900", got)
	}
}

func TestWizardTakesThreePressesOfOneButton(t *testing.T) {
	r := newRun(t, diyBoard(0, 1), []int{0}, []int{500, 0, 0})
	if s := r.c.State(); s.Stage != CalPress {
		t.Fatalf("a board of buttons starts at %+v, want its first press", s)
	}
	press := func(col int) {
		r.move(col, 1023)
		r.move(col, 0)
	}
	press(1)
	press(2)
	if s := r.c.State(); s.Warning != CalMismatch || s.Count != 0 {
		t.Fatalf("a press on another button = %+v, want a mismatch and a fresh start", s)
	}
	press(2)
	press(2)
	if s := r.c.State(); s.Count != 2 || s.Warning != "" {
		t.Fatalf("two presses = %+v", s)
	}
	press(2)
	if !r.c.Done() || r.c.Result()[0].Input != 2 {
		t.Errorf("three presses gave %+v", r.c.Result())
	}
}

func TestWizardOnAMIDIBoard(t *testing.T) {
	d := NewDevice("d2", "Pads", DeviceMIDI, 1, 0, 1, "Default")
	c := NewCalibration(d, []int{0, 1})
	frame := make([]int, MixerColumns)
	for i := range frame {
		frame[i] = -1
	}
	feed := func(col, v int) {
		frame[col] = v
		c.Feed(frame)
	}
	// Already at 0%, the knob sends nothing before the first Next.
	c.Next()
	for _, v := range []int{8, 200, 600, 1023} {
		feed(7, v)
	}
	c.Next()
	if s := c.State(); s.Stage != CalFind {
		t.Fatalf("after both readings = %+v", s)
	}
	feed(7, 900)
	if s := c.State(); s.Control != 1 || s.Stage != CalPress {
		t.Fatalf("after turning the knob = %+v, want the button", s)
	}
	// The button's id may equal a pot's column: they are apart on a MIDI board.
	for range 3 {
		c.Press(7)
	}
	got := c.Result()
	if got[0].Input != 7 || got[0].Reverse || got[0].Min != 0 || got[0].Max != 1023 || got[1].Input != 7 || !c.Done() {
		t.Errorf("result = %+v", got)
	}
}

func TestWizardSkipKeepsWhatWasThere(t *testing.T) {
	d := diyBoard(0, 2)
	d.Controls[0].Input = 4
	c := NewCalibration(d, []int{0, 1})
	c.Skip()
	if s := c.State(); s.Control != 1 || s.Index != 1 {
		t.Fatalf("skip = %+v", s)
	}
	c.Skip()
	if !c.Done() || !reflect.DeepEqual(c.Result(), d.Controls) {
		t.Errorf("skipping everything changed %+v", c.Result())
	}
}

func TestHotkeyBindingsShareOneCombination(t *testing.T) {
	f1 := &Shortcut{VK: 0x70, Mods: 2, Key: "F1"}
	a := NewDevice("d1", "Desk", DeviceDIY, 1, 0, 0, "One")
	a.Profiles = append(a.Profiles, a.NewProfile("Two"))
	a.Profiles[1].Shortcut = f1
	a.Next = &Shortcut{VK: 0x71, Mods: 2, Key: "F2"}
	b := NewDevice("d2", "SMC", DeviceSMC, 0, 0, 0, "One")
	b.Profiles[0].Shortcut = f1
	off := NewDevice("d3", "Spare", DeviceDIY, 1, 0, 0, "One")
	off.Enabled, off.Next = false, f1
	got := HotkeyBindings([]Device{a, b, off})
	want := []HotkeyBinding{
		{Shortcut: *a.Next, Targets: []HotkeyTarget{{Device: "d1", Step: 1}}},
		{Shortcut: *f1, Targets: []HotkeyTarget{{Device: "d1", Profile: 1}, {Device: "d2", Profile: 0}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bindings = %+v, want %+v", got, want)
	}
}
