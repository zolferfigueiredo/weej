package core

import (
	"strconv"
	"strings"
)

func namedKey(vk uint16) (string, bool) {
	switch {
	// With Ctrl+Alt held (AltGr on many layouts) the typed character is often a symbol such as
	// "€", so letters and digits are named from the key itself.
	case vk >= '0' && vk <= '9', vk >= 'A' && vk <= 'Z':
		return string(rune(vk)), true
	case vk >= 0x70 && vk <= 0x87:
		return "F" + strconv.Itoa(int(vk-0x70+1)), true
	case vk == 0x26:
		return "Up", true
	case vk == 0x28:
		return "Down", true
	case vk == 0x25:
		return "Left", true
	case vk == 0x27:
		return "Right", true
	case vk == 0x20:
		return "Space", true
	case vk == 0x0D:
		return "Enter", true
	case vk == 0x09:
		return "Tab", true
	case vk == 0x08:
		return "Backspace", true
	case vk == 0x2E:
		return "Delete", true
	case vk == 0x24:
		return "Home", true
	case vk == 0x23:
		return "End", true
	case vk == 0x21:
		return "PgUp", true
	case vk == 0x22:
		return "PgDn", true
	case vk == 0x2D:
		return "Ins", true
	default:
		return "", false
	}
}

// ctrlName lets the caller localize Ctrl (German's Strg); the other modifier labels are kept
// short and the same across languages.
func Label(s Shortcut, ctrlName string) string {
	var parts []string
	if s.Mods&ModControl != 0 {
		parts = append(parts, ctrlName)
	}
	if s.Mods&ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if s.Mods&ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if s.Mods&ModWin != 0 {
		parts = append(parts, "Win")
	}
	name, ok := namedKey(s.VK)
	if !ok {
		name = strings.ToUpper(s.Key)
	}
	parts = append(parts, name)
	return strings.Join(parts, "+")
}

func isModifierVK(vk uint16) bool {
	switch vk {
	case 0x10, 0x11, 0x12, 0x5B, 0x5C, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5:
		return true
	}
	return false
}

func Valid(s Shortcut) bool {
	return s.Key != "" && s.Mods&(ModControl|ModAlt) != 0 && !isModifierVK(s.VK)
}

func Clash(a, b *Shortcut) bool {
	if a == nil || b == nil {
		return false
	}
	return a.VK == b.VK && a.Mods == b.Mods
}
