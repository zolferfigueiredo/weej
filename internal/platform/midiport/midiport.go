//go:build windows

package midiport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/zolferfigueiredo/weej/internal/core"

	"golang.org/x/sys/windows"
)

var (
	modWinMM = windows.NewLazySystemDLL("winmm.dll")

	procMidiInGetNumDevs   = modWinMM.NewProc("midiInGetNumDevs")
	procMidiInGetDevCapsW  = modWinMM.NewProc("midiInGetDevCapsW")
	procMidiInOpen         = modWinMM.NewProc("midiInOpen")
	procMidiInStart        = modWinMM.NewProc("midiInStart")
	procMidiInStop         = modWinMM.NewProc("midiInStop")
	procMidiInReset        = modWinMM.NewProc("midiInReset")
	procMidiInClose        = modWinMM.NewProc("midiInClose")
	procMidiOutGetNumDevs  = modWinMM.NewProc("midiOutGetNumDevs")
	procMidiOutGetDevCapsW = modWinMM.NewProc("midiOutGetDevCapsW")
	procMidiOutOpen        = modWinMM.NewProc("midiOutOpen")
	procMidiOutShortMsg    = modWinMM.NewProc("midiOutShortMsg")
	procMidiOutClose       = modWinMM.NewProc("midiOutClose")
)

const (
	mmsyserrAllocated = 4
	callbackFunction  = 0x00030000
	mimData           = 0x3C3
)

// Mirrors MIDIINCAPSW.
type midiInCaps struct {
	mid, pid      uint16
	driverVersion uint32
	name          [32]uint16
	support       uint32
}

// Mirrors MIDIOUTCAPSW.
type midiOutCaps struct {
	mid, pid      uint16
	driverVersion uint32
	name          [32]uint16
	technology    uint16
	voices        uint16
	notes         uint16
	channelMask   uint16
	support       uint32
}

var errBusy = errors.New("midiport: device in use")

// WinMM calls this on its own thread, where calling back into winmm can deadlock, so it only
// hands the message over. One callback serves every open, since Go never frees callbacks.
var (
	current  atomic.Pointer[chan uint32]
	callback = windows.NewCallback(func(h, msg, instance, param1, param2 uintptr) uintptr {
		if msg == mimData {
			if ch := current.Load(); ch != nil {
				select {
				case *ch <- uint32(param1):
				default:
				}
			}
		}
		return 0
	})
)

func List() []string {
	n, _, _ := procMidiInGetNumDevs.Call()
	out := make([]string, 0, n)
	for i := uintptr(0); i < n; i++ {
		var caps midiInCaps
		if r, _, _ := procMidiInGetDevCapsW.Call(i, uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps)); r == 0 {
			out = append(out, windows.UTF16ToString(caps.name[:]))
		}
	}
	return out
}

func findInput(name string) (uintptr, bool) {
	for i, n := range List() {
		if n == name {
			return uintptr(i), true
		}
	}
	return 0, false
}

func findOutput(name string) (uintptr, bool) {
	n, _, _ := procMidiOutGetNumDevs.Call()
	for i := uintptr(0); i < n; i++ {
		var caps midiOutCaps
		if r, _, _ := procMidiOutGetDevCapsW.Call(i, uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps)); r == 0 &&
			windows.UTF16ToString(caps.name[:]) == name {
			return i, true
		}
	}
	return 0, false
}

func openInput(id uintptr, ch chan uint32) (uintptr, error) {
	current.Store(&ch)
	var h uintptr
	r, _, _ := procMidiInOpen.Call(uintptr(unsafe.Pointer(&h)), id, callback, 0, callbackFunction)
	if r != 0 {
		current.Store(nil)
		if r == mmsyserrAllocated {
			return 0, errBusy
		}
		return 0, fmt.Errorf("midiport: midiInOpen error %d", r)
	}
	if r, _, _ := procMidiInStart.Call(h); r != 0 {
		closeInput(h)
		return 0, fmt.Errorf("midiport: midiInStart error %d", r)
	}
	return h, nil
}

