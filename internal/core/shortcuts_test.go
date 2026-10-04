package core

import "testing"

func TestShortcutLabel(t *testing.T) {
	cases := []struct {
		s    Shortcut
		want string
	}{
		{Shortcut{VK: '1', Mods: ModControl | ModAlt, Key: "1"}, "Ctrl+Alt+1"},
		{Shortcut{VK: 0x70, Mods: ModControl, Key: "f1"}, "Ctrl+F1"},
		{Shortcut{VK: 0x26, Mods: ModWin, Key: "up"}, "Win+Up"},
		{Shortcut{VK: 0x0D, Mods: ModShift, Key: "enter"}, "Shift+Enter"},
	}
	for _, c := range cases {
		if got := Label(c.s, "Ctrl"); got != c.want {
			t.Errorf("Label(%+v) = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestShortcutLabelLocalizedCtrl(t *testing.T) {
	s := Shortcut{VK: '1', Mods: ModControl, Key: "1"}
	if got := Label(s, "Strg"); got != "Strg+1" {
		t.Errorf("Label() = %q, want Strg+1", got)
	}
}

func TestShortcutValid(t *testing.T) {
	if !Valid(Shortcut{Key: "1", Mods: ModControl}) {
		t.Error("want valid: has Ctrl and a key")
	}
	if Valid(Shortcut{Key: "", Mods: ModControl}) {
		t.Error("want invalid: no key")
	}
	if Valid(Shortcut{Key: "1", Mods: ModShift}) {
		t.Error("want invalid: Shift alone is not enough")
	}
	if !Valid(Shortcut{Key: "1", Mods: ModAlt}) {
		t.Error("want valid: Alt is enough")
	}
	if Valid(Shortcut{VK: 0x11, Key: "Control", Mods: ModControl}) {
		t.Error("want invalid: a modifier on its own is not a shortcut")
	}
	if Valid(Shortcut{VK: 0xA4, Key: "Alt", Mods: ModControl | ModAlt}) {
		t.Error("want invalid: a modifier on its own is not a shortcut")
	}
}

func TestShortcutLabelNamesLettersByKeyNotCharacter(t *testing.T) {
	s := Shortcut{VK: 'E', Mods: ModControl | ModAlt, Key: "€"}
	if got := Label(s, "Ctrl"); got != "Ctrl+Alt+E" {
		t.Errorf("Label() = %q, want Ctrl+Alt+E", got)
	}
}

func TestShortcutClash(t *testing.T) {
	a := &Shortcut{VK: '1', Mods: ModControl}
	b := &Shortcut{VK: '1', Mods: ModControl}
	c := &Shortcut{VK: '2', Mods: ModControl}
	if !Clash(a, b) {
		t.Error("want a clash: same key and mods")
	}
	if Clash(a, c) {
		t.Error("want no clash: different key")
	}
	if Clash(a, nil) || Clash(nil, nil) {
		t.Error("want no clash when either side has no shortcut")
	}
}
