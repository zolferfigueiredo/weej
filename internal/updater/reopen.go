//go:build windows

package updater

import (
	"os"
	"strconv"

	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

func Reopen(self string, args []string) error {
	full := make([]string, 0, len(args)+2)
	full = append(full, args...)
	full = append(full, "--wait-pid", strconv.Itoa(os.Getpid()))
	return sys.Detach(self, full)
}

func Cleanup(self string) {
	os.Remove(self + ".old")
}
