package core

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Applier interface {
	Apply(job Job, s float64)
	HUD(job Job, s float64)
	// Display names the monitor job's HUD shows on, or "" when it shows none.
	Display(job Job) string
	// Level is where job is now, 0 to 1, when that can be read.
	Level(job Job) (float64, bool)
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
	// A muted column keeps recording its position, so unmuting puts it back where it now is.
	muted map[int]bool
	lines []Line
	// Trace, when set, is told what the engine makes of each reading that changed, and why.
	Trace func(format string, args ...any)
	// traced is each column's reading as last traced, so a reading that comes back the same isn't.
	traced map[int]int
}

func (e *Engine) tracef(format string, args ...any) {
	if e.Trace != nil {
		e.Trace(format, args...)
	}
}

// fresh tells whether a column's reading differs from the one last traced, and notes it.
func (e *Engine) fresh(col, raw int) bool {
	if e.Trace == nil {
		return false
	}
	if last, ok := e.traced[col]; ok && last == raw {
		return false
	}
	if e.traced == nil {
		e.traced = map[int]int{}
	}
	e.traced[col] = raw
	return true
}

func jobKeys(jobs []Job) string {
	keys := make([]string, len(jobs))
	for i, j := range jobs {
		keys[i] = jobKey(j)
	}
	return strings.Join(keys, ", ")
}

func NewEngine(a Applier) *Engine {
	return &Engine{applier: a, debouncer: NewDebouncer(), lastApplied: map[int]float64{}, muted: map[int]bool{}}
}

func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastApplied = map[int]float64{}
	e.muted = map[int]bool{}
	e.traced = nil
}

func jobKey(job Job) string {
	return string(job.Kind) + ":" + strconv.Itoa(job.Screen) + ":" + job.Exe
}

// hud shows one HUD per monitor per change, the first job's there.
func (e *Engine) hud(shown map[string]bool, job Job, u float64) {
	if d := e.applier.Display(job); d != "" && !shown[d] {
		shown[d] = true
		e.applier.HUD(job, u)
	}
}

func (e *Engine) Handle(values []int, setup Setup, calibrating bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	mapping := setup.Mapping()
	settle := setup.Speed.Settle()

	shown := map[string]bool{}
	for index, raw := range values {
		if raw < 0 {
			continue
		}
		u := float64(raw) / 1023.0
		switch {
		case u < 0.01:
			u = 0
		case u > 0.99:
			u = 1
		}

		jobs, mapped := mapping[index]
		previous, ok := e.lastApplied[index]
		fresh := e.fresh(index, raw)
		if calibrating || !mapped {
			if fresh {
				e.tracef("control %d: %d (%.3f), not applied: calibrating %v, no job %v", index, raw, u, calibrating, !mapped)
			}
			e.lastApplied[index] = u
			continue
		}
		// The first reading after a connect is only a baseline, so nothing changes until a turn.
		if !ok {
			if fresh {
				e.tracef("control %d: %d (%.3f), first reading, kept as the baseline", index, raw, u)
			}
			e.lastApplied[index] = u
			continue
		}
		extreme := u <= 0 || u >= 1
		if u == previous {
			continue
		}
		if !extreme && math.Abs(u-previous) < 0.01 {
			if fresh {
				e.tracef("control %d: %.3f to %.3f (%d), under 1%%, ignored", index, previous, u, raw)
			}
			continue
		}
		e.lastApplied[index] = u
		if e.muted[index] {
			e.tracef("control %d: %.3f to %.3f (%d), muted, held", index, previous, u, raw)
			continue
		}
		e.tracef("control %d: %.3f to %.3f (%d), applied to %s", index, previous, u, raw, jobKeys(jobs))

		for _, job := range jobs {
			e.apply(job, u, settle)
			e.hud(shown, job, u)
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

// UnmuteAll puts every muted column's jobs back to its position, with no HUD, and reports how
// many were muted.
func (e *Engine) UnmuteAll(setup Setup) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := len(e.muted)
	mapping := setup.Mapping()
	settle := setup.Speed.Settle()
	for col := range e.muted {
		if last, ok := e.lastApplied[col]; ok {
			e.tracef("control %d: unmuted with the others, back to %.3f, applied to %s", col, last, jobKeys(mapping[col]))
			for _, job := range mapping[col] {
				e.apply(job, last, settle)
			}
		}
	}
	e.muted = map[int]bool{}
	return n
}

func (e *Engine) ToggleMute(col int, setup Setup) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	jobs := setup.Mapping()[col]
	muted := !e.muted[col]
	if muted {
		e.muted[col] = true
		// A control not heard from yet unmutes to where its first job was.
		if _, ok := e.lastApplied[col]; !ok && len(jobs) > 0 {
			if level, ok := e.applier.Level(jobs[0]); ok {
				e.lastApplied[col] = level
			}
		}
	} else {
		delete(e.muted, col)
	}
	u := 0.0
	if !muted {
		last, ok := e.lastApplied[col]
		if !ok {
			e.tracef("control %d: unmuted, never heard from, left as it is", col)
			return false
		}
		u = last
	}
	e.tracef("control %d: muted %v, set to %.3f, applied to %s", col, muted, u, jobKeys(jobs))

	settle := setup.Speed.Settle()
	shown := map[string]bool{}
	for _, job := range jobs {
		e.apply(job, u, settle)
		e.hud(shown, job, u)
	}
	for i, ln := range e.lines {
		if slices.Contains(jobs, ln.Job) {
			e.lines[i].Percent = Percent(u)
		}
	}
	return muted
}

// Last is a column's level as the engine last took it, 0 to 1, once it has been heard from.
func (e *Engine) Last(col int) (float64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	u, ok := e.lastApplied[col]
	return u, ok
}

func (e *Engine) Lines() []Line {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Line{}, e.lines...)
}
