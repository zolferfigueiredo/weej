package core

import (
	"math"
	"slices"
)

// LightPatterns are what an SMC-Mixer's button lights can show, by the name settings save.
var LightPatterns = []string{"off", "on", "chase", "bounce", "wave", "sparkle", "blink"}

// ParseLightPattern keeps a known pattern, and turns anything else, "off" included, into "".
func ParseLightPattern(s string) string {
	if s == "off" || !slices.Contains(LightPatterns, s) {
		return ""
	}
	return s
}

// The strip buttons as a grid, by the first note of each row: eight columns, and M, S, R and
// Square from top to bottom.
var lightRows = [4]int{16, 8, 0, 24}

// SMCStripButtons is the grid, row by row. The bottom row is left out of every pattern: lighting it
// sets a real mixer sending fader moves nobody made.
func SMCStripButtons() []int {
	var ids []int
	for _, first := range lightRows {
		for col := 0; col < 8; col++ {
			ids = append(ids, MixerNoteButton(first+col))
		}
	}
	return ids
}

// LightFrame lists the buttons a pattern lights t seconds after it started.
func LightFrame(pattern string, t float64) []int {
	var on []int
	cell := func(row, col int) { on = append(on, MixerNoteButton(lightRows[row]+col)) }
	column := func(col int) {
		for row := range lightRows {
			cell(row, col)
		}
	}
	switch pattern {
	case "on":
		on = SMCStripButtons()
	case "blink":
		if int(t/0.5)%2 == 0 {
			on = SMCStripButtons()
		}
	case "chase":
		column(int(t/0.12) % 8)
	case "bounce":
		step := int(t/0.09) % 14
		if step > 7 {
			step = 14 - step
		}
		column(step)
	case "wave":
		for col := 0; col < 8; col++ {
			height := int(math.Round(2 + 2*math.Sin(2*math.Pi*(t/1.6-float64(col)/8))))
			for row := len(lightRows) - height; row < len(lightRows); row++ {
				cell(row, col)
			}
		}
	case "sparkle":
		step := uint32(t / 0.15)
		for _, id := range SMCStripButtons() {
			if sparkle(step, id)%4 == 0 {
				on = append(on, id)
			}
		}
	}
	return on
}

// NextLightPattern is the pattern after cur, from the last back to "".
func NextLightPattern(cur string) string {
	i := slices.Index(LightPatterns, cur)
	if cur == "" {
		i = 0
	}
	return ParseLightPattern(LightPatterns[(i+1)%len(LightPatterns)])
}

// Animated is whether a pattern changes over time.
func Animated(pattern string) bool { return pattern != "" && pattern != "off" && pattern != "on" }

func sparkle(step uint32, id int) uint32 {
	h := step*2654435761 ^ uint32(id)*2246822519
	h ^= h >> 15
	h *= 2654435761
	return h >> 13
}
