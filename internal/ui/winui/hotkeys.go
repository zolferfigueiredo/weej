//go:build windows

package winui

import "sync"

var (
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
)

const modNoRepeat = 0x4000

type Hotkeys struct {
	loop *Loop

	mu  sync.Mutex
	fns []func(id int)
	ids []int
}

func (h *Hotkeys) Register(id int, mods, vk uint16) bool {
	r, _, _ := procRegisterHotKey.Call(uintptr(h.loop.hwnd), uintptr(id), uintptr(mods)|modNoRepeat, uintptr(vk))
	if r != 0 {
		h.mu.Lock()
		h.ids = append(h.ids, id)
		h.mu.Unlock()
	}
	return r != 0
}

func (h *Hotkeys) UnregisterAll() {
	h.mu.Lock()
	ids := h.ids
	h.ids = nil
	h.mu.Unlock()
	for _, id := range ids {
		procUnregisterHotKey.Call(uintptr(h.loop.hwnd), uintptr(id))
	}
}

func (h *Hotkeys) OnHotkey(fn func(id int)) {
	h.mu.Lock()
	h.fns = append(h.fns, fn)
	h.mu.Unlock()
}
