//go:build windows

package serialport

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

type Status struct {
	Connected bool
	Port      string
	Busy      bool
}

type Config struct {
	// Owner names the board, so two boards never open the same port: opening one restarts the
	// Arduino on it.
	Owner string
	// Exclude lists ports other boards use, which finding a board automatically leaves alone.
	Exclude    func() []string
	ForcedPort string
	// Baud is the serial speed; 0 means core.DefaultBaud.
	Baud     int
	OnLine   func(values []int)
	OnStatus func(Status)
	Log      func(string)
}

// Run is a blocking loop, porting TheeJ's serialLoop (Serial.swift) to Windows. It never
// returns until ctx is done.
func Run(ctx context.Context, cfg Config, reconnect <-chan struct{}) {
	logf := cfg.Log
	if logf == nil {
		logf = func(string) {}
	}
	onLine := cfg.OnLine
	if onLine == nil {
		onLine = func([]int) {}
	}

	var lastStatus Status
	setStatus := func(s Status) {
		if s == lastStatus {
			return
		}
		lastStatus = s
		if cfg.OnStatus != nil {
			cfg.OnStatus(s)
		}
	}

	for {
		if ctx.Err() != nil {
			return
		}

		candidates := pickCandidates(cfg.ForcedPort)
		if cfg.ForcedPort == "" && cfg.Exclude != nil {
			skip := cfg.Exclude()
			candidates = slices.DeleteFunc(candidates, func(name string) bool { return slices.Contains(skip, name) })
		}
		candidates = slices.DeleteFunc(candidates, func(name string) bool { return claimedBy(name, cfg.Owner) })
		if len(candidates) == 0 {
			logf("Waiting for a serial device")
			setStatus(Status{})
			if sleepDiscard(ctx, 2*time.Second, reconnect) {
				return
			}
			continue
		}

		// Several candidates means we don't yet know which one is the real device, so a
		// silent one gets abandoned after a few seconds. A forced or single candidate has
		// nowhere else to go, so it is worth waiting on indefinitely.
		probe := len(candidates) > 1

	candidateLoop:
		for _, name := range candidates {
			if ctx.Err() != nil {
				return
			}

			if !claim(name, cfg.Owner) {
				continue
			}
			port, err := openPort(name, cfg.Baud)
			if err != nil {
				release(name, cfg.Owner)
				logf(fmt.Sprintf("Could not open %s", name))
				busy := isAccessDenied(err)
				busyPort := ""
				if busy {
					busyPort = name
				}
				setStatus(Status{Busy: busy, Port: busyPort})
				if sleepDiscard(ctx, 2*time.Second, reconnect) {
					return
				}
				continue
			}

			connected := func() {
				logf(fmt.Sprintf("Connected: %s", name))
				setStatus(Status{Connected: true, Port: name})
			}
			reason := streamPort(ctx, port, reconnect, probe, onLine, connected)
			port.Close()
			release(name, cfg.Owner)

			switch reason {
			case dropCanceled:
				return
			case dropProbeTimeout:
				// Try the next candidate right away: no log, no status change, since this
				// one was never reported as connected.
				continue candidateLoop
			default:
				logf("Disconnected")
				setStatus(Status{})
				if sleepDiscard(ctx, 1*time.Second, reconnect) {
					return
				}
				break candidateLoop
			}
		}
		// Recompute candidates from scratch: the device set may have changed.
	}
}

// sleepDiscard waits for d, draining (and ignoring) any reconnect signal that arrives while
// disconnected so it never latches and fires the instant a port opens, as it did in TheeJ.
// It reports whether ctx ended the wait early.
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

// claims holds which board has each port open.
var claims = struct {
	sync.Mutex
	owners map[string]string
}{owners: map[string]string{}}

func claim(port, owner string) bool {
	claims.Lock()
	defer claims.Unlock()
	if o, ok := claims.owners[port]; ok && o != owner {
		return false
	}
	claims.owners[port] = owner
	return true
}

func release(port, owner string) {
	claims.Lock()
	defer claims.Unlock()
	if claims.owners[port] == owner {
		delete(claims.owners, port)
	}
}

func claimedBy(port, owner string) bool {
	claims.Lock()
	defer claims.Unlock()
	o, ok := claims.owners[port]
	return ok && o != owner
}
