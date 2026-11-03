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

// pendingKey holds a level per app, so two app knobs turned at once both land.
type pendingKey struct {
	kind targetKind
	exe  string
}

type pendingValue struct {
	level float64
	exes  []string // otherApps: the apps a knob names, which it leaves alone
}

type keepRequest struct {
	apps            []string
	focused, others bool
}

// Audio owns every COM object behind a single worker goroutine: WASAPI's audio session API
// isn't safe to fan out across goroutines, so all of it (device enumeration, session volume,
// endpoint volume) is funneled through one loop instead.
type Audio struct {
	log func(string)

	mu      sync.Mutex
	pending map[pendingKey]pendingValue
	keep    *keepRequest
	wake    chan struct{}

	queries   chan chan []string
	levels    chan levelQuery
	micToggle chan chan MicState

	deviceChanged chan struct{}
	startErr      chan error
	stop          chan struct{}
	stopped       chan struct{}
	closeOnce     sync.Once

	selfExe string

	enumerator    *wca.IMMDeviceEnumerator
	notifyClient  *deviceNotifier
	exeCache      map[uint32]string
	knownSessions map[string]struct{}
	loggedOnce    map[string]bool

	haveSystemSoundsTarget bool
	systemSoundsLevel      float64

	volumes core.AppVolumes
}

func Start(log func(string)) (*Audio, error) {
	if log == nil {
		log = func(string) {}
	}

	a := &Audio{
		log:           log,
		pending:       map[pendingKey]pendingValue{},
		wake:          make(chan struct{}, 1),
		queries:       make(chan chan []string),
		levels:        make(chan levelQuery),
		micToggle:     make(chan chan MicState),
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

func (a *Audio) enqueue(key pendingKey, v pendingValue) {
	a.mu.Lock()
	a.pending[key] = v
	a.mu.Unlock()
	a.poke()
}

func (a *Audio) poke() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *Audio) SetMaster(s float64) {
	a.enqueue(pendingKey{kind: targetMaster}, pendingValue{level: s})
}

func (a *Audio) SetMic(s float64) { a.enqueue(pendingKey{kind: targetMic}, pendingValue{level: s}) }

func (a *Audio) SetSystemSounds(s float64) {
	a.enqueue(pendingKey{kind: targetSystemSounds}, pendingValue{level: s})
}

func (a *Audio) SetApp(exe string, s float64) {
	a.enqueue(pendingKey{kind: targetApps, exe: strings.ToLower(exe)}, pendingValue{level: s})
}

func (a *Audio) SetOtherApps(mapped []string, s float64) {
	a.enqueue(pendingKey{kind: targetOtherApps}, pendingValue{level: s, exes: lowerAll(mapped)})
}

func (a *Audio) SetFocused(s float64) {
	a.enqueue(pendingKey{kind: targetFocused}, pendingValue{level: s})
}

// Keep tells which apps the profiles in use turn, as core.AppVolumes.Keep takes them.
func (a *Audio) Keep(apps []string, focused, others bool) {
	a.mu.Lock()
	a.keep = &keepRequest{apps: lowerAll(apps), focused: focused, others: others}
	a.mu.Unlock()
	a.poke()
}

// MicState is the default microphone after a mute: OK is false when there is none.
type MicState struct {
	Muted, OK bool
	Level     float64
}

// ToggleMicMute mutes the default microphone, or unmutes it if it is muted.
func (a *Audio) ToggleMicMute() MicState {
	resp := make(chan MicState, 1)
	select {
	case a.micToggle <- resp:
	case <-a.stopped:
		return MicState{}
	}
	select {
	case s := <-resp:
		return s
	case <-a.stopped:
		return MicState{}
	}
}

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

type levelQuery struct {
	job  core.Job
	resp chan float64
}

// Level is where a job's volume is now, 0 to 1, as readLevel finds it. It can't tell any other
// job's, nor an app's with no sound open that no knob has set.
func (a *Audio) Level(job core.Job) (float64, bool) {
	q := levelQuery{job: job, resp: make(chan float64, 1)}
	select {
	case a.levels <- q:
	case <-a.stopped:
		return 0, false
	}
	select {
	case level := <-q.resp:
		return level, level >= 0
	case <-a.stopped:
		return 0, false
	}
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
		case q := <-a.levels:
			q.resp <- a.readLevel(q.job)
		case resp := <-a.micToggle:
			resp <- a.toggleMicMute()
		}
	}
}

// drainPending applies apps before Other apps and Focused app last, so an app Focused app turns
// is no longer one of Other apps by the next turn.
func (a *Audio) drainPending() {
	a.mu.Lock()
	snapshot, keep := a.pending, a.keep
	a.pending, a.keep = map[pendingKey]pendingValue{}, nil
	a.mu.Unlock()

	if keep != nil {
		a.volumes.Keep(keep.apps, keep.focused, keep.others)
	}
	if v, ok := snapshot[pendingKey{kind: targetMaster}]; ok {
		a.applyMaster(v.level)
	}
	if v, ok := snapshot[pendingKey{kind: targetMic}]; ok {
		a.applyMic(v.level)
	}
	if v, ok := snapshot[pendingKey{kind: targetSystemSounds}]; ok {
		a.haveSystemSoundsTarget = true
		a.systemSoundsLevel = v.level
		a.applySystemSounds(v.level)
	}
	apps := map[string]float64{}
	for k, v := range snapshot {
		if k.kind == targetApps {
			apps[k.exe] = v.level
			a.volumes.Set(k.exe, v.level)
		}
	}
	if len(apps) > 0 {
		a.applyApps(apps)
	}
	if v, ok := snapshot[pendingKey{kind: targetOtherApps}]; ok {
		a.volumes.SetOthers(v.exes, v.level)
		a.applyOtherApps(v.level)
	}
	if v, ok := snapshot[pendingKey{kind: targetFocused}]; ok {
		a.applyFocused(v.level)
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
