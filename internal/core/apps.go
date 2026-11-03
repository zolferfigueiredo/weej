package core

import (
	"strconv"
	"strings"
)

// Not filepath.Base: this runs on Linux in tests too, where backslash is not a separator, but
// the paths it must split come from Windows.
func ExeName(path string) string {
	normalized := strings.ReplaceAll(path, "\\", "/")
	if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
		normalized = normalized[idx+1:]
	}
	return strings.ToLower(normalized)
}

// TrimExePath is the exe a registry value names, without the quotes around it or the icon index a
// DisplayIcon adds after a comma.
func TrimExePath(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		if end := strings.Index(s[1:], `"`); end >= 0 {
			return s[1 : end+1]
		}
		return strings.TrimPrefix(s, `"`)
	}
	if i := strings.LastIndex(s, ","); i >= 0 {
		if _, err := strconv.Atoi(strings.TrimSpace(s[i+1:])); err == nil {
			s = strings.TrimSpace(s[:i])
		}
	}
	return s
}

// SteamLibraries is the folders Steam keeps games in, the "path" entries of its
// steamapps\libraryfolders.vdf.
func SteamLibraries(vdf string) []string {
	var out []string
	for _, line := range strings.Split(vdf, "\n") {
		// `"path"  "D:\\Games"` splits on its quotes into "", key, gap, value, "".
		parts := strings.Split(strings.TrimSpace(line), `"`)
		if len(parts) == 5 && strings.EqualFold(parts[1], "path") && parts[3] != "" {
			out = append(out, strings.ReplaceAll(parts[3], `\\`, `\`))
		}
	}
	return out
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

// AppVolumes is where the knobs last set each app's volume and Other apps', so a session that
// opens later gets its level. Exes are lowercase.
type AppVolumes struct {
	apps       map[string]float64
	others     float64
	haveOthers bool
	// mapped is the apps a knob of the profiles in use names, which Other apps leaves alone even
	// before their knob turns.
	mapped map[string]bool
}

func (v *AppVolumes) Set(exe string, s float64) {
	if v.apps == nil {
		v.apps = map[string]float64{}
	}
	v.apps[strings.ToLower(exe)] = s
}

func (v *AppVolumes) SetOthers(mapped []string, s float64) {
	v.others, v.haveOthers = s, true
	v.mapped = map[string]bool{}
	for _, exe := range mapped {
		v.mapped[strings.ToLower(exe)] = true
	}
}

// Keep forgets the apps no profile in use names any more, but for the ones Focused app turned while
// a profile has it, and Other apps' level once none has it.
func (v *AppVolumes) Keep(apps []string, focused, others bool) {
	named := map[string]bool{}
	for _, exe := range apps {
		named[strings.ToLower(exe)] = true
	}
	v.mapped = named
	if !focused {
		for exe := range v.apps {
			if !named[exe] {
				delete(v.apps, exe)
			}
		}
	}
	if !others {
		v.haveOthers = false
	}
}

// IsOther tells whether Other apps turns exe: an app with no volume of its own, neither a knob's
// nor one Focused app gave it, and never WeeJ.
func (v *AppVolumes) IsOther(exe, self string) bool {
	exe = strings.ToLower(exe)
	_, own := v.apps[exe]
	return exe != strings.ToLower(self) && !own && !v.mapped[exe]
}

// Level is the volume a session of exe should have: its own, or Other apps' when it is one of them.
func (v *AppVolumes) Level(exe, self string) (float64, bool) {
	if s, ok := v.apps[strings.ToLower(exe)]; ok {
		return s, true
	}
	if v.haveOthers && v.IsOther(exe, self) {
		return v.others, true
	}
	return 0, false
}

func (v *AppVolumes) Others() (float64, bool) { return v.others, v.haveOthers }

// ActiveAudioJobs is what the profiles in use do with apps: the apps a knob names, and whether one
// has Focused app or Other apps.
func ActiveAudioJobs(devices []Device) (apps []string, focused, others bool) {
	seen := map[string]bool{}
	for _, d := range devices {
		if !d.Enabled {
			continue
		}
		for _, row := range d.ActiveProfile().Jobs {
			for _, j := range row {
				switch j.Kind {
				case JobApp:
					if !seen[j.Exe] {
						seen[j.Exe] = true
						apps = append(apps, j.Exe)
					}
				case JobFocusedApp:
					focused = true
				case JobOtherApps:
					others = true
				}
			}
		}
	}
	return apps, focused, others
}
