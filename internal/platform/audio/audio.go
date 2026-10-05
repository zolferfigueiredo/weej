//go:build windows

package audio

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca/pkg/wca"

	"github.com/zolferfigueiredo/weej/internal/core"
)

// comAlreadyInitialized is S_FALSE: CoInitializeEx returns it when some other code on this
// thread already called it. That's fine, not a real error.
const comAlreadyInitialized = 1

// Arbitrary, fixed event context so SetMasterVolume/SetMasterVolumeLevelScalar calls identify
// themselves consistently; some audio consumers key off this to tell apart callers.
var eventCtx = ole.NewGUID("{5e2c9f3a-9b7f-4a1e-9c2d-7a6f0b6e4d10}")

type targetKind int

const (
	targetMaster targetKind = iota
	targetMic
	targetSystemSounds
	targetApps
	targetOtherApps
	targetFocused
)

type pendingValue struct {
	level float64
	exes  []string // apps: the exe allowlist; otherApps: the mapped list to exclude
}

// Audio owns every COM object behind a single worker goroutine: WASAPI's audio session API
// isn't safe to fan out across goroutines, so all of it (device enumeration, session volume,
// endpoint volume) is funneled through one loop instead.
type Audio struct {
	log func(string)

	mu      sync.Mutex
	pending map[targetKind]pendingValue
	wake    chan struct{}

	queries chan chan []string

	deviceChanged chan struct{}
	startErr      chan error
	stop          chan struct{}
	stopped       chan struct{}
	closeOnce     sync.Once

	focusMu    sync.Mutex
	focusedExe string

	selfExe string

	enumerator    *wca.IMMDeviceEnumerator
	notifyClient  *deviceNotifier
	exeCache      map[uint32]string
	knownSessions map[string]struct{}
	loggedOnce    map[string]bool

	haveSystemSoundsTarget bool
	systemSoundsLevel      float64

	haveAppsTarget bool
	appsLevel      float64
	appsExes       map[string]struct{}

	haveOtherAppsTarget bool
	otherAppsLevel      float64
	otherAppsMapped     []string
}

func Start(log func(string)) (*Audio, error) {
	if log == nil {
		log = func(string) {}
	}

	a := &Audio{
		log:           log,
		pending:       map[targetKind]pendingValue{},
		wake:          make(chan struct{}, 1),
		queries:       make(chan chan []string),
		deviceChanged: make(chan struct{}, 1),
		startErr:      make(chan error, 1),
		stop:          make(chan struct{}),
		stopped:       make(chan struct{}),
		exeCache:      map[uint32]string{},
		knownSessions: map[string]struct{}{},
		loggedOnce:    map[string]bool{},
		selfExe:       resolveSelfExe(),
	}

	go a.run()

	if err := <-a.startErr; err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Audio) Close() {
	a.closeOnce.Do(func() {
		close(a.stop)
		<-a.stopped
	})
}

func resolveSelfExe() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	return core.ExeName(exePath)
}

func (a *Audio) enqueue(kind targetKind, v pendingValue) {
	a.mu.Lock()
	a.pending[kind] = v
	a.mu.Unlock()

	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *Audio) SetMaster(s float64) { a.enqueue(targetMaster, pendingValue{level: s}) }

func (a *Audio) SetMic(s float64) { a.enqueue(targetMic, pendingValue{level: s}) }

func (a *Audio) SetSystemSounds(s float64) { a.enqueue(targetSystemSounds, pendingValue{level: s}) }

func (a *Audio) SetApps(exes []string, s float64) {
	a.enqueue(targetApps, pendingValue{level: s, exes: lowerAll(exes)})
}

func (a *Audio) SetOtherApps(mapped []string, s float64) {
	a.enqueue(targetOtherApps, pendingValue{level: s, exes: lowerAll(mapped)})
}

func (a *Audio) SetFocused(s float64) { a.enqueue(targetFocused, pendingValue{level: s}) }

// Playing lists lowercase exes with an active session on a visible top-level window. Unlike
// the Set* calls this needs a result, so it round-trips through the worker instead of just
// dropping a value in the pending map.
func (a *Audio) Playing() []string {
	respCh := make(chan []string, 1)
	select {
	case a.queries <- respCh:
	case <-a.stopped:
		return nil
	}
	select {
	case res := <-respCh:
		return res
	case <-a.stopped:
		return nil
	}
}

// FocusedExe returns the last exe SetFocused resolved the foreground window to, so the HUD can
// show its icon without forcing a round trip through the worker.
func (a *Audio) FocusedExe() string {
	a.focusMu.Lock()
	defer a.focusMu.Unlock()
	return a.focusedExe
}

func (a *Audio) setFocusedExe(exe string) {
	a.focusMu.Lock()
	a.focusedExe = exe
	a.focusMu.Unlock()
}

func (a *Audio) run() {
	defer close(a.stopped)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil && !benignCoInitError(err) {
		a.startErr <- err
		return
	}
	defer ole.CoUninitialize()

	if err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator,
		0,
		wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator,
		&a.enumerator,
	); err != nil {
		a.startErr <- err
		return
	}
	defer a.enumerator.Release()

	a.registerDeviceChangeNotification()

	a.startErr <- nil

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.stop:
			return
		case <-a.wake:
			a.drainPending()
		case <-ticker.C:
			a.pollNewSessions()
		case <-a.deviceChanged:
			a.pollNewSessions()
		case respCh := <-a.queries:
			respCh <- a.computePlaying()
		}
	}
}

func (a *Audio) drainPending() {
	a.mu.Lock()
	snapshot := a.pending
	a.pending = map[targetKind]pendingValue{}
	a.mu.Unlock()

	for kind, v := range snapshot {
		switch kind {
		case targetMaster:
			a.applyMaster(v.level)
		case targetMic:
			a.applyMic(v.level)
		case targetSystemSounds:
			a.haveSystemSoundsTarget = true
			a.systemSoundsLevel = v.level
			a.applySystemSounds(v.level)
		case targetApps:
			a.haveAppsTarget = true
			a.appsLevel = v.level
			a.appsExes = toSet(v.exes)
			a.applyApps(v.level, a.appsExes)
		case targetOtherApps:
			a.haveOtherAppsTarget = true
			a.otherAppsLevel = v.level
			a.otherAppsMapped = v.exes
			a.applyOtherApps(v.level, v.exes)
		case targetFocused:
			a.applyFocused(v.level)
		}
	}
}

func (a *Audio) logOnce(cause, msg string) {
	if a.loggedOnce[cause] {
		return
	}
	a.loggedOnce[cause] = true
	a.log(msg)
}

func benignCoInitError(err error) bool {
	var oleErr *ole.OleError
	if errors.As(err, &oleErr) {
		return oleErr.Code() == comAlreadyInitialized
	}
	return false
}

func lowerAll(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ToLower(v)
	}
	return out
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}
