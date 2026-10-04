//go:build windows

package winui

import (
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

type timerEntry struct {
	fn     func()
	repeat bool
}

type Loop struct {
	hwnd     windows.HWND
	exitCode int

	invokeCh chan func()

	mu            sync.Mutex
	timers        map[uintptr]*timerEntry
	nextTimerID   uintptr
	rawHandlers   map[uint32]func(wparam, lparam uintptr) uintptr
	themeFns      []func()
	displayFns    []func()
	endSessionFns []func()

	taskbarCreatedMsg uint32
	trayListeners     []*Tray
	nextTrayID        uint32

	hotkeys *Hotkeys
}

// Run does everything the UI thread owns: it never returns until something calls l.Quit, at
// which point it returns the given exit code.
func Run(setup func(l *Loop)) int {
	runtime.LockOSThread()

	comOwned := initCOM()
	if comOwned {
		defer windows.CoUninitialize()
	}

	l := &Loop{
		invokeCh:    make(chan func(), 64),
		timers:      make(map[uintptr]*timerEntry),
		rawHandlers: make(map[uint32]func(wparam, lparam uintptr) uintptr),
	}
	l.hotkeys = &Hotkeys{loop: l}

	className := mustUTF16PtrFromString(sys.MainWindowClass)
	wc := wndClassExW{
		size:      uint32(unsafe.Sizeof(wndClassExW{})),
		wndProc:   windows.NewCallback(l.wndProc),
		instance:  getModuleHandle(),
		cursor:    loadCursorArrow(),
		className: className,
	}
	if _, err := registerClassExW(&wc); err != nil {
		return 1
	}

	hwnd, err := createWindowExW(wsExToolWindow, className, className, 0, 0, 0, 0, 0, 0, 0, getModuleHandle())
	if err != nil {
		return 1
	}
	l.hwnd = hwnd

	setup(l)

	var msg msgT
	for {
		r := getMessageW(&msg)
		if r == 0 || r == -1 {
			break
		}
		translateMessage(&msg)
		dispatchMessageW(&msg)
	}
	return l.exitCode
}

// initCOM reports whether this call is the one that should balance with CoUninitialize: S_FALSE
// means COM was already initialized compatibly on this thread, which still counts as ours to
// release; any other error (e.g. a prior incompatible CoInitializeEx) means it is not.
func initCOM() bool {
	err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	if err == nil {
		return true
	}
	errno, ok := err.(syscall.Errno)
	return ok && errno == 1 // S_FALSE
}

func (l *Loop) wndProc(hwnd windows.HWND, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case msgInvoke:
		l.drainInvoke()
		return 0
	case wmTimer:
		l.fireTimer(wparam)
		return 0
	case wmHotkey:
		l.fireHotkey(int(wparam))
		return 0
	case wmSettingChange:
		if readForeignUTF16(lparam) == "ImmersiveColorSet" {
			l.fireList(&l.themeFns)
		}
		return defWindowProcW(hwnd, msg, wparam, lparam)
	case wmDisplayChange:
		l.fireList(&l.displayFns)
		return defWindowProcW(hwnd, msg, wparam, lparam)
	case wmQueryEndSession:
		return 1 // allow the session to end
	case wmEndSession:
		if wparam != 0 {
			l.fireList(&l.endSessionFns)
			l.exitCode = 0
			postQuitMessageW(0)
		}
		return 0
	case wmDestroy:
		return 0
	}

	l.mu.Lock()
	taskbarMsg := l.taskbarCreatedMsg
	l.mu.Unlock()
	if taskbarMsg != 0 && msg == taskbarMsg {
		l.mu.Lock()
		listeners := append([]*Tray(nil), l.trayListeners...)
		l.mu.Unlock()
		for _, t := range listeners {
			t.readd()
		}
		return 0
	}

	l.mu.Lock()
	h, ok := l.rawHandlers[msg]
	l.mu.Unlock()
	if ok {
		return h(wparam, lparam)
	}
	return defWindowProcW(hwnd, msg, wparam, lparam)
}

