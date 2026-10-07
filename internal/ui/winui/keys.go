//go:build windows

package winui

import "unsafe"

var procSendInput = user32.NewProc("SendInput")

const (
	VKMediaNextTrack = 0xB0
	VKMediaPrevTrack = 0xB1
	VKMediaStop      = 0xB2
	VKMediaPlayPause = 0xB3
	VKVolumeMute     = 0xAD
	VKVolumeDown     = 0xAE
	VKVolumeUp       = 0xAF

	inputKeyboard        = 1
	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
)

type keybdInput struct {
	vk        uint16
	scan      uint16
	flags     uint32
	time      uint32
	extraInfo uintptr
}

// Mirrors INPUT with its keyboard member. The tail pads the union to MOUSEINPUT's size, so the
// struct is 40 bytes on 64-bit and 28 on 32-bit, which SendInput checks against cbSize.
type keyboardInput struct {
	typ uint32
	ki  keybdInput
	_   [8]byte
}

func MediaKey(vk uint16) {
	inputs := [2]keyboardInput{
		{typ: inputKeyboard, ki: keybdInput{vk: vk, flags: keyeventfExtendedKey}},
		{typ: inputKeyboard, ki: keybdInput{vk: vk, flags: keyeventfExtendedKey | keyeventfKeyUp}},
	}
	procSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
}

// SendShortcut presses a key with modifiers held, as typing it would. mods uses RegisterHotKey's
// MOD_ALT, MOD_CONTROL, MOD_SHIFT and MOD_WIN bits, as core.Shortcut does.
func SendShortcut(mods, vk uint16) {
	var held []uint16
	for _, m := range []struct{ bit, vk uint16 }{{2, 0x11}, {1, 0x12}, {4, 0x10}, {8, 0x5B}} {
		if mods&m.bit != 0 {
			held = append(held, m.vk)
		}
	}
	var inputs []keyboardInput
	for _, k := range held {
		inputs = append(inputs, keyInput(k, false))
	}
	inputs = append(inputs, keyInput(vk, false), keyInput(vk, true))
	for i := len(held) - 1; i >= 0; i-- {
		inputs = append(inputs, keyInput(held[i], true))
	}
	procSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
}

func keyInput(vk uint16, up bool) keyboardInput {
	var flags uint32
	// Arrows, Insert, Delete, Home, End, Page Up/Down, Win, numpad divide, Num Lock and the
	// browser and media keys arrive with the E0 prefix, so they need the extended flag.
	switch {
	case vk >= 0x21 && vk <= 0x28, vk == 0x2D, vk == 0x2E, vk == 0x5B, vk == 0x5C, vk == 0x6F, vk == 0x90,
		vk >= 0xA6 && vk <= 0xB7:
		flags |= keyeventfExtendedKey
	}
	if up {
		flags |= keyeventfKeyUp
	}
	return keyboardInput{typ: inputKeyboard, ki: keybdInput{vk: vk, flags: flags}}
}
