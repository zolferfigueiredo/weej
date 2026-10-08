package core

// Calibration finds a board's controls. Every knob and fader is read once at 0% and once at
// 100%, each confirmed with Next, which gives each input its ends and its direction; then each
// is told apart by turning it back to 0%. A button is pressed three times.
type Calibration struct {
	midi     bool
	original []Control
	controls []Control
	order    []int
	pos      int
	// stage is CalZero or CalFull while the board is read at 0% and at 100%, and empty once
	// both are, when the controls are found one at a time.
	stage CalStage
	zero  []int
	full  []int
	// base is each input's value when the current control's turn began, or when a MIDI input
	// first reported, which is where its first move started.
	base []int
	last []int

	input   int
	count   int
	held    int
	warning string
	other   int
}

// CalStage is where the calibration is.
type CalStage string

const (
	CalZero  CalStage = "zero"
	CalFull  CalStage = "full"
	CalFind  CalStage = "find"
	CalPress CalStage = "press"
	CalDone  CalStage = "done"
)

// Warnings State reports.
const (
	CalWrong    = "wrong"    // an input another control has
	CalMismatch = "mismatch" // a press on a different button than the first two
	CalNothing  = "nothing"  // no input moved between the 0% and 100% readings
	CalUnswept  = "unswept"  // the input turned didn't move between the 0% and 100% readings
)

// CalState is what Settings shows.
type CalState struct {
	Control int
	Kind    ControlKind
	Stage   CalStage
	Count   int
	Need    int
	Warning string
	Other   int
	Index   int
	Total   int
}

const (
	calSwept    = 300 // an input this far apart at 0% and 100% is a knob or fader
	calFindDIY  = 300 // a pot moving this far from where it was is the one being found
	calFindMIDI = 64  // MIDI holds still, so a smaller move will do
	calWrongDIY = 200 // a found pot moving this far is the wrong control
	pressAt     = 400 // a DIY button reads this far from its rest when held, as ButtonWatcher
	releaseAt   = 200
)

// NewCalibration goes through the given controls of a board, in order; the others keep their
// inputs and aren't moved to.
func NewCalibration(d Device, controls []int) *Calibration {
	c := &Calibration{midi: d.Type.IsMIDI(), original: append([]Control(nil), d.Controls...)}
	for _, i := range controls {
		if i >= 0 && i < len(c.original) {
			c.order = append(c.order, i)
		}
	}
	c.restart()
	return c
}

// restart begins again from the 0% reading, with the controls being found cleared so their old
// inputs don't count as taken.
func (c *Calibration) restart() {
	c.controls = append([]Control(nil), c.original...)
	c.pos, c.zero, c.full, c.stage = 0, nil, nil, ""
	for _, k := range c.order {
		c.controls[k].Input = -1
		if c.controls[k].Kind != KindButton {
			c.stage = CalZero
		}
	}
	c.begin()
}

func (c *Calibration) current() int {
	if c.pos >= len(c.order) {
		return -1
	}
	return c.order[c.pos]
}

func (c *Calibration) begin() {
	c.input, c.count, c.held = -1, 0, -1
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

// Next takes the reading the board is at: 0%, then 100%. A 100% reading where nothing moved
// goes back to 0%.
func (c *Calibration) Next() {
	switch c.stage {
	case CalZero:
		c.zero, c.stage, c.warning = append([]int(nil), c.last...), CalFull, ""
	case CalFull:
		c.full = append([]int(nil), c.last...)
		for col := range c.full {
			if _, _, _, ok := c.ends(col); ok {
				c.stage = ""
				c.begin()
				return
			}
		}
		c.zero, c.full, c.stage, c.warning = nil, nil, CalZero, CalNothing
	}
}

// ends is an input's 0% and 100% readings as Min, Max and Reverse, when it moved between them.
func (c *Calibration) ends(col int) (lo, hi int, reverse, ok bool) {
	if col >= len(c.full) || c.full[col] < 0 {
		return 0, 0, false, false
	}
	at0, at100 := -1, c.full[col]
	if col < len(c.zero) {
		at0 = c.zero[col]
	}
	// A MIDI control already at 0% sends nothing until it moves, so its 0% is the far end.
	if at0 < 0 {
		at0 = 1023
		if at100 >= 512 {
			at0 = 0
		}
	}
	if abs(at100-at0) < calSwept {
		return 0, 0, false, false
	}
	return min(at0, at100), max(at0, at100), at100 < at0, true
}

// Skip leaves the current control as it was and goes on to the next.
func (c *Calibration) Skip() {
	if k := c.current(); k >= 0 && c.stage == "" {
		c.controls[k] = c.original[k]
		c.next()
	}
}

// Redo starts again from the 0% reading.
func (c *Calibration) Redo() { c.restart() }

// Feed takes a raw frame: a DIY board's line, or a MIDI board's MixerState values.
func (c *Calibration) Feed(raw []int) {
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
	if k < 0 || c.stage != "" {
		return
	}
	if c.controls[k].Kind == KindButton {
		if !c.midi {
			c.feedDIYButton(raw)
		}
		return
	}
	c.feedPot(raw)
}

func (c *Calibration) moved(raw []int, col int) int {
	if col >= len(raw) || raw[col] < 0 || c.base[col] < 0 {
		return 0
	}
	return raw[col] - c.base[col]
}

func (c *Calibration) feedPot(raw []int) {
	find, wrong := calFindDIY, calWrongDIY
	if c.midi {
		find, wrong = calFindMIDI, calFindMIDI
	}
	best, bestCol := 0, -1
	for col := range raw {
		d := abs(c.moved(raw, col))
		if d < min(find, wrong) {
			continue
		}
		if o := c.taken(col, false); o >= 0 {
			if d >= wrong {
				c.warning, c.other = CalWrong, o
			}
			continue
		}
		if d >= find && d > best {
			best, bestCol = d, col
		}
	}
	if bestCol < 0 {
		return
	}
	lo, hi, reverse, ok := c.ends(bestCol)
	if !ok {
		c.warning = CalUnswept
		return
	}
	ctl := &c.controls[c.current()]
	ctl.Input, ctl.Min, ctl.Max, ctl.Reverse = bestCol, lo, hi, reverse
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
	if k < 0 || c.stage != "" || c.controls[k].Kind != KindButton || !c.midi {
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
	switch {
	case c.stage != "":
		s.Stage = c.stage
	case k < 0:
	case c.controls[k].Kind == KindButton:
		s.Kind, s.Stage, s.Need, s.Count = KindButton, CalPress, 3, c.count
	default:
		s.Kind, s.Stage = c.controls[k].Kind, CalFind
	}
	return s
}

func (c *Calibration) Done() bool { return c.stage == "" && c.current() < 0 }

// Result is the board's controls with what was found; a skipped control keeps what it had.
func (c *Calibration) Result() []Control { return append([]Control(nil), c.controls...) }