func (l *Loop) drainInvoke() {
	for {
		select {
		case fn := <-l.invokeCh:
			fn()
		default:
			return
		}
	}
}

func (l *Loop) fireTimer(id uintptr) {
	l.mu.Lock()
	entry, ok := l.timers[id]
	if ok && !entry.repeat {
		delete(l.timers, id)
	}
	l.mu.Unlock()
	if !ok {
		return
	}
	if !entry.repeat {
		killTimer(l.hwnd, id)
	}
	entry.fn()
}

func (l *Loop) fireHotkey(id int) {
	l.hotkeys.mu.Lock()
	var fns []func(int)
	fns = append(fns, l.hotkeys.fns...)
	l.hotkeys.mu.Unlock()
	for _, fn := range fns {
		fn(id)
	}
}

func (l *Loop) fireList(list *[]func()) {
	l.mu.Lock()
	var fns []func()
	fns = append(fns, *list...)
	l.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// on registers a handler that sees the raw wParam/lParam, for messages carrying data (the tray
// callback) rather than just acting as a signal (OnMessage).
func (l *Loop) on(msg uint32, h func(wparam, lparam uintptr) uintptr) {
	l.mu.Lock()
	l.rawHandlers[msg] = h
	l.mu.Unlock()
}

func (l *Loop) Invoke(fn func()) {
	l.invokeCh <- fn
	postMessageW(l.hwnd, msgInvoke, 0, 0)
}

func (l *Loop) Quit(code int) {
	l.Invoke(func() {
		l.exitCode = code
		postQuitMessageW(int32(code))
	})
}

// After and Every marshal the SetTimer/KillTimer calls onto the UI thread via Invoke, so they
// are safe to call from any goroutine even though timers themselves are UI-thread-only.
func (l *Loop) After(d time.Duration, fn func()) (stop func()) {
	return l.addTimer(d, false, fn)
}

func (l *Loop) Every(d time.Duration, fn func()) (stop func()) {
	return l.addTimer(d, true, fn)
}

func (l *Loop) addTimer(d time.Duration, repeat bool, fn func()) func() {
	l.mu.Lock()
	l.nextTimerID++
	id := l.nextTimerID
	l.mu.Unlock()

	l.Invoke(func() {
		l.mu.Lock()
		l.timers[id] = &timerEntry{fn: fn, repeat: repeat}
		l.mu.Unlock()
		setTimer(l.hwnd, id, millis(d))
	})

	var stopped int32
	return func() {
		if !atomic.CompareAndSwapInt32(&stopped, 0, 1) {
			return
		}
		l.Invoke(func() {
			l.mu.Lock()
			delete(l.timers, id)
			l.mu.Unlock()
			killTimer(l.hwnd, id)
		})
	}
}

func millis(d time.Duration) uint32 {
	if d <= 0 {
		return 1
	}
	return uint32(d.Milliseconds())
}

func (l *Loop) OnMessage(name string, fn func()) {
	id := registerWindowMessageW("WeeJ." + name)
	l.on(id, func(wparam, lparam uintptr) uintptr {
		fn()
		return 0
	})
}

func (l *Loop) OnThemeChange(fn func()) {
	l.mu.Lock()
	l.themeFns = append(l.themeFns, fn)
	l.mu.Unlock()
}

func (l *Loop) OnDisplayChange(fn func()) {
	l.mu.Lock()
	l.displayFns = append(l.displayFns, fn)
	l.mu.Unlock()
}

func (l *Loop) OnEndSession(fn func()) {
	l.mu.Lock()
	l.endSessionFns = append(l.endSessionFns, fn)
	l.mu.Unlock()
}

func (l *Loop) Hotkeys() *Hotkeys { return l.hotkeys }
