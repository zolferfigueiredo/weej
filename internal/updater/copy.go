//go:build windows

package updater

import (
	"runtime"

	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

type Copy int

const (
	Installed Copy = iota
	Other
)

func WhichCopy(self string) Copy {
	switch {
	case sys.IsInstalledCopy(self):
		return Installed
	default:
		return Other
	}
}

// Arch names this build's assets in a release (WeeJ-<version>-x64.zip) and its
// checksum in latest.json.
func Arch() string {
	if runtime.GOARCH == "386" {
		return "x86"
	}
	return "x64"
}

func DownloadURL(site, version string) string {
	name := "WeeJ-" + version + "-" + Arch() + ".zip"
	if site == DefaultSite {
		return "https://github.com/zolferfigueiredo/weej/releases/download/v" + version + "/" + name
	}
	return site + name
}

func PageURL(version string) string {
	return "https://github.com/zolferfigueiredo/weej/releases/tag/v" + version
}
