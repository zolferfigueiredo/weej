package core

import "math"

const turnSeconds = 20.0

// Kept free of real time and I/O, unlike Calibrator.swift's caller, so tests can drive it with
// fake lines and a fake clock.
type Calibrator struct {
	saved   []int
	onlyNew bool

	first     int
	found     []int
	phase     int
	left      float64
	full      bool
	wrongKnob int

	low       []int
	high      []int
	anchor    int
	lastMove  float64
	lastTime  float64
	stepStart float64
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func NewCalibrator(saved []int, onlyNew bool) *Calibrator {
	c := &Calibrator{
		saved:     saved,
		onlyNew:   onlyNew,
		left:      turnSeconds,
		wrongKnob: -1,
		anchor:    -1,
		lastMove:  math.Inf(-1),
		stepStart: math.Inf(1),
	}
	c.skipKept()
	c.first = c.Knob()
	return c
}

func (c *Calibrator) Knob() int {
	if c.phase == 0 {
		return len(c.found)
	}
	return len(c.found) - 1
}

func (c *Calibrator) Phase() int     { return c.phase }
func (c *Calibrator) Full() bool     { return c.full }
func (c *Calibrator) Left() float64  { return c.left }
func (c *Calibrator) WrongKnob() int { return c.wrongKnob }
func (c *Calibrator) First() int     { return c.first }
func (c *Calibrator) Found() []int   { return append([]int{}, c.found...) }

func (c *Calibrator) Paused() bool {
	if c.phase != 1 {
		return false
	}
	m := c.lastMove
	if c.stepStart > m {
		m = c.stepStart
	}
	return c.lastTime-m > 1
}

// Only a knob found in this run, or found before it on a column no other knob here has taken:
// otherwise the knob asked for isn't there, and Finish is what's left.
func (c *Calibrator) CanSkip() bool {
	if c.full {
		return false
	}
	if c.phase > 0 {
		return true
	}
	knob := c.Knob()
	if knob >= len(c.saved) || c.saved[knob] < 0 {
		return false
	}
	return !containsInt(c.found, c.saved[knob])
}

func (c *Calibrator) Result() []int {
	result := append([]int{}, c.found...)
	for i := len(c.found); i < len(c.saved); i++ {
		col := c.saved[i]
		if col >= 0 && !containsInt(c.found, col) {
			result = append(result, col)
		} else {
			result = append(result, -1)
		}
	}
	return result
}

type StepKey struct {
	Knob  int
	Phase int
	Full  bool
}

func (c *Calibrator) StepKey() StepKey { return StepKey{c.Knob(), c.phase, c.full} }

func (c *Calibrator) Feed(values []int, now float64) {
	if c.full {
		return
	}
	if c.phase == 0 {
		c.feedSearch(values)
		return
	}

	if len(c.found) == 0 {
		return
	}
	column := c.found[len(c.found)-1]
	if column >= len(values) {
		return
	}
	value := values[column]
	if c.anchor < 0 {
		c.anchor = value
		c.stepStart = now
	}
	if now-c.lastMove <= 1 {
		c.left -= now - c.lastTime
	}
	if absInt(value-c.anchor) >= 10 {
		c.anchor = value
		c.lastMove = now
	}
	c.lastTime = now
	if c.left <= 0 {
		c.next()
	}
}

func (c *Calibrator) feedSearch(values []int) {
	if len(c.low) != len(values) {
		c.low = append([]int{}, values...)
		c.high = append([]int{}, values...)
	}
	for i, v := range values {
		// A mixer control that has not moved yet reads -1; its first real value is a start, not a swing.
		if v < 0 {
			continue
		}
		if c.low[i] < 0 {
			c.low[i], c.high[i] = v, v
		}
		if v < c.low[i] {
			c.low[i] = v
		}
		if v > c.high[i] {
			c.high[i] = v
		}
	}

	best, bestSwing := -1, -1
	for i := range values {
		if containsInt(c.found, i) {
			continue
		}
		if swing := c.high[i] - c.low[i]; swing > bestSwing {
			best, bestSwing = i, swing
		}
	}
	if best == -1 {
		c.full = true
		return
	}

	// A found knob moving: say so and start over, so a pin echoing it can't pass for the next knob.
	for fi, col := range c.found {
		if col < len(values) && c.high[col]-c.low[col] >= 200 {
			c.wrongKnob = fi
			c.low = append([]int{}, values...)
			c.high = append([]int{}, values...)
			return
		}
	}

	if bestSwing >= 512 {
		c.found = append(c.found, best)
		c.next()
	}
}

func (c *Calibrator) Skip() {
	if !c.CanSkip() {
		return
	}
	if c.phase == 0 {
		c.found = append(c.found, c.saved[c.Knob()])
	}
	c.reset(0)
	c.skipKept()
}

func (c *Calibrator) next() {
	if c.phase == 1 {
		c.reset(0)
	} else {
		c.reset(1)
	}
	c.skipKept()
}

func (c *Calibrator) skipKept() {
	for c.onlyNew && c.phase == 0 && c.CanSkip() {
		c.found = append(c.found, c.saved[c.Knob()])
		c.reset(0)
	}
}

func (c *Calibrator) reset(phase int) {
	c.phase = phase
	c.low = nil
	c.high = nil
	c.left = turnSeconds
	c.anchor = -1
	c.lastMove = math.Inf(-1)
	c.stepStart = math.Inf(1)
	c.wrongKnob = -1
}
