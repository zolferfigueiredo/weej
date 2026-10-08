//go:build windows

package winui

import (
	"testing"
	"unsafe"
)

func TestKeyboardInputMatchesINPUT(t *testing.T) {
	want := uintptr(40)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 28
	}
	if got := unsafe.Sizeof(keyboardInput{}); got != want {
		t.Errorf("sizeof(keyboardInput) = %d, want %d like INPUT", got, want)
	}
}
