//go:build windows

package sys

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/windows"
)

const startupInfoEnv = "WEEJ_TEST_STARTUPINFO_OUT"

// A copy started the way Detach starts one (after an update, or with --detach) must not be
// told to start hidden: Windows applies that to the process's first ShowWindow, so the first
// Settings window it opened stayed hidden.
func TestDetachedCopyIsNotStartedHidden(t *testing.T) {
	if out := os.Getenv(startupInfoEnv); out != "" {
		var si windows.StartupInfo
		_ = windows.GetStartupInfo(&si)
		hidden := si.Flags&windows.STARTF_USESHOWWINDOW != 0 && si.ShowWindow == windows.SW_HIDE
		_ = os.WriteFile(out, []byte(strconv.FormatBool(hidden)), 0o600)
		return
	}

	out := filepath.Join(t.TempDir(), "hidden.txt")
	cmd := exec.Command(os.Args[0], "-test.run=^TestDetachedCopyIsNotStartedHidden$")
	cmd.Env = append(os.Environ(), startupInfoEnv+"="+out)
	cmd.SysProcAttr = detachAttr()
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Error("the detached copy was started with SW_HIDE, so its first window would stay hidden")
	}
}
