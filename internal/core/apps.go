package core

import "strings"

// Not filepath.Base: this runs on Linux in tests too, where backslash is not a separator, but
// the paths it must split come from Windows.
func ExeName(path string) string {
	normalized := strings.ReplaceAll(path, "\\", "/")
	if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
		normalized = normalized[idx+1:]
	}
	return strings.ToLower(normalized)
}

type KnownApp struct {
	Exe     string
	Name    string
	Aliases []string
}

var KnownApps = []KnownApp{
	{Exe: "spotify.exe", Name: "Spotify"},
	{Exe: "chrome.exe", Name: "Google Chrome"},
	{Exe: "msedge.exe", Name: "Microsoft Edge"},
	{Exe: "firefox.exe", Name: "Firefox"},
	{Exe: "brave.exe", Name: "Brave"},
	{Exe: "opera.exe", Name: "Opera"},
	{Exe: "vivaldi.exe", Name: "Vivaldi"},
	{Exe: "vlc.exe", Name: "VLC"},
	{Exe: "discord.exe", Name: "Discord"},
	{Exe: "ms-teams.exe", Name: "Microsoft Teams"},
	{Exe: "zoom.exe", Name: "Zoom"},
	{Exe: "slack.exe", Name: "Slack"},
	{Exe: "whatsapp.exe", Name: "WhatsApp"},
	{Exe: "telegram.exe", Name: "Telegram"},
	{Exe: "signal.exe", Name: "Signal"},
	{Exe: "steam.exe", Name: "Steam", Aliases: []string{"steamwebhelper.exe"}},
}

func OtherApps(sessionExes, mapped []string, self string) []string {
	mappedSet := map[string]bool{}
	for _, m := range mapped {
		mappedSet[strings.ToLower(m)] = true
	}
	selfLower := strings.ToLower(self)

	var out []string
	for _, exe := range sessionExes {
		lower := strings.ToLower(exe)
		if lower == selfLower || mappedSet[lower] {
			continue
		}
		out = append(out, exe)
	}
	return out
}
