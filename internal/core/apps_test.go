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

func TestAppVolumesKeepALevelPerApp(t *testing.T) {
	var v AppVolumes
	v.Set("Spotify.exe", 0.2)
	v.Set("chrome.exe", 0.7)
	if s, _ := v.Level("spotify.exe", "weej.exe"); s != 0.2 {
		t.Errorf("spotify's level = %v, want 0.2", s)
	}
	if s, _ := v.Level("chrome.exe", "weej.exe"); s != 0.7 {
		t.Errorf("chrome's level = %v, want 0.7", s)
	}
	if _, ok := v.Level("discord.exe", "weej.exe"); ok {
		t.Error("an app no knob turned has a level")
	}
}

func TestOtherAppsLeaveOutAppsWithAVolumeOfTheirOwn(t *testing.T) {
	var v AppVolumes
	v.SetOthers([]string{"Spotify.exe"}, 0.3)
	v.Set("discord.exe", 0.9) // as Focused app turned it
	for _, exe := range []string{"spotify.exe", "discord.exe", "weej.exe"} {
		if v.IsOther(exe, "weej.exe") {
			t.Errorf("%s is one of Other apps", exe)
		}
	}
	if s, ok := v.Level("chrome.exe", "weej.exe"); !ok || s != 0.3 {
		t.Errorf("chrome's level = %v, %v, want Other apps' 0.3", s, ok)
	}
}

func TestAppVolumesKeepOnlyWhatProfilesInUseHave(t *testing.T) {
	var v AppVolumes
	v.Set("spotify.exe", 0.2)
	v.Set("discord.exe", 0.9)
	v.SetOthers(nil, 0.3)

	v.Keep([]string{"spotify.exe"}, true, true)
	if _, ok := v.Level("discord.exe", ""); !ok {
		t.Error("an app Focused app turned was forgotten while a profile has Focused app")
	}

	v.Keep([]string{"spotify.exe"}, false, false)
	if _, ok := v.Level("discord.exe", ""); ok {
		t.Error("an app Focused app turned was kept with no Focused app in use")
	}
	if _, ok := v.Others(); ok {
		t.Error("Other apps' level was kept with no Other apps in use")
	}
	if v.IsOther("spotify.exe", "") {
		t.Error("an app a profile in use names became one of Other apps")
	}
}

func TestActiveAudioJobsReadOnlyBoardsThatAreOn(t *testing.T) {
	on := Device{Enabled: true, Profiles: []DeviceProfile{{Jobs: [][]Job{
		{{Kind: JobApp, Exe: "spotify.exe"}, {Kind: JobFocusedApp}},
		{{Kind: JobApp, Exe: "spotify.exe"}},
	}}}}
	off := Device{Profiles: []DeviceProfile{{Jobs: [][]Job{{{Kind: JobApp, Exe: "chrome.exe"}, {Kind: JobOtherApps}}}}}}
	apps, focused, others := ActiveAudioJobs([]Device{on, off})
	if !reflect.DeepEqual(apps, []string{"spotify.exe"}) || !focused || others {
		t.Errorf("ActiveAudioJobs() = %v, %v, %v, want [spotify.exe], true, false", apps, focused, others)
	}
}