func closeInput(h uintptr) {
	procMidiInStop.Call(h)
	procMidiInReset.Call(h)
	procMidiInClose.Call(h)
	current.Store(nil)
}

var out struct {
	mu sync.Mutex
	h  uintptr
}

// mode outlives a connection, so a button's LED can be cleared before the mixer sends again.
var mode core.SMCMode

func openOutput(name string) {
	id, ok := findOutput(name)
	if !ok {
		return
	}
	var h uintptr
	if r, _, _ := procMidiOutOpen.Call(uintptr(unsafe.Pointer(&h)), id, 0, 0, 0); r != 0 {
		return
	}
	out.mu.Lock()
	out.h = h
	out.mu.Unlock()
}

func closeOutput() {
	out.mu.Lock()
	defer out.mu.Unlock()
	if out.h != 0 {
		procMidiOutClose.Call(out.h)
		out.h = 0
	}
}

func send(msg uint32) {
	out.mu.Lock()
	defer out.mu.Unlock()
	if out.h != 0 {
		procMidiOutShortMsg.Call(out.h, uintptr(msg))
	}
}

var (
	// muteLEDs is what SetLED last asked of each button's light, by id.
	muteLEDs     [256]atomic.Bool
	lightPattern atomic.Value
	lightLEDs    atomic.Bool
	spectrum     atomic.Pointer[core.Spectrum]
	lightsPoke   = make(chan struct{}, 1)
)

func pokeLights() {
	select {
	case lightsPoke <- struct{}{}:
	default:
	}
}

// SetLED shows a mute on a button's light, the way the mode the mixer is in takes it.
func SetLED(button int, on bool) {
	if button >= 0 && button < len(muteLEDs) {
		muteLEDs[button].Store(on)
		pokeLights()
	}
}

// SetSpectrum is the sound the "eq" pattern follows.
func SetSpectrum(s *core.Spectrum) { spectrum.Store(s) }

// SetLights picks the pattern an SMC-Mixer's button lights run (core.LightPatterns), and whether
// the LEDs over its faders join in.
func SetLights(pattern string, leds bool) {
	lightPattern.Store(core.ParseLightPattern(pattern))
	lightLEDs.Store(leds)
	pokeLights()
}

type Config struct {
	Device   string
	OnValues func(values []int)
	OnButton func(id int)
	OnStatus func(connected, busy bool)
	// Lights runs the SetLights pattern, on an SMC-Mixer.
	Lights bool
	Log    func(string)
}

type dropReason int

const (
	dropGone dropReason = iota
	dropReconnect
	dropCanceled
)

// Run is a blocking loop like serialport.Run: it opens the named MIDI input, feeds what the
// mixer sends to cfg, and reopens it whenever it goes away. It returns once ctx is done.
func Run(ctx context.Context, cfg Config, reconnect <-chan struct{}) {
	logf := cfg.Log
	if logf == nil {
		logf = func(string) {}
	}
	type status struct{ connected, busy bool }
	var last *status
	setStatus := func(connected, busy bool) {
		s := status{connected, busy}
		if last != nil && *last == s {
			return
		}
		last = &s
		if cfg.OnStatus != nil {
			cfg.OnStatus(connected, busy)
		}
	}

	for {
		if ctx.Err() != nil {
			return
		}
		id, ok := findInput(cfg.Device)
		if !ok {
			if last == nil || last.connected {
				logf("Waiting for MIDI device " + cfg.Device)
			}
			setStatus(false, false)
			if sleepDiscard(ctx, 2*time.Second, reconnect) {
				return
			}
			continue
		}

		ch := make(chan uint32, 1024)
		h, err := openInput(id, ch)
		if err != nil {
			logf(fmt.Sprintf("Could not open %s: %v", cfg.Device, err))
			setStatus(false, errors.Is(err, errBusy))
			if sleepDiscard(ctx, 2*time.Second, reconnect) {
				return
			}
			continue
		}
		openOutput(cfg.Device)
		logf("Connected: " + cfg.Device)
		setStatus(true, false)

		reason := stream(ctx, ch, cfg, reconnect)
		closeOutput()
		closeInput(h)
		if reason == dropCanceled {
			return
		}
		logf("Disconnected")
		setStatus(false, false)
		if sleepDiscard(ctx, time.Second, reconnect) {
			return
		}
	}
}

