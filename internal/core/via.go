package core

import "math"

// Both VIA dialects in one packet each, so a keyboard that speaks only one of the two just
// ignores the other. The Windows layer prefixes report id 0x00 itself.
func ViaReports(s float64) [][]byte {
	clamped := math.Max(0, math.Min(1, s))
	v := byte(math.Round(clamped * 255))
	reports := [][]byte{{0x07, 0x80, v}, {0x07, 0x03, 0x01, v}}
	out := make([][]byte, len(reports))
	for i, r := range reports {
		buf := make([]byte, 32)
		copy(buf, r)
		out[i] = buf
	}
	return out
}
