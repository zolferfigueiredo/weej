//go:build windows

package nightlight

import (
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/zolferfigueiredo/weej/internal/core"
)

// Windows keeps Night light in CloudStore blobs and applies them live when they change. There
// is a default copy and, on newer builds, a per-device copy of each; both are kept in step.
const store = `Software\Microsoft\Windows\CurrentVersion\CloudStore\Store\DefaultAccount\Current`

const (
	stateName    = "windows.data.bluelightreduction.bluelightreductionstate"
	settingsName = "windows.data.bluelightreduction.settings"

	// Stamps run ahead of the clock by at most this many seconds while a knob turns; see run.
	maxLead = 2
)

// NightLight drives Windows Night light from a knob: off at the bottom, on anywhere above it,
// from barely warm up to the strongest warmth, like the strength slider in Settings.
type NightLight struct {
	log  func(string)
	mu   sync.Mutex
	want float64
	wake chan struct{}
}

func New(log func(string)) *NightLight {
	n := &NightLight{log: log, want: -1, wake: make(chan struct{}, 1)}
	go n.run()
	return n
}

func (n *NightLight) Set(s float64) {
	if s < 0 {
		s = 0
	}
	if s > 1 {
		s = 1
	}
	n.mu.Lock()
	n.want = s
	n.mu.Unlock()
	select {
	case n.wake <- struct{}{}:
	default:
	}
}

// Windows ignores a blob stamped older than the one it last applied (maybe an equal one
// too), and stamps have one-second resolution. Each write is therefore stamped a second past
// the newest stamp in the store; a fast turn may run up to maxLead seconds ahead of the
// clock, and past that the writes wait, so the latest knob position always lands, at worst
// a second late.
func (n *NightLight) run() {
	applied := -1 // percent last written; -1 until the first write
	warned := false
	for range n.wake {
		for {
			n.mu.Lock()
			s := n.want
			n.mu.Unlock()
			if s < 0 || core.Percent(s) == applied {
				break
			}
			pct := core.Percent(s)
			blobs, err := readAll()
			if err != nil || len(blobs) == 0 {
				if !warned {
					n.log("Night light is unavailable: its settings were not found")
					warned = true
				}
				break
			}
			now := time.Now().Unix()
			stamp := now
			for _, b := range blobs {
				if ts, err := core.NightLightStamp(b.data); err == nil && ts+1 > stamp {
					stamp = ts + 1
				}
			}
			if stamp > now+maxLead {
				time.Sleep(time.Until(time.Unix(stamp-maxLead, 0)))
				continue
			}
			if err := apply(blobs, pct, time.Unix(stamp, 0)); err != nil && !warned {
				n.log("Could not change Night light: " + err.Error())
				warned = true
			}
			applied = pct
		}
	}
}

// Toggle turns Night light off if it is on, or on at its own warmth if it is off.
func (n *NightLight) Toggle() {
	go func() {
		blobs, err := readAll()
		if err != nil || len(blobs) == 0 {
			n.log("Night light is unavailable: its settings were not found")
			return
		}
		on := false
		stamp := time.Now().Unix()
		for _, b := range blobs {
			if b.state {
				if isOn, err := core.IsOn(b.data); err == nil && isOn {
					on = true
				}
			}
			if ts, err := core.NightLightStamp(b.data); err == nil && ts+1 > stamp {
				stamp = ts + 1
			}
		}
		for _, b := range blobs {
			if !b.state {
				continue
			}
			next, err := core.SetOn(b.data, !on, time.Unix(stamp, 0))
			if err == nil {
				err = write(b.path, next)
			}
			if err != nil {
				n.log("Could not change Night light: " + err.Error())
				return
			}
		}
	}()
}

type blob struct {
	path  string
	state bool
	data  []byte
}

func readAll() ([]blob, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, store, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	names, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return nil, err
	}
	var out []blob
	for _, name := range names {
		i := strings.IndexByte(name, '$')
		if i < 0 {
			continue
		}
		inner := name[i+1:]
		var state bool
		switch inner {
		case stateName, stateName + "perdevice":
			state = true
		case settingsName, settingsName + "perdevice":
		default:
			continue
		}
		path := store + `\` + name + `\` + inner
		sk, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		data, _, err := sk.GetBinaryValue("Data")
		sk.Close()
		if err != nil {
			continue
		}
		out = append(out, blob{path: path, state: state, data: data})
	}
	return out, nil
}

// Settings go first, so turning on lands at the knob's warmth rather than the old one.
func apply(blobs []blob, pct int, stamp time.Time) error {
	on := pct > 0
	var firstErr error
	keep := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, pass := range []bool{false, true} {
		for _, b := range blobs {
			if b.state != pass {
				continue
			}
			var next []byte
			var err error
			if b.state {
				if isOn, e := core.IsOn(b.data); e == nil && isOn == on {
					continue
				}
				next, err = core.SetOn(b.data, on, stamp)
			} else {
				if !on {
					continue
				}
				next, err = core.SetKelvin(b.data, core.NightLightKelvin(float64(pct)/100), stamp)
			}
			if err != nil {
				keep(err)
				continue
			}
			keep(write(b.path, next))
		}
	}
	return firstErr
}

func write(path string, data []byte) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetBinaryValue("Data", data)
}
