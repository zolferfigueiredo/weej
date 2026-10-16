package core

import (
	"reflect"
	"testing"
)

func TestExeName(t *testing.T) {
	cases := map[string]string{
		`C:\x\Spotify.exe`:       "spotify.exe",
		`C:\Program Files\A.exe`: "a.exe",
		`/usr/bin/vlc`:           "vlc",
		"chrome.exe":             "chrome.exe",
	}
	for in, want := range cases {
		if got := ExeName(in); got != want {
			t.Errorf("ExeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTrimExePath(t *testing.T) {
	cases := map[string]string{
		`"C:\Opera\opera.exe"`:            `C:\Opera\opera.exe`,
		`"C:\Opera\opera.exe",0`:          `C:\Opera\opera.exe`,
		`C:\Steam\steam.exe,0`:            `C:\Steam\steam.exe`,
		`C:\x\app.exe, -101`:              `C:\x\app.exe`,
		`  C:\Program Files\A, B\a.exe  `: `C:\Program Files\A, B\a.exe`,
		`"C:\x\a.exe" --flag`:             `C:\x\a.exe`,
		"":                                "",
	}
	for in, want := range cases {
		if got := TrimExePath(in); got != want {
			t.Errorf("TrimExePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSteamLibraries(t *testing.T) {
	vdf := `"libraryfolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
		"label"		""
		"apps"
		{
			"730"		"36843211223"
		}
	}
	"1"
	{
		"path"		"D:\\SteamLibrary"
	}
}`
	got := SteamLibraries(vdf)
	want := []string{`C:\Program Files (x86)\Steam`, `D:\SteamLibrary`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SteamLibraries() = %v, want %v", got, want)
	}
}

func TestOtherApps(t *testing.T) {
	sessions := []string{"chrome.exe", "spotify.exe", "weej.exe", "discord.exe"}
	mapped := []string{"Spotify.exe"}
	got := OtherApps(sessions, mapped, "weej.exe")
	want := []string{"chrome.exe", "discord.exe"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OtherApps() = %v, want %v", got, want)
	}
}