// stream hands the values over once per burst, so a fast fader sweep costs one frame, not one
// per message.
func stream(ctx context.Context, ch <-chan uint32, cfg Config, reconnect <-chan struct{}) dropReason {
	state := core.NewMixerState()
	check := time.NewTicker(2 * time.Second)
	defer check.Stop()
	lights := newButtonLights(cfg.Lights, state)
	var guard core.LightGuard
	lights.update()
	defer lights.clear()

	for {
		select {
		case <-ctx.Done():
			return dropCanceled
		case <-reconnect:
			return dropReconnect
		case <-check.C:
			if _, ok := findInput(cfg.Device); !ok {
				return dropGone
			}
		case <-lights.tick:
			lights.update()
		case <-lightsPoke:
			lights.update()
		case <-lights.paced:
			lights.flush()
		case msg := <-ch:
			changed := false
			feed := func(m uint32) {
				daw := mode.DAW()
				if mode.Seen(m); daw != mode.DAW() {
					lights.update()
				}
				if cfg.Lights && !guard.Pass(m, time.Since(lights.start0).Seconds(), lights.last.Sub(lights.start0).Seconds()) {
					return
				}
				c, pressed := state.Feed(m)
				changed = changed || c
				if c {
					lights.moved(state.LastChanged())
				}
				if pressed >= 0 {
					lights.hold(pressed, true)
					if cfg.OnButton != nil {
						cfg.OnButton(pressed)
					}
				}
				if id, ok := core.SMCButtonReleased(m); ok {
					lights.hold(id, false)
				}
			}
			feed(msg)
		drain:
			for {
				select {
				case m := <-ch:
					feed(m)
				default:
					break drain
				}
			}
			if changed && cfg.OnValues != nil {
				cfg.OnValues(state.Values())
			}
		}
	}
}

// buttonLights keeps each button's light as the pattern, its mute and a hand on it want it: lit
// when any of them says so. It starts over when the mixer changes mode, since a light is sent
// differently in each, and sends no more than lightsBurst changes every 10 ms: a real SMC-Mixer
// sent fader moves nobody made after a few dozen at once.
type buttonLights struct {
	patterns bool
	state    *core.MixerState
	pattern  string
	start    time.Time
	ticker   *time.Ticker
	tick     <-chan time.Time
	daw      bool
	held     [256]bool
	// strip is an SMC-Mixer's strip buttons, the only ones it lights, and which a pattern from
	// before may have left on.
	strip [256]bool
	// want and sent are each light as decided and as last sent: 0 unknown, 1 off, 2 on.
	want, sent [256]int8
	next       int
	// last is when a light was last sent, after start0, when these lights began.
	start0, last time.Time
	pace         *time.Ticker
	paced        <-chan time.Time
	// knobUntil is when each strip's LED stops blinking for its knob, and blink the message
	// keeping each blinking, 0 when none was sent.
	knobUntil [8]time.Time
	blink     [8]uint32
}

const (
	lightsBurst = 4
	// knobTail keeps a fader's LED blinking a moment after its knob stops, so it doesn't flicker.
	knobTail = 300 * time.Millisecond
)

func newButtonLights(patterns bool, state *core.MixerState) *buttonLights {
	l := &buttonLights{patterns: patterns, state: state, start0: time.Now()}
	if patterns {
		for _, id := range core.SMCStripButtons() {
			l.strip[id] = true
		}
	}
	return l
}

func (l *buttonLights) hold(id int, down bool) {
	if id >= 0 && id < len(l.held) && (!l.patterns || l.strip[id]) {
		l.held[id] = down
		l.update()
	}
}

// moved hears a column change: a knob blinks the LED over its fader, and a fader puts that LED
// out on the mixer itself, so it is sent again if still wanted.
func (l *buttonLights) moved(col int) {
	if !l.patterns {
		return
	}
	switch {
	case col >= 30 && col < 38:
		l.knobUntil[col-30] = time.Now().Add(knobTail)
		l.update()
	case col >= 40 && col < 48:
		l.blink[col-40] = 0
	}
}

