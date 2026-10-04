//go:build windows

package updater

import "github.com/zolferfigueiredo/weej/internal/platform/sys"

type Copy int

const (
	Installed Copy = iota
	Scoop
	Other
)

func WhichCopy(self string) Copy {
	switch {
	case sys.IsInstalledCopy(self):
		return Installed
	case sys.IsScoopCopy(self):
		return Scoop
	default:
		return Other
	}
}

func DownloadURL(site, version string) string {
	if site == DefaultSite {
		return "https://github.com/zolferfigueiredo/weej/releases/download/v" + version + "/WeeJ-" + version + "-x64.zip"
	}
	return site + "WeeJ-" + version + "-x64.zip"
}

func PageURL(version string) string {
	return "https://github.com/zolferfigueiredo/weej/releases/tag/v" + version
}
