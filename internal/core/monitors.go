package core

import "sort"

type Display struct {
	ID       string
	Left     int
	Top      int
	Right    int
	Bottom   int
	Internal bool
	Primary  bool
}

// The internal display is dropped, not sorted to the front: on a machine where it sits at a
// negative coordinate, leaving it in would silently make it "screen 1".
func Externals(displays []Display) []Display {
	var out []Display
	for _, d := range displays {
		if !d.Internal {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Left != out[j].Left {
			return out[i].Left < out[j].Left
		}
		return out[i].Top < out[j].Top
	})
	return out
}
