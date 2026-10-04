//go:build windows

package sys

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func SettingsPath() string {
	return filepath.Join(ExpandEnv("%APPDATA%"), "WeeJ", "settings.json")
}

func LocalDir() string {
	return filepath.Join(ExpandEnv("%LOCALAPPDATA%"), "WeeJ")
}

func ExpandEnv(s string) string {
	src, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return s
	}
	n, err := windows.ExpandEnvironmentStrings(src, nil, 0)
	if err != nil || n == 0 {
		return s
	}
	buf := make([]uint16, n)
	if _, err := windows.ExpandEnvironmentStrings(src, &buf[0], n); err != nil {
		return s
	}
	return windows.UTF16ToString(buf)
}

// WriteFileAtomic writes through a temp file in the same directory so a reader never sees a
// partial write, then swaps it into place with MOVEFILE_WRITE_THROUGH so the rename itself is
// durable, not just queued.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("sys: create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".weej-*.tmp")
	if err != nil {
		return fmt.Errorf("sys: create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sys: write %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sys: flush %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("sys: close %s: %w", tmpPath, err)
	}

	fromPtr, err := windows.UTF16PtrFromString(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return err
	}
	toPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := windows.MoveFileEx(fromPtr, toPtr, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("sys: replace %s: %w", path, err)
	}
	return nil
}
