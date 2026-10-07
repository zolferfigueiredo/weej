package core

// Calibration finds a board's controls one at a time. It starts with every pot at 0%, so a pot's
// first move away from where it was tells which way its 100% lies. A pot is then moved to 100%,
// back and to 100% again; a button is pressed three times.
type Calibration struct {
	midi     bool
	controls []Control
	order    []int
	pos      int
	// base is each input's value when the current control's turn began, or when a MIDI input
	// first reported, which is where its first move started.
	base []int
	last []int

	input   int
	dir     int
	start   int
	lo, hi  int
	out     bool
	count   int
	still   float64
	held    int
	warning string
	other   int
}

// CalStage is where the current control is in its turn.
type CalStage string

const (
	CalFind  CalStage = "find"
	CalSweep CalStage = "sweep"
	CalPress CalStage = "press"
	CalDone  CalStage = "done"
)

// Warnings State reports.
const (
	CalWrong    = "wrong"    // an input another control has moved
	CalMismatch = "mismatch" // a press on a different button than the first two
)

// CalState is what Settings shows. Level is the pot's travel from 0 to 1023 as found so far.
type CalState struct {
	Control int
	Kind    ControlKind
	Stage   CalStage
	Count   int
	Need    int
	Level   int
	Warning string
	Other   int
	Index   int
	Total   int
}

const (
	calFindDIY  = 300  // a pot moving this far from where it was is the one being found
	calFindMIDI = 64   // MIDI holds still, so a smaller move will do
	calWrongDIY = 200  // a found pot moving this far is the wrong control
	calOut      = 409  // 40% of the travel away from 0% counts as reaching 100%
	calBack     = 0.25 // within this share of the travel from 0% is back at 0%
	calJitter   = 6    // what a resting pot wanders by
	calRest     = 0.3  // seconds a pot stays still at 100% before its second sweep is taken
	pressAt     = 400  // a DIY button reads this far from its rest when held, as ButtonWatcher
	releaseAt   = 200
)

// NewCalibration goes through the given controls of a board, in order; the others keep their
// inputs and aren't moved to.
func NewCalibration(d Device, controls []int) *Calibration {
	c := &Calibration{midi: d.Type.IsMIDI(), controls: append([]Control(nil), d.Controls...)}
	for _, i := range controls {
		if i >= 0 && i < len(c.controls) {
			c.order = append(c.order, i)
		}
	}
	c.begin()
	return c
}

func (c *Calibration) current() int {
	if c.pos >= len(c.order) {
		return -1
	}
	return c.order[c.pos]
}

func (c *Calibration) begin() {
	c.input, c.dir, c.out, c.count, c.held = -1, 0, false, 0, -1
	c.warning, c.other = "", -1
	c.base = append([]int(nil), c.last...)
}

// taken is the other control an input belongs to, or -1. A DIY board's controls share its
// columns; a MIDI board's buttons are ids, apart from its pots' columns.
func (c *Calibration) taken(input int, button bool) int {
	cur := c.current()
	for i, ctl := range c.controls {
		if i == cur || ctl.Input != input || c.midi && (ctl.Kind == KindButton) != button {
			continue
		}
		return i
	}
	return -1
}

func (c *Calibration) next() {
	c.pos++
	c.begin()
}

// Skip leaves the current control as it was and goes on to the next.
func (c *Calibration) Skip() {
	if c.current() >= 0 {
		c.next()
	}
}

// Redo starts the current control's turn again, after the pot is back at 0%.
func (c *Calibration) Redo() { c.begin() }

// Feed takes a raw frame: a DIY board's line, or a MIDI board's MixerState values.
func (c *Calibration) Feed(raw []int, now float64) {
	if len(c.last) < len(raw) {
		c.last = append(c.last, make([]int, len(raw)-len(c.last))...)
		for i := len(c.base); i < len(raw); i++ {
			c.base = append(c.base, -1)
		}
	}
	for i, v := range raw {
		if c.base[i] < 0 && v >= 0 {
			c.base[i] = v
		}
	}
	defer func() { copy(c.last, raw) }()
	k := c.current()
	if k < 0 {
		return
	}
	if c.controls[k].Kind == KindButton {
		if !c.midi {
			c.feedDIYButton(raw)
		}
		return
	}
	c.feedPot(raw, now)
}

