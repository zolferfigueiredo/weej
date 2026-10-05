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
// ScreenCount is how many screens the job list offers: the external screens Windows reports,
// and at least as many as the highest one a profile already uses, so a job for a screen that
// is unplugged right now can still be seen and unticked.
func ScreenCount(externals int, setup Setup) int {
	n := externals
	for _, p := range setup.Profiles {
		for _, row := range p.Jobs {
			for _, j := range row {
				if (j.Kind == JobBrightness || j.Kind == JobContrast) && j.Screen+1 > n {
					n = j.Screen + 1
				}
			}
		}
	}
	return n
}

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
