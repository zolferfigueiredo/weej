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
	// Changed, when set, hears of each control that changes its jobs: from where to where.
	Changed func(col int, from, to float64, jobs []Job)

	// HoldJumps holds a reading that jumps further than holdJump until the next one confirms it,
	// for a board that sends every value all the time, as an Arduino does: a dirty pot or a lost
	// character gives a single stray reading, and the volumes would follow it at once.
	HoldJumps bool
	held      map[int]float64
	// Jump, when set, hears of a reading held back (dropped false) and of one dropped, its next
	// reading having gone back (dropped true).
	Jump func(col int, from, to float64, dropped bool)
}

// holdJump is how far one reading may move a control before it waits for the next to confirm it.
const holdJump = 0.1

func (e *Engine) changed(col int, from, to float64, jobs []Job) {
	if e.Changed != nil {
		e.Changed(col, from, to, jobs)
	}
}

func NewEngine(a Applier) *Engine {
	return &Engine{applier: a, debouncer: NewDebouncer(), lastApplied: map[int]float64{}, muted: map[int]bool{}}
}

func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastApplied = map[int]float64{}
	e.muted = map[int]bool{}
	e.held = nil
}

// holdJump tells whether to hold back this reading: the first that jumps further than holdJump.
// The next reading lands either way, the jump dropped when it went back.
func (e *Engine) holdJump(col int, previous, u float64) bool {
	if !e.HoldJumps {
		return false
	}
	held, waiting := e.held[col]
	far := math.Abs(u-previous) > holdJump
	if !waiting {
		if !far {
			return false
		}
		if e.held == nil {
			e.held = map[int]float64{}
		}
		e.held[col] = u
		e.jump(col, previous, u, false)
		return true
	}
	delete(e.held, col)
	if !far {
		e.jump(col, previous, held, true)
	}
	return false
}

func (e *Engine) jump(col int, from, to float64, dropped bool) {
	if e.Jump != nil {
		e.Jump(col, from, to, dropped)
	}
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
		if calibrating || !mapped {
			e.lastApplied[index] = u
			continue
		}
		// The first reading after a connect is only a baseline, so nothing changes until a turn.
		previous, ok := e.lastApplied[index]
		if !ok {
			e.lastApplied[index] = u
			continue
		}
		if e.holdJump(index, previous, u) {
			continue
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
		e.changed(index, previous, u, jobs)

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
			e.changed(col, 0, last, mapping[col])
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
			return false
		}
		u = last
		e.changed(col, 0, u, jobs)
	} else {
		e.changed(col, e.lastApplied[col], 0, jobs)
	}

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
