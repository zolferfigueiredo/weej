//go:build windows

package via

import (
	"sync"

	"github.com/zolferfigueiredo/weej/internal/core"
)

type VIA struct {
	log func(string)

	mu      sync.Mutex
	pending *float64

	wake chan struct{}
	done chan struct{}
	wg   sync.WaitGroup
}

func NewVIA(log func(string)) *VIA {
	v := &VIA{
		log:  log,
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}
	v.wg.Add(1)
	go v.run()
	return v
}

func (v *VIA) Set(s float64) {
	v.mu.Lock()
	v.pending = &s
	v.mu.Unlock()
	select {
	case v.wake <- struct{}{}:
	default:
	}
}

func (v *VIA) Close() {
	close(v.done)
	v.wg.Wait()
}

func (v *VIA) run() {
	defer v.wg.Done()

	var devices []*hidDevice
	defer func() { closeDevices(devices) }()

	for {
		select {
		case <-v.done:
			return
		case <-v.wake:
		}

		v.mu.Lock()
		p := v.pending
		v.pending = nil
		v.mu.Unlock()
		if p == nil {
			continue
		}

		// Nothing cached yet, either because this is the first write or because the
		// last enumeration found no VIA keyboard; try again so a keyboard plugged in
		// after launch still gets picked up.
		if len(devices) == 0 {
			devices = findKeyboards(v.log)
		}
		if len(devices) == 0 {
			continue
		}

		if !writeAll(devices, *p) {
			closeDevices(devices)
			devices = findKeyboards(v.log)
			if len(devices) > 0 {
				writeAll(devices, *p)
			}
		}
	}
}

func writeAll(devices []*hidDevice, s float64) bool {
	reports := core.ViaReports(s)
	ok := true
	for _, d := range devices {
		for _, r := range reports {
			if err := d.write(r); err != nil {
				ok = false
			}
		}
	}
	return ok
}

func closeDevices(devices []*hidDevice) {
	for _, d := range devices {
		d.close()
	}
}
