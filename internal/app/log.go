//go:build windows

package app

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

type logger struct {
	mu     sync.Mutex
	w      io.WriteCloser
	stdout bool
	last   string
}

func newLogger(w io.WriteCloser) *logger {
	return &logger{w: w, stdout: sys.StdoutIsTerminal()}
}

func (lg *logger) Log(msg string) {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	if msg == lg.last {
		return
	}
	lg.last = msg
	line := "WeeJ: " + msg
	if lg.w != nil {
		fmt.Fprintln(lg.w, time.Now().Format("2006-01-02 15:04:05")+" "+line)
	}
	if lg.stdout {
		fmt.Println(line)
	}
}

func (lg *logger) Close() {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	if lg.w != nil {
		lg.w.Close()
	}
}
