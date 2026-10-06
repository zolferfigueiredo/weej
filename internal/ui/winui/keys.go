//go:build windows

package winui

import "unsafe"

var procSendInput = user32.NewProc("SendInput")

const (
	VKMediaNextTrack = 0xB0
	VKMediaPrevTrack = 0xB1
	VKMediaStop      = 0xB2
	VKMediaPlayPause = 0xB3

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
