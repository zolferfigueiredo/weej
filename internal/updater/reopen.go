//go:build windows

package updater

import (
	"os"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

func Reopen(self string, args []string) error {
	return sys.Detach(self, core.RestartArgs(args, os.Getpid()))
}

func Cleanup(self string) {
	os.Remove(self + ".old")
}
