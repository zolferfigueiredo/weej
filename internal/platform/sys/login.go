//go:build windows

package sys

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath          = `Software\Microsoft\Windows\CurrentVersion\Run`
	startupApprovedPath = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	runValueName        = "WeeJ"
)

func SetLogin(on bool, exe string, args []string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("sys: open Run key: %w", err)
	}
	defer k.Close()

	if !on {
		if err := k.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("sys: delete Run value: %w", err)
		}
		return nil
	}
	if err := k.SetStringValue(runValueName, runCommand(exe, args)); err != nil {
		return fmt.Errorf("sys: set Run value: %w", err)
	}
	return nil
}

func runCommand(exe string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, windows.EscapeArg(exe))
	for _, a := range args {
		parts = append(parts, windows.EscapeArg(a))
	}
	return strings.Join(parts, " ")
}

func LoginEnabled(exe string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	value, _, err := k.GetStringValue(runValueName)
	if err != nil || !commandPointsTo(value, exe) {
		return false
	}
	return !startupSuppressed()
}

func startupSuppressed() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, startupApprovedPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	data, _, err := k.GetBinaryValue(runValueName)
	if err != nil {
		return false
	}
	return startupApprovedDisabled(data)
}

// startupApprovedDisabled reads the first byte of the StartupApproved\Run binary value: Explorer
// writes 0x02/0x06 for an entry the user left enabled and 0x03/0x07 for one they disabled.
func startupApprovedDisabled(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return data[0] == 0x03 || data[0] == 0x07
}

func commandExe(commandLine string) string {
	s := strings.TrimSpace(commandLine)
	if s == "" {
		return ""
	}
	if s[0] == '"' {
		rest := s[1:]
		if end := strings.IndexByte(rest, '"'); end >= 0 {
			return rest[:end]
		}
		return rest
	}
	if idx := strings.IndexAny(s, " \t"); idx >= 0 {
		return s[:idx]
	}
	return s
}

func commandPointsTo(commandLine, exe string) bool {
	return strings.EqualFold(commandExe(commandLine), exe)
}

func InstalledExe() string {
	return filepath.Join(ExpandEnv("%LOCALAPPDATA%"), "Programs", "WeeJ", "WeeJ.exe")
}

func IsInstalledCopy(self string) bool {
	return strings.EqualFold(filepath.Clean(self), filepath.Clean(InstalledExe()))
}

func IsScoopCopy(self string) bool {
	return strings.Contains(strings.ToLower(self), `\scoop\apps\`)
}
