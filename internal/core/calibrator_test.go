package core

import (
	"math"
	"reflect"
	"testing"
)

func TestCalibrationRun(t *testing.T) {
	run := NewCalibrator(nil, false)
	clock := 0.0
	tick := func(values []int) { clock += 0.03; run.Feed(values, clock) }

	tick([]int{500, 500, 500})
	tick([]int{520, 500, 100})
	if run.Phase() != 0 || run.CanSkip() {
		t.Fatalf("after a 400-swing: phase=%d canSkip=%v", run.Phase(), run.CanSkip())
	}
	tick([]int{520, 500, 1000})
	if !reflect.DeepEqual(run.Found(), []int{2}) || run.Phase() != 1 || !run.CanSkip() || run.Paused() {
		t.Fatalf("after knob A found: found=%v phase=%d canSkip=%v paused=%v", run.Found(), run.Phase(), run.CanSkip(), run.Paused())
	}
	tick([]int{520, 500, 1000})
	if run.Paused() {
		t.Fatal("should not pause before the knob has had a second")
	}
	for i := 0; i < 1000; i++ {
		tick([]int{520, 500, 1000}) // 30 seconds untouched
	}
	if run.Phase() != 1 || run.Left() != turnSeconds || !run.Paused() {
		t.Fatalf("after 30s untouched: phase=%d left=%v paused=%v", run.Phase(), run.Left(), run.Paused())
	}
	tick([]int{520, 500, 600})
	if run.Paused() {
		t.Fatal("should un-pause once it moves")
	}
	turning := 400
	for run.Phase() == 1 {
		turning = 1000 - turning
		tick([]int{520, 500, turning})
	}
	if math.Abs(clock-30-turnSeconds) >= 1 {
		t.Errorf("clock = %v, want close to %v", clock, 30+turnSeconds)
	}
	if run.Knob() != 1 || run.Phase() != 0 {
		t.Fatalf("after turning finished: knob=%d phase=%d", run.Knob(), run.Phase())
	}

	tick([]int{520, 500, 0})
	tick([]int{520, 500, 1023})
	if run.Phase() != 0 || run.WrongKnob() != 0 {
		t.Fatalf("knob A's column moving: phase=%d wrongKnob=%d", run.Phase(), run.WrongKnob())
	}
	tick([]int{0, 500, 1023})
	if !reflect.DeepEqual(run.Found(), []int{2, 0}) || run.Knob() != 1 || run.Phase() != 1 || run.WrongKnob() != -1 {
		t.Fatalf("after knob B found: found=%v knob=%d phase=%d wrongKnob=%d", run.Found(), run.Knob(), run.Phase(), run.WrongKnob())
	}
	run.Skip()
	if run.Knob() != 2 || run.Phase() != 0 || run.Full() {
		t.Fatalf("after skipping knob B's turning: knob=%d phase=%d full=%v", run.Knob(), run.Phase(), run.Full())
	}
	run.Skip()
	if run.Knob() != 2 || run.Phase() != 0 {
		t.Fatalf("skip with nothing to skip: knob=%d phase=%d", run.Knob(), run.Phase())
	}
	tick([]int{0, 0, 1023})
	tick([]int{0, 1023, 1023})
	if !reflect.DeepEqual(run.Found(), []int{2, 0, 1}) {
		t.Fatalf("found = %v, want [2 0 1]", run.Found())
	}
	run.Skip()
	tick([]int{0, 1023, 1023})
	if !run.Full() || run.Phase() != 0 || run.CanSkip() || !reflect.DeepEqual(run.Result(), []int{2, 0, 1}) {
		t.Fatalf("at the end: full=%v phase=%d canSkip=%v result=%v", run.Full(), run.Phase(), run.CanSkip(), run.Result())
	}
}

