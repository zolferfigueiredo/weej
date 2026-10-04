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

func TestOtherApps(t *testing.T) {
	sessions := []string{"chrome.exe", "spotify.exe", "weej.exe", "discord.exe"}
	mapped := []string{"Spotify.exe"}
	got := OtherApps(sessions, mapped, "weej.exe")
	want := []string{"chrome.exe", "discord.exe"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OtherApps() = %v, want %v", got, want)
	}
}
