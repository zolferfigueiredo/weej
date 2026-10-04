//go:build windows

package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

func buildZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeZipFile(t *testing.T, path, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, buildZip(t, name, content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sha256HexOfFile(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestInstallOnlyInstalledCopy(t *testing.T) {
	self := filepath.Join(t.TempDir(), "WeeJ.exe")
	err := Install(context.Background(), DefaultSite, Release{Version: "1.2.3"}, self, nil)

	var uerr *Error
	if !errors.As(err, &uerr) || uerr.Key != "installed_only" {
		t.Fatalf("Install() = %v, want installed_only", err)
	}
}

func TestSwap(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "WeeJ.exe")
	newExe := filepath.Join(dir, "WeeJ.exe.new")
	writeFile(t, self, "old")
	writeFile(t, newExe, "new")

	if err := swap(self, newExe); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, self, "new")
	assertFileContent(t, self+".old", "old")
}

func TestSwapRestoresOnFailure(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "WeeJ.exe")
	writeFile(t, self, "old")

	// no newExe file exists at this path, so the second rename fails and self must come back.
	if err := swap(self, filepath.Join(dir, "missing.new")); err == nil {
		t.Fatal("want an error when the new exe is missing")
	}
	assertFileContent(t, self, "old")
}

func TestVersionMatches(t *testing.T) {
	cases := []struct {
		got, want string
		match     bool
	}{
		{"1.2.3", "1.2.3", true},
		{"1.2.3.0", "1.2.3", true},
		{"1.2.4", "1.2.3", false},
		{"1.2", "1.2.3", false},
	}
	for _, c := range cases {
		if got := versionMatches(c.got, c.want); got != c.match {
			t.Errorf("versionMatches(%q, %q) = %v, want %v", c.got, c.want, got, c.match)
		}
	}
}

// TestInstallRejectsSHAMismatch drives the real download step against an httptest server, the
// way a stale or tampered release asset would behave in production.
func TestInstallRejectsSHAMismatch(t *testing.T) {
	zipBytes := buildZip(t, "WeeJ.exe", []byte("whatever happens to be in the zip"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipBytes)
	}))
	defer server.Close()

	rel := Release{Version: "1.2.3", SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}
	self := filepath.Join(t.TempDir(), "WeeJ.exe")

	err := install(context.Background(), server.URL+"/", rel, self, nil)

	var uerr *Error
	if !errors.As(err, &uerr) || uerr.Key != "download_failed" {
		t.Fatalf("install() = %v, want download_failed", err)
	}
}

func TestVerifyAndExtractMissingEntry(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "WeeJ-1.2.3-x64.zip")
	writeZipFile(t, zipPath, "LICENSE", []byte("mit"))

	rel := Release{Version: "1.2.3", SHA256: sha256HexOfFile(t, zipPath)}
	err := verifyAndExtract(zipPath, rel, filepath.Join(dir, "WeeJ.exe.new"))

	var uerr *Error
	if !errors.As(err, &uerr) || uerr.Key != "wrong_download" {
		t.Fatalf("verifyAndExtract() = %v, want wrong_download", err)
	}
}

func TestVerifyAndExtractWrongVersion(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "WeeJ-1.2.3-x64.zip")
	writeZipFile(t, zipPath, "WeeJ.exe", []byte("not a real exe, so it carries no version resource"))

	rel := Release{Version: "1.2.3", SHA256: sha256HexOfFile(t, zipPath)}
	err := verifyAndExtract(zipPath, rel, filepath.Join(dir, "WeeJ.exe.new"))

	var uerr *Error
	if !errors.As(err, &uerr) || uerr.Key != "wrong_download" {
		t.Fatalf("verifyAndExtract() = %v, want wrong_download", err)
	}
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "WeeJ.exe")
	writeFile(t, self+".old", "leftover")

	Cleanup(self)

	if _, err := os.Stat(self + ".old"); !os.IsNotExist(err) {
		t.Errorf("Cleanup() left %s.old behind", self)
	}

	Cleanup(filepath.Join(t.TempDir(), "WeeJ.exe")) // must not panic when there's nothing to remove
}
