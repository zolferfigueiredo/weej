//go:build windows

package updater

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/zolferfigueiredo/weej/internal/core"
)

type Step int

const (
	Downloading Step = iota
	Checking
	Installing
	Done
)

// Inno's AppId for WeeJ; DisplayVersion lives under this uninstall key.
const uninstallKeyPath = `Software\Microsoft\Windows\CurrentVersion\Uninstall\{82E4CB37-0DB9-4E44-B883-8C04E719EA7D}_is1`

func Install(ctx context.Context, site string, rel Release, self string, progress func(Step)) error {
	if WhichCopy(self) != Installed {
		return &Error{Key: "installed_only"}
	}
	return install(ctx, site, rel, self, progress)
}

// install is Install's body, split out so tests can run the download, verify and swap pipeline
// against a temp-dir self path without tripping the installed-copy gate.
func install(ctx context.Context, site string, rel Release, self string, progress func(Step)) error {
	dir := filepath.Dir(self)
	zipPath := filepath.Join(dir, "WeeJ-"+rel.Version+".download")
	newExe := filepath.Join(dir, "WeeJ.exe.new")

	report(progress, Downloading)
	if err := downloadZip(ctx, DownloadURL(site, rel.Version), zipPath); err != nil {
		return err
	}
	defer os.Remove(zipPath)

	report(progress, Checking)
	if err := verifyAndExtract(zipPath, rel, newExe); err != nil {
		os.Remove(newExe)
		return err
	}

	report(progress, Installing)
	if err := swap(self, newExe); err != nil {
		os.Remove(newExe)
		return err
	}
	setUninstallDisplayVersion(rel.Version)

	report(progress, Done)
	return nil
}

func report(progress func(Step), s Step) {
	if progress != nil {
		progress(s)
	}
}

// downloadZip writes straight to dest in dir(self), so the later rename into place stays on the
// same volume and is atomic.
func downloadZip(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &Error{Key: "download_failed", Detail: err.Error()}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &Error{Key: "download_failed", Detail: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &Error{Key: "download_failed", Detail: resp.Status}
	}

	f, err := os.Create(dest)
	if err != nil {
		return &Error{Key: "download_failed", Detail: err.Error()}
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return &Error{Key: "download_failed", Detail: err.Error()}
	}
	if err := f.Close(); err != nil {
		return &Error{Key: "download_failed", Detail: err.Error()}
	}
	return nil
}

func verifyAndExtract(zipPath string, rel Release, newExe string) error {
	sumFile, err := os.Open(zipPath)
	if err != nil {
		return &Error{Key: "download_failed", Detail: err.Error()}
	}
	sumErr := core.CheckSHA256(sumFile, rel.SHA256)
	sumFile.Close()
	if sumErr != nil {
		return &Error{Key: "download_failed", Detail: sumErr.Error()}
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return &Error{Key: "download_failed", Detail: err.Error()}
	}
	defer zr.Close()

	var entry *zip.File
	for _, zf := range zr.File {
		if strings.EqualFold(zf.Name, "WeeJ.exe") {
			entry = zf
			break
		}
	}
	if entry == nil {
		return &Error{Key: "wrong_download", Detail: "zip has no WeeJ.exe"}
	}
	if err := extractEntry(entry, newExe); err != nil {
		return &Error{Key: "download_failed", Detail: err.Error()}
	}

	got, err := productVersion(newExe)
	if err != nil {
		return &Error{Key: "wrong_download", Detail: err.Error()}
	}
	if !versionMatches(got, rel.Version) {
		return &Error{Key: "wrong_download", Detail: "got " + got}
	}
	return nil
}

// versionMatches allows the trailing ".0" that Windows adds to a three-part version when the
// file version resource always carries four parts.
func versionMatches(got, want string) bool {
	return got == want || got == want+".0"
}

func extractEntry(zf *zip.File, dest string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// swap is the rename dance Windows allows on a running exe: self moves aside first and the new
// build takes its place. A failed second rename restores the old exe rather than leaving self
// missing.
func swap(self, newExe string) error {
	old := self + ".old"
	os.Remove(old) // a leftover from a previous failed update must not block this rename

	if err := os.Rename(self, old); err != nil {
		return fmt.Errorf("updater: rename %s: %w", self, err)
	}
	if err := os.Rename(newExe, self); err != nil {
		if rerr := os.Rename(old, self); rerr != nil {
			return fmt.Errorf("updater: restore %s after failed install: %w (original error: %v)", self, rerr, err)
		}
		return fmt.Errorf("updater: rename %s into place: %w", newExe, err)
	}
	return nil
}

func setUninstallDisplayVersion(version string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKeyPath, registry.SET_VALUE)
	if err != nil {
		return // not installed via the installer: nothing to update
	}
	defer k.Close()
	k.SetStringValue("DisplayVersion", version)
}
