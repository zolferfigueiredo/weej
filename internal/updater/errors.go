//go:build windows

package updater

// Key is a lang catalog key such as download_failed, wrong_download or installed_only, which the
// caller translates. Detail is the underlying cause, for logs, not for display.
type Error struct {
	Key    string
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Key
	}
	return e.Key + ": " + e.Detail
}
