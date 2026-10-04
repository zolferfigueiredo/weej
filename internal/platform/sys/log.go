//go:build windows

package sys

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func OpenLog(path string, maxBytes int64) (io.WriteCloser, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("sys: create log dir: %w", err)
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() > maxBytes {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return nil, fmt.Errorf("sys: open log %s: %w", path, err)
	}
	return f, nil
}
