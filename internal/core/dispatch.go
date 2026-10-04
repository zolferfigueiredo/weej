package core

import (
	"math"
	"strconv"
	"sync"
	"time"
)

type Applier interface {
	Apply(job Job, s float64)
	HUD(job Job, s float64)
}

type Debouncer struct {
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewDebouncer() *Debouncer { return &Debouncer{timers: map[string]*time.Timer{}} }

func (d *Debouncer) Debounce(key string, after time.Duration, fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.timers[key]; ok {
		t.Stop()
	}
	d.timers[key] = time.AfterFunc(after, fn)
}

func (d *Debouncer) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range d.timers {
		t.Stop()
	}
	d.timers = map[string]*time.Timer{}
}

type Line struct {
	Job     Job
	Percent int
}

// lastApplied is keyed by serial column, never by job, so a knob given a new job keeps its old
// value and the job only lands when the knob next moves (no jumps). Debounced Apply calls run
// later on the timer's goroutine.
type Engine struct {
	mu          sync.Mutex
	applier     Applier
	debouncer   *Debouncer
	lastApplied map[int]float64
	lastInvert  bool
	lines       []Line
}

func NewEngine(a Applier) *Engine {
	return &Engine{applier: a, debouncer: NewDebouncer(), lastApplied: map[int]float64{}}
}

func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastApplied = map[int]float64{}
}

func jobKey(job Job) string {
	return string(job.Kind) + ":" + strconv.Itoa(job.Screen) + ":" + job.Exe
}

// Display identity for "one HUD per display per knob change": brightness and contrast on the
// same screen share a display (so only the first of the two claims it there), but every screen
// index is its own display.
func displayKey(job Job) string {
	switch job.Kind {
	case JobBuiltinBrightness:
		return "builtin"
	case JobBrightness, JobContrast:
		return "screen:" + strconv.Itoa(job.Screen)
	case JobZoom:
		return "pointer"
	default:
		return "primary"
	}
}

func (e *Engine) Handle(values []int, setup Setup, calibrating bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	mapping := setup.Mapping()
	settle := setup.Speed.Settle()

	if setup.Invert != e.lastInvert {
		for k, v := range e.lastApplied {
			e.lastApplied[k] = 1 - v
		}
		e.lastInvert = setup.Invert
	}

	shown := map[string]bool{}
	once := func(display string, show func()) {
		if !shown[display] {
			shown[display] = true
			show()
		}
	}

	for index, raw := range values {
		scalar := float64(raw) / 1023.0
		u := scalar
		if !setup.Invert {
			u = 1 - scalar
		}
		switch {
		case u < 0.01:
			u = 0
		case u > 0.99:
			u = 1
		}

		jobs, mapped := mapping[index]
		if calibrating || !mapped {
			e.lastApplied[index] = u
			continue
		}
		// After a connect nothing is known yet, so every mapped knob applies its position once, as in TheeJ.
		previous, ok := e.lastApplied[index]
		if !ok {
			previous = -1
		}
		extreme := u <= 0 || u >= 1
		if u == previous {
			continue
		}
		if !extreme && math.Abs(u-previous) < 0.01 {
			continue
		}
		e.lastApplied[index] = u

		for _, job := range jobs {
			if job.Immediate() {
				e.applier.Apply(job, u)
			} else {
				e.debouncer.Debounce(jobKey(job), settle, func() { e.applier.Apply(job, u) })
			}
			once(displayKey(job), func() { e.applier.HUD(job, u) })
		}
	}

	if calibrating {
		return
	}

	var lines []Line
	for _, col := range setup.MenuOrder() {
		for _, job := range mapping[col] {
			lines = append(lines, Line{Job: job, Percent: Percent(e.lastApplied[col])})
		}
	}
	e.lines = lines
}

func (e *Engine) Lines() []Line {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Line{}, e.lines...)
}
