//go:build windows

package serialport

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"

	"go.bug.st/serial"
	"golang.org/x/sys/windows"
)

const probeWindow = 4 * time.Second

type dropReason int

const (
	dropError dropReason = iota
	dropReconnect
	dropProbeTimeout
	dropCanceled
)

func openPort(name string, baud int) (serial.Port, error) {
	if baud <= 0 {
		baud = core.DefaultBaud
	}
	// InitialStatusBits left nil: DTR and RTS come up on, which is what resets the CH340 and
	// its kin on open.
	port, err := serial.Open(name, &serial.Mode{
		BaudRate: baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return nil, err
	}
	if err := port.SetReadTimeout(250 * time.Millisecond); err != nil {
		port.Close()
		return nil, err
	}
	return port, nil
}

func streamPort(ctx context.Context, port serial.Port, reconnect <-chan struct{}, probe bool, onLine func([]int), connected func()) dropReason {
	reader := core.NewLineReader()
	buf := make([]byte, 256)
	deadline := time.Now().Add(probeWindow)
	gotLine := false

	for {
		select {
		case <-ctx.Done():
			return dropCanceled
		case <-reconnect:
			return dropReconnect
		default:
		}

		n, err := port.Read(buf)
		if err != nil {
			return dropError
		}
		if n == 0 {
			// A 0-byte, nil-error read is the Windows read timeout firing, not EOF.
			if probe && !gotLine && time.Now().After(deadline) {
				return dropProbeTimeout
			}
			continue
		}

		for _, values := range reader.Feed(buf[:n]) {
			if !gotLine {
				gotLine = true
				connected()
			}
			onLine(values)
		}
	}
}

// go.bug.st/serial turns ERROR_ACCESS_DENIED (another app, usually deej, holds the port) into
// PortBusy and drops the errno, so the PortBusy check is the one that fires.
func isAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	var portErr *serial.PortError
	if errors.As(err, &portErr) && portErr.Code() == serial.PortBusy {
		return true
	}
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "access is denied")
}
