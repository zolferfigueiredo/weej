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
