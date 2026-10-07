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

// buttonLEDs is what SetLED last set each button's LED to, by id.
var buttonLEDs [256]atomic.Bool

// SetLED lights or clears a button's LED, the way the mode the mixer is in takes it.
func SetLED(button int, on bool) {
	if button >= 0 && button < len(buttonLEDs) {
		buttonLEDs[button].Store(on)
	}
	if msg, ok := core.SMCButtonLED(button, on, mode.DAW()); ok {
		send(msg)
	}
}

type Config struct {
	Device   string
	OnValues func(values []int)
	OnButton func(id int)
	OnStatus func(connected, busy bool)
	// Lights lights an SMC-Mixer's controls as they are used (smcLights).
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
	strips := &smcLights{cfg: cfg, state: state, start: time.Now()}
	defer strips.allOff()

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
		case <-strips.tick:
			strips.due()
		case msg := <-ch:
			changed := false
			feed := func(m uint32) {
				mode.Seen(m)
				c, pressed := state.Feed(m)
				changed = changed || c
				if c {
					strips.moved(state.LastChanged())
				}
				if pressed >= 0 {
					strips.held(pressed, true)
					if cfg.OnButton != nil {
						cfg.OnButton(pressed)
					}
				}
				if id, ok := core.SMCButtonReleased(m); ok {
					strips.held(id, false)
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

// smcLights lights an SMC-Mixer's controls as they are used, in DAW mode. A button lights while it
// is held. The LED over a fader blinks while the strip's knob turns: sending the fader channel a
// position far from the fader starts it, and the fader's own stops it. A moving fader puts that LED
// out itself, so the fader's own moves are left alone.
type smcLights struct {
	cfg    Config
	state  *core.MixerState
	lights core.StripLights
	start  time.Time
	ticker *time.Ticker
	tick   <-chan time.Time
	// blink is the message keeping each strip blinking, 0 when none was sent.
	blink [8]uint32
}

func (s *smcLights) now() float64 { return time.Since(s.start).Seconds() }

func (s *smcLights) moved(col int) {
	strip, ok := core.SMCStripOf(col)
	if !s.cfg.Lights || !ok || col != 30+strip {
		return
	}
	s.lights.Move(strip, s.now())
	if s.ticker == nil {
		s.ticker = time.NewTicker(50 * time.Millisecond)
		s.tick = s.ticker.C
	}
	_, msb, ok := s.state.Pitch(strip)
	if !ok || !mode.DAW() {
		return
	}
	if msg := core.SMCStripBlink(strip, msb); msg != s.blink[strip] {
		send(msg)
		s.blink[strip] = msg
	}
}

func (s *smcLights) due() {
	for _, strip := range s.lights.Due(s.now()) {
		s.off(strip)
	}
	if !s.lights.Any() {
		s.ticker.Stop()
		s.ticker, s.tick = nil, nil
	}
}

func (s *smcLights) off(strip int) {
	if s.blink[strip] == 0 {
		return
	}
	if lsb, msb, ok := s.state.Pitch(strip); ok {
		send(core.SMCStripRestore(strip, lsb, msb))
	}
	s.blink[strip] = 0
}

func (s *smcLights) allOff() {
	for _, strip := range s.lights.AllOff() {
		s.off(strip)
	}
	if s.ticker != nil {
		s.ticker.Stop()
	}
}

// held lights a button while it is down, and gives it back what SetLED last set when it comes up.
func (s *smcLights) held(id int, down bool) {
	if !s.cfg.Lights {
		return
	}
	on := down
	if !down && id >= 0 && id < len(buttonLEDs) {
		on = buttonLEDs[id].Load()
	}
	if msg, ok := core.SMCButtonLED(id, on, mode.DAW()); ok {
		send(msg)
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
