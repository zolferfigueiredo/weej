package core

import (
	"reflect"
	"testing"
)

// run feeds a DIY wizard frames 10 ms apart, each from the previous one with the given changes.
type wizardRun struct {
	t     *testing.T
	c     *Calibration
	frame []int
	now   float64
}

func newRun(t *testing.T, d Device, controls []int, frame []int) *wizardRun {
	r := &wizardRun{t: t, c: NewCalibration(d, controls), frame: append([]int(nil), frame...)}
	r.c.Feed(r.frame, r.now)
	return r
}

// move slides col to v in steps, then holds it there for hold seconds.
func (r *wizardRun) move(col, v int, hold float64) {
	from := r.frame[col]
	for i := 1; i <= 10; i++ {
		r.frame[col] = from + (v-from)*i/10
		r.now += 0.01
		r.c.Feed(r.frame, r.now)
	}
	for end := r.now + hold; r.now < end; {
		r.now += 0.01
		r.c.Feed(r.frame, r.now)
	}
}

func diyBoard(knobs, buttons int) Device {
	return NewDevice("d1", "Desk", DeviceDIY, knobs, 0, buttons, "Default")
}

func TestWizardFindsAPotByTwoSweeps(t *testing.T) {
	r := newRun(t, diyBoard(2, 0), []int{0, 1}, []int{12, 1010, 9, 500})
	if s := r.c.State(); s.Stage != CalFind || s.Index != 0 || s.Total != 2 {
		t.Fatalf("start = %+v", s)
	}
	r.move(2, 1000, 0.1)
	if s := r.c.State(); s.Stage != CalSweep || s.Count != 1 || s.Level < 1000 {
		t.Fatalf("after the first sweep = %+v", s)
	}
	r.move(2, 15, 0.1)
	r.move(2, 1003, 0.4)
	// The second knob rests at its top: its 0% is there, so it is wired the other way round.
	if s := r.c.State(); s.Control != 1 || s.Stage != CalFind {
		t.Fatalf("after the second sweep = %+v, want the next knob", s)
	}
	r.move(1, 4, 0.1)
	r.move(1, 1010, 0.1)
	r.move(1, 2, 0.4)
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

func TestWizardIgnoresJitterAndWarnsOfTheWrongControl(t *testing.T) {
	d := diyBoard(2, 0)
	d.Controls[0].Input = 0
	r := newRun(t, d, []int{1}, []int{20, 20, 20})
	for i := range 50 {
		r.frame[2] = 20 + i%5
		r.now += 0.01
		r.c.Feed(r.frame, r.now)
	}
	if s := r.c.State(); s.Stage != CalFind || s.Warning != "" {
		t.Fatalf("jitter = %+v, want still looking", s)
	}
	r.move(0, 900, 0.1)
	if s := r.c.State(); s.Warning != CalWrong || s.Other != 0 || s.Stage != CalFind {
		t.Errorf("moving the first knob's input = %+v, want a warning naming it", s)
	}
	r.move(1, 900, 0.1)
	if s := r.c.State(); s.Warning != "" || s.Stage != CalSweep {
		t.Errorf("moving the right one = %+v", s)
	}
}

func TestWizardTakesThreePressesOfOneButton(t *testing.T) {
	r := newRun(t, diyBoard(0, 1), []int{0}, []int{500, 0, 0})
	press := func(col int) {
		r.move(col, 1023, 0.05)
		r.move(col, 0, 0.05)
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
	now := 0.0
	feed := func(col, v int) {
		frame[col] = v
		now += 0.02
		c.Feed(frame, now)
	}
	for _, v := range []int{8, 40, 200, 600, 1023, 700, 100, 0, 300, 1023} {
		feed(7, v)
	}
	if s := c.State(); s.Stage != CalSweep || s.Count != 2 {
		t.Fatalf("after two sweeps = %+v", s)
	}
	c.Tick(now + 0.1)
	if c.State().Control != 0 {
		t.Fatal("finished before resting")
	}
	c.Tick(now + 0.5)
	if s := c.State(); s.Control != 1 || s.Stage != CalPress {
		t.Fatalf("after resting = %+v, want the button", s)
	}
	// The button's id may equal a pot's column: they are apart on a MIDI board.
	for range 3 {
		c.Press(7)
	}
	got := c.Result()
	if got[0].Input != 7 || got[0].Reverse || got[0].Max != 1023 || got[1].Input != 7 || !c.Done() {
		t.Errorf("result = %+v", got)
	}
}

func TestWizardSkipKeepsWhatWasThere(t *testing.T) {
	d := diyBoard(2, 0)
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
