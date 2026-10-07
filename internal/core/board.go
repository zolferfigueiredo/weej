package core

import "sort"

// ControlKind is what one of the board's knobs is on the board itself.
type ControlKind string

const (
	KindKnob   ControlKind = "knob"
	KindFader  ControlKind = "fader"
	KindButton ControlKind = "button"
)

func ParseControlKind(s string) ControlKind {
	switch ControlKind(s) {
	case KindFader, KindButton:
		return ControlKind(s)
	}
	return KindKnob
}

// CleanLayout keeps a saved layout drawable for n knobs: indices out of range or seen before go,
// as do rows left empty, and knobs it misses join the last row.
func CleanLayout(layout [][]int, n int) [][]int {
	if layout == nil {
		return nil
	}
	seen := map[int]bool{}
	out := [][]int{}
	for _, row := range layout {
		var kept []int
		for _, i := range row {
			if i >= 0 && i < n && !seen[i] {
				seen[i] = true
				kept = append(kept, i)
			}
		}
		if len(kept) > 0 {
			out = append(out, kept)
		}
	}
	for i := 0; i < n; i++ {
		if seen[i] {
			continue
		}
		if len(out) == 0 {
			out = append(out, []int{})
		}
		out[len(out)-1] = append(out[len(out)-1], i)
	}
	return out
}

// ButtonWatcher tells when one of the board's buttons is pressed. A button reads one end of the
// range at rest and the other while held, whichever way it is wired, so where its input first
// reads counts as rest.
type ButtonWatcher struct {
	rest map[int]int
	down map[int]bool
	last map[int]float64
}

const (
	buttonPressAt   = 400
	buttonReleaseAt = 200
	// Contacts bounce for a few milliseconds, which would read as a second press.
	buttonRepeat = 0.15
)

// Pressed takes a frame and the buttons' inputs by knob index, and lists the knobs just pressed.
func (w *ButtonWatcher) Pressed(values []int, inputs map[int]int, now float64) []int {
	if w.rest == nil {
		w.rest, w.down, w.last = map[int]int{}, map[int]bool{}, map[int]float64{}
	}
	knobs := make([]int, 0, len(inputs))
	for k := range inputs {
		knobs = append(knobs, k)
	}
	sort.Ints(knobs)
	var pressed []int
	for _, k := range knobs {
		col := inputs[k]
		if col < 0 || col >= len(values) || values[col] < 0 {
			continue
		}
		v := values[col]
		rest, ok := w.rest[col]
		if !ok {
			w.rest[col] = v
			continue
		}
		away := abs(v - rest)
		switch {
		case !w.down[col] && away >= buttonPressAt:
			w.down[col] = true
			if last, ok := w.last[col]; !ok || now-last >= buttonRepeat {
				pressed = append(pressed, k)
			}
			w.last[col] = now
		case w.down[col] && away < buttonReleaseAt:
			w.down[col] = false
		}
	}
	return pressed
}

// Reset forgets where the buttons rest, for a board that just connected.
func (w *ButtonWatcher) Reset() { w.rest, w.down, w.last = nil, nil, nil }
