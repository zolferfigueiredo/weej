//go:build windows

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

// debugLog writes down everything that moves a level, while its switch is on, to find out why one
// changes by itself. It stays on this PC. Its switch is a file among the logs, so a Windows restart,
// which clears the folder, also turns it off.
type debugLog struct {
	on   atomic.Bool
	dir  string
	mu   sync.Mutex
	f    *os.File
	size int64
}

const (
	debugMarker  = "on"
	debugFileMax = 200 << 20
	debugDirMax  = 1 << 30
)

func debugDir() string { return filepath.Join(os.TempDir(), "WeeJ", "debug") }

func newDebugLog(dir string) *debugLog {
	d := &debugLog{dir: dir}
	clearBefore(dir, sys.BootTime())
	if _, err := os.Stat(filepath.Join(dir, debugMarker)); err == nil {
		d.on.Store(true)
	}
	return d
}

func (d *debugLog) On() bool { return d.on.Load() }

func (d *debugLog) Set(on bool) error {
	marker := filepath.Join(d.dir, debugMarker)
	if on {
		if err := os.MkdirAll(d.dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(marker, nil, 0o644); err != nil {
			return err
		}
		d.on.Store(true)
		return nil
	}
	d.on.Store(false)
	d.mu.Lock()
	d.closeFile()
	d.mu.Unlock()
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Trace writes one line under tag, formatting it only while the switch is on.
func (d *debugLog) Trace(tag, format string, args ...any) {
	if d == nil || !d.on.Load() {
		return
	}
	now := time.Now()
	line := now.Format("15:04:05.000") + " [" + tag + "] " + fmt.Sprintf(format, args...) + "\n"
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.on.Load() {
		return
	}
	if d.f == nil || d.size+int64(len(line)) > debugFileMax {
		d.closeFile()
		if err := d.openFile(now); err != nil {
			return
		}
	}
	n, _ := d.f.WriteString(line)
	d.size += int64(n)
}

func (d *debugLog) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closeFile()
}

func (d *debugLog) closeFile() {
	if d.f != nil {
		d.f.Close()
		d.f = nil
	}
}

func (d *debugLog) openFile(now time.Time) error {
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return err
	}
	pruneTo(d.dir, debugDirMax-debugFileMax)
	f, err := os.Create(filepath.Join(d.dir, "debug-"+now.Format("20060102-150405.000")+".log"))
	if err != nil {
		return err
	}
	d.f, d.size = f, 0
	return nil
}

// setDebugLog turns the debug log on or off at once, from Settings. Turning it on writes down the
// version and every board's controls first, so the lines after can be read.
func (app *App) setDebugLog(on bool) {
	if err := app.debug.Set(on); err != nil {
		app.log("Could not turn the debug log on or off: " + err.Error())
	}
	if app.debug.On() {
		app.trace("start", "WeeJ %s, debug log on", core.AppVersion)
		app.printSetup(func(s string) { app.trace("setup", "%s", s) })
	}
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "debugLog", "on": app.debug.On()})
	}
}

func (app *App) openDebugFolder() {
	if err := os.MkdirAll(app.debug.dir, 0o755); err == nil {
		launch(app.debug.dir)
	}
}

// clearBefore deletes what the folder holds from before t, the switch included.
func clearBefore(dir string, t time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().Before(t) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// pruneTo deletes the oldest logs until the folder holds at most max bytes of them.
func pruneTo(dir string, max int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type logFile struct {
		path string
		size int64
		mod  time.Time
	}
	var logs []logFile
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.Name() == debugMarker {
			continue
		}
		logs = append(logs, logFile{filepath.Join(dir, e.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	slices.SortFunc(logs, func(a, b logFile) int { return a.mod.Compare(b.mod) })
	for _, l := range logs {
		if total <= max {
			return
		}
		if os.Remove(l.path) == nil {
			total -= l.size
		}
	}
}
