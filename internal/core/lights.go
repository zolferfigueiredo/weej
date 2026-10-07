package core

import (
	"math"
	"slices"
	"time"
)

// LightPatterns are what an SMC-Mixer's button lights can show, by the name settings save. "eq"
// follows the computer's sound (EQFrame) and "clock" the time (ClockFrame); LightFrame draws the
// rest.
var LightPatterns = []string{
	"off", "on", "random", "eq", "fire", "chase", "bounce", "wave", "sparkle", "blink", "rain", "matrix",
	"snake", "fill", "explode", "checker", "rise", "zigzag", "orbit", "heartbeat", "stars", "bars",
	"ball", "comet", "helix", "breathe", "clock",
}

// ParseLightPattern keeps a known pattern, and turns anything else, "off" included, into "".
func ParseLightPattern(s string) string {
	if s == "off" || !slices.Contains(LightPatterns, s) {
		return ""
	}
	return s
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

// The strip buttons as a grid, by the first note of each row: eight columns, and M, S, R and
// Square from top to bottom.
var lightRows = [4]int{16, 8, 0, 24}

const (
	gridCols = 8
	gridRows = len(lightRows)
)

// SMCStripButtons is the grid, row by row. The bottom row is left out of every pattern: lighting it
// sets a real mixer sending fader moves nobody made.
func SMCStripButtons() []int {
	var ids []int
	for _, first := range lightRows {
		for col := range gridCols {
			ids = append(ids, MixerNoteButton(first+col))
		}
	}
	return ids
}

type grid [gridRows][gridCols]bool

func (g *grid) set(row, col int) {
	if row >= 0 && row < gridRows && col >= 0 && col < gridCols {
		g[row][col] = true
	}
}

func (g *grid) column(col int) {
	for row := range gridRows {
		g.set(row, col)
	}
}

// fillUp lights a column's bottom height rows.
func (g *grid) fillUp(col, height int) {
	for row := gridRows - height; row < gridRows; row++ {
		g.set(row, col)
	}
}

func (g *grid) all() {
	for col := range gridCols {
		g.column(col)
	}
}

func (g *grid) ids() []int {
	var on []int
	for row := range gridRows {
		for col := range gridCols {
			if g[row][col] {
				on = append(on, MixerNoteButton(lightRows[row]+col))
			}
		}
	}
	return on
}

// randomEvery is how long "random" keeps each pattern, in seconds; it picks among LightFrame's
// own, leaving out On, the EQ and the clock.
const randomEvery = 6

// RandomPattern is the pattern "random" shows t seconds after it started. It goes through all of
// them in a shuffled order, then another, never showing the same one twice in a row.
func RandomPattern(t float64) string {
	var pool []string
	for _, p := range LightPatterns {
		if Animated(p) && p != "random" && p != "eq" && p != "clock" {
			pool = append(pool, p)
		}
	}
	round := func(r int) []string {
		keys := make([]int, len(pool))
		for i := range keys {
			keys[i] = noise(r, i)<<8 | i
		}
		slices.Sort(keys)
		order := make([]string, len(pool))
		for i, k := range keys {
			order[i] = pool[k&0xFF]
		}
		return order
	}
	n := int(t / randomEvery)
	r, pos := n/len(pool), n%len(pool)
	order := round(r)
	if r > 0 && order[0] == round(r - 1)[len(pool)-1] {
		order[0], order[1] = order[1], order[0]
	}
	return order[pos]
}

// LightFrame lists the buttons a pattern lights t seconds after it started.
func LightFrame(pattern string, t float64) []int {
	if pattern == "random" {
		pattern = RandomPattern(t)
	}
	var g grid
	step := func(every float64) int { return int(t / every) }
	switch pattern {
	case "on":
		g.all()
	case "blink":
		if step(0.5)%2 == 0 {
			g.all()
		}
	case "chase":
		g.column(step(0.12) % gridCols)
	case "bounce":
		g.column(triangle(step(0.09), gridCols-1))
	case "wave":
		for col := range gridCols {
			g.fillUp(col, int(math.Round(2+2*math.Sin(2*math.Pi*(t/1.6-float64(col)/8)))))
		}
	case "sparkle":
		s := step(0.15)
		for _, cell := range cells() {
			if noise(s, cell)%4 == 0 {
				g.set(cell/gridCols, cell%gridCols)
			}
		}
	case "fire":
		s := step(0.08)
		for col := range gridCols {
			height := 1 + noise(s, col)%3
			if noise(s, col+50)%6 == 0 {
				height = gridRows
			}
			g.fillUp(col, height)
		}
	case "rain", "matrix":
		every, tail := 0.11, 1
		if pattern == "matrix" {
			every, tail = 0.14, 2
		}
		s := step(every)
		for col := range gridCols {
			cycle := gridRows + tail + noise(col, 7)%4
			head := (s + noise(col, 3)) % cycle
			for k := range tail {
				g.set(head-k, col)
			}
		}
	case "snake":
		path := serpentine()
		head := step(0.07) % len(path)
		for k := range 5 {
			cell := path[(head-k+len(path))%len(path)]
			g.set(cell/gridCols, cell%gridCols)
		}
	case "fill":
		s := step(0.15) % (2 * gridCols)
		for col := range gridCols {
			if s < gridCols && col <= s || s >= gridCols && col > s-gridCols {
				g.column(col)
			}
		}
	case "explode":
		r := step(0.12) % 5
		for col := range gridCols {
			if int(math.Ceil(math.Abs(float64(col)-3.5))) == r {
				g.column(col)
			}
		}
	case "checker":
		s := step(0.4)
		for _, cell := range cells() {
			if (cell/gridCols+cell%gridCols+s)%2 == 0 {
				g.set(cell/gridCols, cell%gridCols)
			}
		}
	case "rise":
		s := step(0.15) % (2 * gridRows)
		for col := range gridCols {
			if s < gridRows {
				g.fillUp(col, s+1)
			} else {
				for row := range 2*gridRows - 1 - s {
					g.set(row, col)
				}
			}
		}
	case "zigzag":
		s := step(0.1)
		for col := range gridCols {
			g.set(triangle(col+s, gridRows-1), col)
		}
	case "orbit":
		path := border()
		head := step(0.06) % len(path)
		for k := range 4 {
			cell := path[(head-k+len(path))%len(path)]
			g.set(cell/gridCols, cell%gridCols)
		}
	case "heartbeat":
		if p := math.Mod(t, 1.2); p < 0.1 || p >= 0.22 && p < 0.32 {
			for col := 2; col < 6; col++ {
				g.column(col)
			}
		}
	case "stars":
		s := step(0.4)
		for _, cell := range cells() {
			if noise(s, cell)%10 == 0 {
				g.set(cell/gridCols, cell%gridCols)
			}
		}
	case "bars":
		s := step(0.15)
		for col := range gridCols {
			g.fillUp(col, noise(s, col)%(gridRows+1))
		}
	case "ball":
		g.set(triangle(step(0.13), gridRows-1), triangle(step(0.1), gridCols-1))
	case "comet":
		head := step(0.09) % (gridCols + 2)
		g.column(head)
		g.set(1, head-1)
		g.set(2, head-1)
		g.set(2, head-2)
	case "helix":
		s := step(0.12)
		for col := range gridCols {
			row := triangle(col+s, gridRows-1)
			g.set(row, col)
			g.set(gridRows-1-row, col)
		}
	case "breathe":
		switch []int{0, 1, 2, 2, 1, 0}[step(0.25)%6] {
		case 1:
			for col := range gridCols {
				g.set(1, col)
				g.set(2, col)
			}
		case 2:
			g.all()
		}
	}
	return g.ids()
}

// EQFrame lights each column as high as its band of the sound, each band 0 to 1, lowest first.
func EQFrame(bands [gridCols]float64) []int {
	var g grid
	for col, b := range bands {
		g.fillUp(col, int(math.Ceil(min(max(b, 0), 1)*float64(gridRows)-0.25)))
	}
	return g.ids()
}

// ClockFrame shows the time as a binary clock: hours, minutes and seconds, two columns each, one
// digit per column, from 1 on the bottom row to 8 on the top.
func ClockFrame(now time.Time) []int {
	var g grid
	for i, n := range []int{now.Hour(), now.Minute(), now.Second()} {
		for j, digit := range []int{n / 10, n % 10} {
			col := 1 + 2*i + j
			for bit := range gridRows {
				if digit>>bit&1 == 1 {
					g.set(gridRows-1-bit, col)
				}
			}
		}
	}
	return g.ids()
}

// triangle runs 0, 1 ... top, top-1 ... 1, 0, 1 ... as n grows.
func triangle(n, top int) int {
	n %= 2 * top
	if n > top {
		return 2*top - n
	}
	return n
}

// cells numbers the grid row by row: row*gridCols + col.
func cells() []int {
	out := make([]int, gridRows*gridCols)
	for i := range out {
		out[i] = i
	}
	return out
}

// serpentine walks the grid row by row, turning at each end.
func serpentine() []int {
	var path []int
	for row := range gridRows {
		for k := range gridCols {
			col := k
			if row%2 == 1 {
				col = gridCols - 1 - k
			}
			path = append(path, row*gridCols+col)
		}
	}
	return path
}

// border walks the grid's edge clockwise from the top left.
func border() []int {
	var path []int
	for col := range gridCols {
		path = append(path, col)
	}
	for row := 1; row < gridRows; row++ {
		path = append(path, row*gridCols+gridCols-1)
	}
	for col := gridCols - 2; col >= 0; col-- {
		path = append(path, (gridRows-1)*gridCols+col)
	}
	for row := gridRows - 2; row > 0; row-- {
		path = append(path, row*gridCols)
	}
	return path
}

func noise(a, b int) int {
	h := uint32(a)*2654435761 ^ uint32(b)*2246822519
	h ^= h >> 15
	h *= 2654435761
	return int(h >> 13)
}

// LightGuard drops the small fader and pot moves an SMC-Mixer reports while its button lights
// change: lighting many at once lifts its readings by up to 4 steps of 127 within milliseconds,
// and they creep back over the next half second. A bigger move is a hand, followed closely until
// it rests.
type LightGuard struct {
	reported [faderFirstCC + 8]int
	known    [faderFirstCC + 8]bool
	awake    [faderFirstCC + 8]float64
}

const (
	guardWindow    = 0.6  // seconds after a light change
	guardSteps     = 5    // of 127
	guardJump      = 0.05 // seconds after a light change, when the jumps come
	guardJumpSteps = 10   // of 127, then
	guardAwake     = 0.3  // seconds a hand's move keeps being followed
)

// Pass tells whether a message should be read, at now, the last light change having been at
// lightChanged (both in seconds).
func (g *LightGuard) Pass(msg uint32, now, lightChanged float64) bool {
	col, v, ok := potReading(msg)
	if !ok {
		return true
	}
	steps := guardSteps
	if now-lightChanged <= guardJump {
		steps = guardJumpSteps
	}
	hand := now < g.awake[col] || g.known[col] && abs(v-g.reported[col]) > steps
	if g.known[col] && now-lightChanged <= guardWindow && !hand {
		return false
	}
	if hand {
		g.awake[col] = now + guardAwake
	}
	g.known[col], g.reported[col] = true, v
	return true
}

// potReading is the column and 7-bit value of a fader or absolute knob: a fader's pitch bend in
// DAW mode, or the CC of a fader or knob in CC mode.
func potReading(msg uint32) (col, value int, ok bool) {
	status := int(msg & 0xFF)
	data1 := int(msg>>8) & 0x7F
	data2 := int(msg>>16) & 0x7F
	switch {
	case status >= 0xE0 && status < 0xE8:
		return faderFirstCC + status - 0xE0, data2, true
	case status&0xF0 == 0xB0 && (data1 >= knobFirstCC && data1 < knobFirstCC+8 || data1 >= faderFirstCC && data1 < faderFirstCC+8):
		return data1, data2, true
	}
	return 0, 0, false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
