//go:build windows

package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"
)

func TestClearBeforeKeepsOnlyWhatCameAfter(t *testing.T) {
	dir := t.TempDir()
	boot := time.Now().Add(-time.Hour)
	for name, mod := range map[string]time.Time{
		"old.log":       boot.Add(-time.Minute),
		oldChangeSwitch: boot.Add(-time.Minute),
		"new.log":       boot.Add(time.Minute),
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	clearBefore(dir, boot)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "new.log" {
		t.Errorf("left %v, want only new.log: the switch and the old log go with the boot", entries)
	}
}

func TestChangeLogWritesOnlyWhileOn(t *testing.T) {
	d := newChangeLog(t.TempDir())
	d.Note("before")
	d.Set(true)
	d.Note("Speaker volume: %d%% to %d%%", 71, 55)
	d.Set(false)
	d.Note("after")

	entries, _ := os.ReadDir(d.dir)
	if !core.IsBeta(core.AppVersion) {
		if len(entries) != 0 {
			t.Fatalf("a release wrote %v, want no change log", entries)
		}
		return
	}
	if len(entries) != 1 {
		t.Fatalf("folder holds %v, want one log and no switch", entries)
	}
	data, _ := os.ReadFile(filepath.Join(d.dir, entries[0].Name()))
	if got := string(data); len(got) < 20 || got[14:] != "Speaker volume: 71% to 55%\n" {
		t.Errorf("log = %q, want only the line written while on", got)
	}
}
