//go:build windows

package winui

var procMessageBeep = user32.NewProc("MessageBeep")

const mbOK = 0x00000000

func Beep() {
	procMessageBeep.Call(mbOK)
}
