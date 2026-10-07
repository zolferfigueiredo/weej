package core

// HotkeyTarget is what a shortcut does on one board: step its profile by Step, or with Step 0
// switch it to Profile.
type HotkeyTarget struct {
	Device  string
	Step    int
	Profile int
}

// HotkeyBinding is one key combination and everything it does, on every board using it.
type HotkeyBinding struct {
	Shortcut Shortcut
	Targets  []HotkeyTarget
}

// HotkeyBindings groups the enabled boards' shortcuts by key: a combination can only be
// registered once, and the same one may switch several boards.
func HotkeyBindings(devices []Device) []HotkeyBinding {
	var out []HotkeyBinding
	index := map[[2]uint16]int{}
	add := func(s *Shortcut, t HotkeyTarget) {
		if s == nil || s.VK == 0 {
			return
		}
		key := [2]uint16{s.Mods, s.VK}
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, HotkeyBinding{Shortcut: *s})
		}
		out[i].Targets = append(out[i].Targets, t)
	}
	for _, d := range devices {
		if !d.Enabled {
			continue
		}
		add(d.Next, HotkeyTarget{Device: d.ID, Step: 1})
		add(d.Previous, HotkeyTarget{Device: d.ID, Step: -1})
		for i, p := range d.Profiles {
			add(p.Shortcut, HotkeyTarget{Device: d.ID, Profile: i})
		}
	}
	return out
}
