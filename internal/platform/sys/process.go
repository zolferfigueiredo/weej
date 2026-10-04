//go:build windows

package sys

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/zolferfigueiredo/weej/internal/core"
)

var (
	user32Process        = windows.NewLazySystemDLL("user32.dll")
	procGetSystemMetrics = user32Process.NewProc("GetSystemMetrics")
)

const smShuttingDown = 0x2000

func Detach(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags:    windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		HideWindow:       true,
		NoInheritHandles: true,
	}
	return cmd.Start()
}

func KeepAlive(exe string, args []string, log func(string)) int {
	supervisor := core.NewSupervisor()
	for {
		cmd := exec.Command(exe, args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		runErr := cmd.Run()
		var exitErr *exec.ExitError
		if runErr != nil && !errors.As(runErr, &exitErr) && log != nil {
			log("Could not start " + exe + ": " + runErr.Error())
		}
		exitCode := exitCodeOf(runErr)

		restart, giveUp := supervisor.Decide(exitCode, systemShuttingDown(), time.Now())
		if giveUp {
			if log != nil {
				log("Giving up after repeated crashes")
			}
			return 1
		}
		if !restart {
			return 0
		}
		if log != nil {
			log("Restarting after exit code")
		}
		time.Sleep(supervisor.Delay)
	}
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func systemShuttingDown() bool {
	r, _, _ := procGetSystemMetrics.Call(uintptr(smShuttingDown))
	return r != 0
}

func WaitPID(pid uint32, timeout time.Duration) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return true
	}
	defer windows.CloseHandle(h)
	event, err := windows.WaitForSingleObject(h, uint32(timeout.Milliseconds()))
	return err == nil && event == windows.WAIT_OBJECT_0
}
