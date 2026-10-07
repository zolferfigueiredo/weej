package core

import "testing"

func TestExternalsLeftToRightWithoutTheBuiltIn(t *testing.T) {
	displays := []Display{
		{ID: "R", Left: 2560, Internal: false},
		{ID: "BUILTIN", Left: -1470, Internal: true},
		{ID: "L", Left: 0, Internal: false},
	}
	got := Externals(displays)
	if len(got) != 2 || got[0].ID != "L" || got[1].ID != "R" {
		t.Errorf("Externals() = %v, want [L R]", got)
	}

	if got := Externals([]Display{{ID: "BUILTIN", Left: 0, Internal: true}}); len(got) != 0 {
		t.Errorf("Externals() = %v, want empty", got)
	}
}

func TestScreenCountIsTheScreensThere(t *testing.T) {
	empty := []Device{NewDevice("d1", "Desk", DeviceDIY, 1, 0, 0, "Default")}
	for _, externals := range []int{0, 1, 2, 4} {
		if got := ScreenCount(externals, empty); got != externals {
			t.Errorf("ScreenCount(%d, no jobs) = %d, want %d", externals, got, externals)
		}
	}

	// A job for screen 3 keeps it listed while only one screen is plugged in.
	withScreen3 := []Device{{Profiles: []DeviceProfile{{Name: "Default", Jobs: [][]Job{{{Kind: JobContrast, Screen: 2}}}}}}}
	if got := ScreenCount(1, withScreen3); got != 3 {
		t.Errorf("ScreenCount(1, screen 3 in use) = %d, want 3", got)
	}
}
