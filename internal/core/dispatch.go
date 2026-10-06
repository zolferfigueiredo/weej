package core

import (
	"math"
	"slices"
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
	// A muted column keeps recording its position, so unmuting puts it back where it now is.
	muted map[int]bool
	lines []Line
}

func NewEngine(a Applier) *Engine {
	return &Engine{applier: a, debouncer: NewDebouncer(), lastApplied: map[int]float64{}, muted: map[int]bool{}}
}

func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastApplied = map[int]float64{}
	e.muted = map[int]bool{}
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
		if raw < 0 {
			continue
		}
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
		if e.muted[index] {
			continue
		}

		for _, job := range jobs {
			e.apply(job, u, settle)
			once(displayKey(job), func() { e.applier.HUD(job, u) })
		}
	}

	if calibrating {
		return
	}

	var lines []Line
	for _, col := range setup.MenuOrder() {
		for _, job := range mapping[col] {
			percent := Percent(e.lastApplied[col])
			if e.muted[col] {
				percent = 0
			}
			lines = append(lines, Line{Job: job, Percent: percent})
		}
	}
	e.lines = lines
}

func (e *Engine) apply(job Job, u float64, settle time.Duration) {
	if job.Immediate() {
		e.applier.Apply(job, u)
	} else {
		e.debouncer.Debounce(jobKey(job), settle, func() { e.applier.Apply(job, u) })
	}
}

func (e *Engine) ToggleMute(col int, setup Setup) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	muted := !e.muted[col]
	if muted {
		e.muted[col] = true
	} else {
		delete(e.muted, col)
	}
	u := 0.0
	if !muted {
		last, ok := e.lastApplied[col]
		if !ok {
			return false
		}
		u = last
	}

	jobs := setup.Mapping()[col]
	settle := setup.Speed.Settle()
	shown := map[string]bool{}
	for _, job := range jobs {
		e.apply(job, u, settle)
		if d := displayKey(job); !shown[d] {
			shown[d] = true
			e.applier.HUD(job, u)
		}
	}
	for i, ln := range e.lines {
		if slices.Contains(jobs, ln.Job) {
			e.lines[i].Percent = Percent(u)
		}
	}
	return muted
}

func (e *Engine) Lines() []Line {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Line{}, e.lines...)
}