func TestCalibrationOfOnlyNewKnobs(t *testing.T) {
	run := NewCalibrator([]int{0, 3, 2, 4, -1}, true)
	if !reflect.DeepEqual(run.Found(), []int{0, 3, 2, 4}) || run.Knob() != 4 || run.First() != 4 || run.Phase() != 0 || run.CanSkip() {
		t.Fatalf("initial: found=%v knob=%d first=%d phase=%d canSkip=%v", run.Found(), run.Knob(), run.First(), run.Phase(), run.CanSkip())
	}
	run.Feed([]int{500, 500, 500, 500, 500}, 0)
	run.Feed([]int{500, 1023, 500, 500, 500}, 0.03)
	if !reflect.DeepEqual(run.Found(), []int{0, 3, 2, 4, 1}) || run.Phase() != 1 {
		t.Fatalf("after knob found: found=%v phase=%d", run.Found(), run.Phase())
	}
	run.Skip()
	run.Feed([]int{500, 1023, 500, 500, 500}, 0.06)
	if !run.Full() || !reflect.DeepEqual(run.Result(), []int{0, 3, 2, 4, 1}) {
		t.Fatalf("at the end: full=%v result=%v", run.Full(), run.Result())
	}

	middle := NewCalibrator([]int{0, -1, 2}, true)
	if middle.Knob() != 1 {
		t.Fatalf("middle knob = %d, want 1", middle.Knob())
	}
	middle.Feed([]int{500, 500, 500, 500}, 0)
	middle.Feed([]int{500, 500, 500, 1023}, 0.03)
	middle.Skip()
	if !reflect.DeepEqual(middle.Found(), []int{0, 3, 2}) || middle.Knob() != 3 {
		t.Fatalf("middle after skip: found=%v knob=%d", middle.Found(), middle.Knob())
	}
}

func TestOnlyAFoundKnobCanBeSkipped(t *testing.T) {
	run := NewCalibrator([]int{0, 3, 2, -1, -1}, true)
	if run.Knob() != 3 || run.CanSkip() || !reflect.DeepEqual(run.Result(), []int{0, 3, 2, -1, -1}) {
		t.Fatalf("initial: knob=%d canSkip=%v result=%v", run.Knob(), run.CanSkip(), run.Result())
	}
	run.Feed([]int{500, 500, 500, 500, 500}, 0)
	run.Feed([]int{500, 1023, 500, 500, 500}, 0.03)
	if run.Phase() != 1 || !run.CanSkip() {
		t.Fatalf("after found: phase=%d canSkip=%v", run.Phase(), run.CanSkip())
	}
	run.Skip()
	if run.Knob() != 4 || run.CanSkip() || !reflect.DeepEqual(run.Result(), []int{0, 3, 2, 1, -1}) {
		t.Fatalf("after skip: knob=%d canSkip=%v result=%v", run.Knob(), run.CanSkip(), run.Result())
	}
}

func TestCalibrationSkipsKnobsAlreadySetUp(t *testing.T) {
	run := NewCalibrator([]int{2, -1, 0, 3}, false)
	if !run.CanSkip() {
		t.Fatal("want canSkip for a knob with a saved column")
	}
	run.Skip()
	if !reflect.DeepEqual(run.Found(), []int{2}) || run.Knob() != 1 || run.Phase() != 0 {
		t.Fatalf("after skip: found=%v knob=%d phase=%d", run.Found(), run.Knob(), run.Phase())
	}
	if run.CanSkip() {
		t.Fatal("knob B has no column yet")
	}
	run.Feed([]int{500, 500, 500, 500}, 0)
	run.Feed([]int{500, 500, 500, 1023}, 0.03)
	if !reflect.DeepEqual(run.Found(), []int{2, 3}) || run.Phase() != 1 {
		t.Fatalf("knob B found on column 3: found=%v phase=%d", run.Found(), run.Phase())
	}
	run.Skip()
	if run.Knob() != 2 || !run.CanSkip() {
		t.Fatalf("knob C: knob=%d canSkip=%v", run.Knob(), run.CanSkip())
	}
	run.Skip()
	if !reflect.DeepEqual(run.Found(), []int{2, 3, 0}) || run.Knob() != 3 || run.CanSkip() {
		t.Fatalf("knob D: found=%v knob=%d canSkip=%v", run.Found(), run.Knob(), run.CanSkip())
	}
	if !reflect.DeepEqual(run.Result(), []int{2, 3, 0, -1}) {
		t.Fatalf("result = %v, want [2 3 0 -1]", run.Result())
	}
}
