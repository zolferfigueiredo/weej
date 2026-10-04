//go:build windows

package winui

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winmm = windows.NewLazySystemDLL("winmm.dll")

	procMessageBeep = user32.NewProc("MessageBeep")
	procPlaySoundW  = winmm.NewProc("PlaySoundW")
)

const (
	mbOK = 0x00000000

	sndAlias = 0x00010000
	sndAsync = 0x00000001
)

func Beep() {
	procMessageBeep.Call(mbOK)
}

func StepSound() {
	name := mustUTF16PtrFromString("SystemAsterisk")
	procPlaySoundW.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(sndAlias|sndAsync))
}