func (c *Calibration) moved(raw []int, col int) int {
	if col >= len(raw) || raw[col] < 0 || c.base[col] < 0 {
		return 0
	}
	return raw[col] - c.base[col]
}

func (c *Calibration) feedPot(raw []int, now float64) {
	find, wrong := calFindDIY, calWrongDIY
	if c.midi {
		find, wrong = calFindMIDI, calFindMIDI
	}
	if c.input < 0 {
		best, bestCol := 0, -1
		for col := range raw {
			d := c.moved(raw, col)
			if abs(d) < find && abs(d) < wrong {
				continue
			}
			if o := c.taken(col, false); o >= 0 {
				if abs(d) >= wrong {
					c.warning, c.other = CalWrong, o
				}
				continue
			}
			if abs(d) >= find && abs(d) > abs(best) {
				best, bestCol = d, col
			}
		}
		if bestCol < 0 {
			return
		}
		c.input, c.start, c.warning = bestCol, c.base[bestCol], ""
		c.dir = 1
		if best < 0 {
			c.dir = -1
		}
		c.lo, c.hi = c.start, c.start
	}
	v := raw[c.input]
	if v < 0 {
		return
	}
	c.lo, c.hi = min(c.lo, v), max(c.hi, v)
	travel := (v - c.start) * c.dir
	reach := max((c.hi-c.start)*c.dir, (c.lo-c.start)*c.dir)
	switch {
	case !c.out && travel >= calOut:
		c.out, c.count, c.still = true, c.count+1, now
	case c.out && c.count < 2 && float64(travel) <= calBack*float64(reach):
		c.out = false
	}
	if c.count == 2 && c.out {
		if prev := c.last[c.input]; abs(v-prev) > calJitter {
			c.still = now
		}
		c.Tick(now)
	}
}

// Tick lets a pot resting at 100% finish its turn when no frames come, as MIDI sends none then.
func (c *Calibration) Tick(now float64) {
	k := c.current()
	if k < 0 || c.input < 0 || c.count < 2 || !c.out || now-c.still < calRest {
		return
	}
	ctl := &c.controls[k]
	ctl.Input, ctl.Min, ctl.Max, ctl.Reverse = c.input, c.lo, c.hi, c.dir < 0
	c.next()
}

func (c *Calibration) feedDIYButton(raw []int) {
	if c.held >= 0 {
		if abs(c.moved(raw, c.held)) < releaseAt {
			c.held = -1
		}
		return
	}
	for col := range raw {
		if abs(c.moved(raw, col)) >= pressAt {
			c.held = col
			c.press(col)
			return
		}
	}
}

// Press takes a MIDI button's press, by its id.
func (c *Calibration) Press(id int) {
	k := c.current()
	if k < 0 || c.controls[k].Kind != KindButton || !c.midi {
		return
	}
	c.press(id)
}

func (c *Calibration) press(input int) {
	if o := c.taken(input, true); o >= 0 {
		c.warning, c.other = CalWrong, o
		return
	}
	switch {
	case c.input >= 0 && input != c.input:
		c.input, c.count, c.warning = -1, 0, CalMismatch
		return
	case c.input < 0:
		c.input, c.count = input, 1
	default:
		c.count++
	}
	c.warning = ""
	if c.count == 3 {
		k := c.current()
		c.controls[k].Input = input
		c.next()
	}
}

func (c *Calibration) State() CalState {
	k := c.current()
	s := CalState{Control: k, Stage: CalDone, Index: c.pos, Total: len(c.order), Warning: c.warning, Other: c.other}
	if k < 0 {
		return s
	}
	s.Kind = c.controls[k].Kind
	switch {
	case s.Kind == KindButton:
		s.Stage, s.Need, s.Count = CalPress, 3, c.count
	case c.input < 0:
		s.Stage, s.Need = CalFind, 2
	default:
		s.Stage, s.Need, s.Count = CalSweep, 2, c.count
		if c.input < len(c.last) && c.last[c.input] >= 0 {
			reach := max(abs(c.hi-c.start), abs(c.lo-c.start), calOut)
			s.Level = min(max((c.last[c.input]-c.start)*c.dir*1023/reach, 0), 1023)
		}
	}
	return s
}

func (c *Calibration) Done() bool { return c.current() < 0 }

// Result is the board's controls with what was found; a skipped control keeps what it had.
func (c *Calibration) Result() []Control { return append([]Control(nil), c.controls...) }
