//go:build windows

package updater

import (
	"testing"

	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

func TestWhichCopy(t *testing.T) {
	cases := []struct {
		name string
		self string
		want Copy
	}{
		{"installed", sys.InstalledExe(), Installed},
		{"scoop", `C:\Users\x\scoop\apps\weej\current\WeeJ.exe`, Scoop},
		{"other", `C:\Users\x\Desktop\WeeJ.exe`, Other},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := WhichCopy(c.self); got != c.want {
				t.Errorf("WhichCopy(%q) = %v, want %v", c.self, got, c.want)
			}
		})
	}
}

func TestDownloadURL(t *testing.T) {
	cases := []struct {
		name string
		site string
		want string
	}{
		{"default site", DefaultSite, "https://github.com/zolferfigueiredo/weej/releases/download/v1.2.3/WeeJ-1.2.3-x64.zip"},
		{"test feed", "https://example.com/feeds/", "https://example.com/feeds/WeeJ-1.2.3-x64.zip"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DownloadURL(c.site, "1.2.3"); got != c.want {
				t.Errorf("DownloadURL(%q, 1.2.3) = %q, want %q", c.site, got, c.want)
			}
		})
	}
}

func TestPageURL(t *testing.T) {
	if got := PageURL("1.2.3"); got != "https://github.com/zolferfigueiredo/weej/releases/tag/v1.2.3" {
		t.Errorf("PageURL(1.2.3) = %q", got)
	}
}
