//go:build windows

package display

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	dxva2 = windows.NewLazySystemDLL("dxva2.dll")

	procGetNumberOfPhysicalMonitorsFromHMONITOR = dxva2.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	procGetPhysicalMonitorsFromHMONITOR         = dxva2.NewProc("GetPhysicalMonitorsFromHMONITOR")
	procDestroyPhysicalMonitors                 = dxva2.NewProc("DestroyPhysicalMonitors")
	procGetVCPFeatureAndVCPFeatureReply         = dxva2.NewProc("GetVCPFeatureAndVCPFeatureReply")
	procSetVCPFeature                           = dxva2.NewProc("SetVCPFeature")
)

const ddcMinInterval = 50 * time.Millisecond

type physicalMonitor struct {
	Handle      windows.Handle
	Description [128]uint16
}

type ddcKey struct {
	screen int
	vcp    byte
}

// DDC serializes DDC/CI writes through one goroutine: dxva2 wants them one at a time, and
// several targets settle on the same physical monitor if they're written too close together.
type DDC struct {
	log func(string)

	mu      sync.Mutex
	pending map[ddcKey]int
	wake    chan struct{}
	done    chan struct{}
	wg      sync.WaitGroup
}

func NewDDC(log func(string)) *DDC {
	d := &DDC{
		log:     log,
		pending: make(map[ddcKey]int),
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	d.wg.Add(1)
	go d.run()
	return d
}

func (d *DDC) Set(screen int, vcp byte, percent int) {
	d.mu.Lock()
	d.pending[ddcKey{screen, vcp}] = percent
	d.mu.Unlock()
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *DDC) Close() {
	close(d.done)
	d.wg.Wait()
}

func (d *DDC) run() {
	defer d.wg.Done()

	maxCache := map[ddcKey]uint32{}
	loggedScreens := map[int]bool{}
	var lastWrite time.Time

	for {
		select {
		case <-d.done:
			return
		case <-d.wake:
		}

		for {
			d.mu.Lock()
			var key ddcKey
			var percent int
			found := false
			for k, v := range d.pending {
				key, percent, found = k, v, true
				break
			}
			if found {
				delete(d.pending, key)
			}
			d.mu.Unlock()
			if !found {
				break
			}

			if wait := ddcMinInterval - time.Since(lastWrite); wait > 0 {
				select {
				case <-time.After(wait):
				case <-d.done:
					return
				}
			}
			d.write(key.screen, key.vcp, percent, maxCache, loggedScreens)
			lastWrite = time.Now()
		}
	}
}

func (d *DDC) write(screen int, vcp byte, percent int, maxCache map[ddcKey]uint32, loggedScreens map[int]bool) {
	var target *Monitor
	for _, m := range Monitors() {
		if m.Screen == screen {
			target = &m
			break
		}
	}
	if target == nil {
		return // screen beyond the connected count: ignore it
	}

	var count uint32
	ok, _, _ := procGetNumberOfPhysicalMonitorsFromHMONITOR.Call(target.Handle, uintptr(unsafe.Pointer(&count)))
	if ok == 0 || count == 0 {
		d.logOnce(loggedScreens, screen, fmt.Sprintf("Could not get the physical monitor for screen %d", screen))
		return
	}

	monitors := make([]physicalMonitor, count)
	ok, _, _ = procGetPhysicalMonitorsFromHMONITOR.Call(target.Handle, uintptr(count), uintptr(unsafe.Pointer(&monitors[0])))
	if ok == 0 {
		d.logOnce(loggedScreens, screen, fmt.Sprintf("Could not open the physical monitor for screen %d", screen))
		return
	}
	defer procDestroyPhysicalMonitors.Call(uintptr(count), uintptr(unsafe.Pointer(&monitors[0])))

	h := monitors[0].Handle
	key := ddcKey{screen, vcp}
	max, cached := maxCache[key]
	if !cached {
		var current, maximum uint32
		ret, _, _ := procGetVCPFeatureAndVCPFeatureReply.Call(uintptr(h), uintptr(vcp), 0, uintptr(unsafe.Pointer(&current)), uintptr(unsafe.Pointer(&maximum)))
		if ret == 0 || maximum == 0 {
			max = 100
		} else {
			max = maximum
		}
		maxCache[key] = max
	}

	value := uint32(percent) * max / 100
	ret, _, _ := procSetVCPFeature.Call(uintptr(h), uintptr(vcp), uintptr(value))
	if ret == 0 {
		d.logOnce(loggedScreens, screen, fmt.Sprintf("Could not set VCP feature 0x%02X on screen %d", vcp, screen))
	}
}

func (d *DDC) logOnce(logged map[int]bool, screen int, msg string) {
	if logged[screen] {
		return
	}
	logged[screen] = true
	if d.log != nil {
		d.log(msg)
	}
}
