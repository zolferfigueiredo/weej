//go:build windows

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

// changeLog writes a line each time a level or a setting changes, with what changed it, while its
// switch is on, to find out why one moves by itself. It stays on this PC, and a Windows restart
// clears its logs. The switch itself is a setting (core.Settings.ChangeLog), kept across restarts.
type changeLog struct {
	on   atomic.Bool
	dir  string
	mu   sync.Mutex
	f    *os.File
	size int64
}

const (
	changeFileMax = 200 << 20
	changeDirMax  = 1 << 30
	// oldChangeSwitch is the file 2.7.0-beta.2 kept its switch as, read once to carry it over.
	oldChangeSwitch = "on"
)

func changeDir() string { return filepath.Join(os.TempDir(), "WeeJ", "changes") }

func newChangeLog(dir string) *changeLog {
	d := &changeLog{dir: dir}
	clearBefore(dir, sys.BootTime())
	return d
}

func (d *changeLog) On() bool { return d != nil && d.on.Load() }

func (d *changeLog) Set(on bool) {
	d.on.Store(on)
	if !on {
		d.Close()
	}
}

// carryOldSwitch tells whether 2.7.0-beta.2 left its switch on this boot, and removes that file.
func (d *changeLog) carryOldSwitch() bool {
	path := filepath.Join(d.dir, oldChangeSwitch)
	if _, err := os.Stat(path); err != nil {
		return false
	}
	_ = os.Remove(path)
	return true
}

// Note writes one line, formatting it only while the switch is on.
func (d *changeLog) Note(format string, args ...any) {
	if !d.On() {
		return
	}
	now := time.Now()
	line := now.Format("15:04:05.000") + "  " + fmt.Sprintf(format, args...) + "\n"
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.on.Load() {
		return
	}
	if d.f == nil || d.size+int64(len(line)) > changeFileMax {
		d.closeFile()
		if err := d.openFile(now); err != nil {
			return
		}
	}
	n, _ := d.f.WriteString(line)
	d.size += int64(n)
}

func (d *changeLog) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closeFile()
}

func (d *changeLog) closeFile() {
	if d.f != nil {
		d.f.Close()
		d.f = nil
	}
}

func (d *changeLog) openFile(now time.Time) error {
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return err
	}
	pruneTo(d.dir, changeDirMax-changeFileMax)
	f, err := os.Create(filepath.Join(d.dir, "changes-"+now.Format("20060102-150405")+".log"))
	if err != nil {
		return err
	}
	d.f, d.size = f, 0
	return nil
}

// note writes to the change log while its switch is on.
func (app *App) note(format string, args ...any) { app.changes.Note(format, args...) }

// setChangeLog turns the change log on or off at once, from Settings.
func (app *App) setChangeLog(on bool) {
	app.changes.Set(on)
	s := app.snapshotSettings()
	s.ChangeLog = on
	if err := app.persistSettings(s); err != nil {
		app.log("Could not save settings: " + err.Error())
	}
	app.note("WeeJ %s: change log on", core.AppVersion)
	if win := app.settingsWin; win != nil {
		win.Send(map[string]any{"type": "changeLog", "on": app.changes.On()})
	}
}

func (app *App) openChangeFolder() {
	if err := os.MkdirAll(app.changes.dir, 0o755); err == nil {
		launch(app.changes.dir)
	}
}

// noteControl writes down a control whose move changed its jobs, in the words Settings uses.
func (app *App) noteControl(id string, col int, from, to float64, jobs []core.Job) {
	if !app.changes.On() {
		return
	}
	d, ok := app.device(id)
	if !ok || col >= len(d.Controls) {
		return
	}
	tr := app.trFunc()
	titles := make([]string, len(jobs))
	for i, j := range jobs {
		titles[i] = j.Title(tr, app.appDisplayName)
	}
	app.note("%s, %s %s: %d%% to %d%%, sets %s", d.Name, d.Controls[col].Kind, core.Letter(col),
		core.Percent(from), core.Percent(to), strings.Join(titles, ", "))
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
		if err != nil {
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