func (l *buttonLights) update() {
	pattern, leds := "", false
	if l.patterns {
		pattern, _ = lightPattern.Load().(string)
		leds = lightLEDs.Load()
	}
	now := time.Now()
	if pattern != l.pattern {
		l.pattern, l.start = pattern, now
	}
	knobs := false
	for _, until := range l.knobUntil {
		knobs = knobs || now.Before(until)
	}
	if tick := core.Animated(pattern) || knobs || leds && pattern != ""; tick != (l.ticker != nil) {
		if tick {
			l.ticker = time.NewTicker(40 * time.Millisecond)
			l.tick = l.ticker.C
		} else {
			l.ticker.Stop()
			l.ticker, l.tick = nil, nil
		}
	}
	if daw := mode.DAW(); daw != l.daw {
		l.daw, l.want, l.sent, l.blink = daw, [256]int8{}, [256]int8{}, [8]uint32{}
	}
	frame := core.LightFrame(pattern, time.Since(l.start).Seconds())
	switch pattern {
	case "eq":
		if sp := spectrum.Load(); sp != nil {
			frame = core.EQFrame(sp.Bands(time.Since(l.start).Seconds()))
		}
	case "clock":
		frame = core.ClockFrame(time.Now())
	}
	var lit [256]bool
	for _, id := range frame {
		lit[id] = true
	}
	for id := range lit {
		switch {
		case lit[id] || l.held[id] || muteLEDs[id].Load():
			l.want[id] = 2
		case l.sent[id] != 0 || l.strip[id]:
			l.want[id] = 1
		}
	}
	l.flush()

	var stripLEDs [8]bool
	if leds {
		stripLEDs = core.StripLEDs(frame)
	}
	for strip := range stripLEDs {
		l.setLED(strip, stripLEDs[strip] || now.Before(l.knobUntil[strip]))
	}
}

// setLED starts or stops a fader's LED blinking, once that fader has said where it is. A blinking
// LED counts as a light change for the fader drift guard, since it flashes on its own.
func (l *buttonLights) setLED(strip int, on bool) {
	lsb, msb, ok := l.state.Pitch(strip)
	if !ok || !l.daw {
		return
	}
	switch {
	case on:
		if msg := core.SMCStripBlink(strip, msb); msg != l.blink[strip] {
			send(msg)
			l.blink[strip] = msg
		}
		l.last = time.Now()
	case l.blink[strip] != 0:
		send(core.SMCStripRestore(strip, lsb, msb))
		l.blink[strip] = 0
		l.last = time.Now()
	}
}

// flush sends up to lightsBurst of the lights that differ from what was sent, taking turns, and
// keeps pacing itself until none differ.
func (l *buttonLights) flush() {
	budget := lightsBurst
	for range len(l.want) {
		id := l.next
		l.next = (l.next + 1) % len(l.want)
		if l.want[id] == 0 || l.want[id] == l.sent[id] {
			continue
		}
		if budget == 0 {
			if l.pace == nil {
				l.pace = time.NewTicker(10 * time.Millisecond)
				l.paced = l.pace.C
			}
			return
		}
		budget--
		l.last = time.Now()
		l.sent[id] = l.want[id]
		if msg, ok := core.SMCButtonLED(id, l.want[id] == 2, l.daw); ok {
			send(msg)
		}
	}
	if l.pace != nil {
		l.pace.Stop()
		l.pace, l.paced = nil, nil
	}
}

func (l *buttonLights) clear() {
	for _, t := range []*time.Ticker{l.ticker, l.pace} {
		if t != nil {
			t.Stop()
		}
	}
	for strip := range l.blink {
		l.setLED(strip, false)
	}
	n := 0
	for id, state := range l.sent {
		if state != 2 {
			continue
		}
		if msg, ok := core.SMCButtonLED(id, false, l.daw); ok {
			send(msg)
		}
		if n++; n%lightsBurst == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func sleepDiscard(ctx context.Context, d time.Duration, reconnect <-chan struct{}) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return true
		case <-timer.C:
			return false
		case <-reconnect:
		}
	}
}
