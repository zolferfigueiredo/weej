package core

import (
	"encoding/json"
	"testing"
)

func englishTr(key string, vars map[string]string) string {
	if n, ok := vars["n"]; ok {
		return key + ":" + n
	}
	return key
}

func TestPercentRoundsAndClamps(t *testing.T) {
	cases := []struct {
		in   float64
		want int
	}{
		{0, 0},
		{1, 100},
		{-0.5, 0},
		{1.5, 100},
		{0.355, 36},
		{0.004, 0},
	}
	for _, c := range cases {
		if got := Percent(c.in); got != c.want {
			t.Errorf("Percent(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

var testColumns = []int{0, 3, 2, 4, 1}
var testJobs = [][]Job{
	{{Kind: JobMaster}},
	{{Kind: JobBrightness, Screen: 0}},
	{{Kind: JobBrightness, Screen: 1}},
	{},
	{{Kind: JobBuiltinBrightness}},
}

func TestKnobsMapToSerialColumns(t *testing.T) {
	got := mapping(testColumns, testJobs)
	want := map[int][]Job{
		0: {{Kind: JobMaster}},
		1: {{Kind: JobBuiltinBrightness}},
		3: {{Kind: JobBrightness, Screen: 0}},
		2: {{Kind: JobBrightness, Screen: 1}},
	}
	if len(got) != len(want) {
		t.Fatalf("mapping() = %v, want %v", got, want)
	}
	for k, v := range want {
		if gv, ok := got[k]; !ok || !jobSlicesEqual(gv, v) {
			t.Errorf("mapping()[%d] = %v, want %v", k, got[k], v)
		}
	}

	single := mapping(testColumns, [][]Job{{{Kind: JobMaster}}})
	if len(single) != 1 || !jobSlicesEqual(single[0], []Job{{Kind: JobMaster}}) {
		t.Errorf("single-knob mapping = %v", single)
	}

	// One knob can do several jobs, of any kind.
	several := mapping(testColumns, [][]Job{{{Kind: JobBrightness, Screen: 0}, {Kind: JobBrightness, Screen: 1}, {Kind: JobApp, Exe: "music.exe"}}})
	want2 := []Job{{Kind: JobBrightness, Screen: 0}, {Kind: JobBrightness, Screen: 1}, {Kind: JobApp, Exe: "music.exe"}}
	if !jobSlicesEqual(several[0], want2) {
		t.Errorf("several-job mapping = %v, want %v", several[0], want2)
	}
}

func jobSlicesEqual(a, b []Job) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMenuOrder(t *testing.T) {
	s := Setup{Columns: testColumns, Profiles: []Profile{{Jobs: testJobs}}}
	got := s.MenuOrder()
	want := []int{0, 3, 2, 1}
	if len(got) != len(want) {
		t.Fatalf("MenuOrder() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("MenuOrder() = %v, want %v", got, want)
			break
		}
	}
}

func everyJobKind() []Job {
	var every []Job
	every = append(every,
		Job{Kind: JobMaster}, Job{Kind: JobMicrophone}, Job{Kind: JobSystemSounds},
		Job{Kind: JobBuiltinBrightness})
	for i := 0; i < 16; i++ {
		every = append(every, Job{Kind: JobBrightness, Screen: i}, Job{Kind: JobContrast, Screen: i})
	}
	every = append(every,
		Job{Kind: JobNightLight}, Job{Kind: JobExternalKeyboard}, Job{Kind: JobZoom},
		Job{Kind: JobFocusedApp}, Job{Kind: JobOtherApps}, Job{Kind: JobApp, Exe: "music.exe"})
	return every
}

func TestEveryJobHasItsOwnRank(t *testing.T) {
	every := everyJobKind()
	seen := map[int]bool{}
	for _, j := range every {
		r := j.Rank()
		if seen[r] {
			t.Fatalf("duplicate rank %d for job %v", r, j)
		}
		seen[r] = true
	}
	if len(seen) != len(every) {
		t.Errorf("got %d distinct ranks, want %d", len(seen), len(every))
	}
}

func TestShortTitlesAreDistinctWithinASection(t *testing.T) {
	every := everyJobKind()
	seen := map[string]bool{}
	for _, j := range every {
		key := j.Section().Key() + j.ShortTitle(englishTr, nil)
		if seen[key] {
			t.Fatalf("duplicate short title %q for job %v", key, j)
		}
		seen[key] = true
	}
}

func TestJobsSurviveSaving(t *testing.T) {
	every := everyJobKind()
	data, err := json.Marshal(every)
	if err != nil {
		t.Fatal(err)
	}
	var back []Job
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !jobSlicesEqual(every, back) {
		t.Errorf("round trip = %v, want %v", back, every)
	}
}

func TestJobJSONShape(t *testing.T) {
	cases := []struct {
		job  Job
		want string
	}{
		{Job{Kind: JobMaster}, `{"kind":"master"}`},
		{Job{Kind: JobBrightness, Screen: 0}, `{"kind":"brightness","screen":0}`},
		{Job{Kind: JobApp, Exe: "chrome.exe"}, `{"kind":"app","exe":"chrome.exe"}`},
	}
	for _, c := range cases {
		data, err := json.Marshal(c.job)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != c.want {
			t.Errorf("Marshal(%v) = %s, want %s", c.job, data, c.want)
		}
	}
}

func TestUnknownJobKindErrors(t *testing.T) {
	var j Job
	if err := json.Unmarshal([]byte(`{"kind":"doesNotExist"}`), &j); err == nil {
		t.Error("want error for unknown job kind")
	}
}

func TestLongNamesAreClipped(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  string
	}{
		{"Default", 20, "Default"},
		{"Default dasdas asd asdas asd asdas", 20, "Default dasdas asd…"},
	}
	for _, c := range cases {
		if got := Clipped(c.name, c.limit); got != c.want {
			t.Errorf("Clipped(%q, %d) = %q, want %q", c.name, c.limit, got, c.want)
		}
	}
	got := Clipped(stringsRepeat("a", 30), 20)
	if len([]rune(got)) != 20 {
		t.Errorf("Clipped length = %d, want 20", len([]rune(got)))
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

func TestSteppingThroughProfilesWrapsRound(t *testing.T) {
	s := Setup{Profiles: []Profile{{Name: "A"}, {Name: "B"}, {Name: "C"}}}
	if s.Stepped(1) != 1 || s.Stepped(-1) != 2 {
		t.Errorf("Stepped from 0: +1=%d -1=%d", s.Stepped(1), s.Stepped(-1))
	}
	s.Active = 2
	if s.Stepped(1) != 0 || s.Stepped(-1) != 1 {
		t.Errorf("Stepped from 2: +1=%d -1=%d", s.Stepped(1), s.Stepped(-1))
	}
}

func TestProfilesAndShortcutsSurviveSaving(t *testing.T) {
	games := []Profile{{
		Name:     "Games",
		Jobs:     testJobs,
		Shortcut: &Shortcut{VK: 0x31, Mods: ModControl | ModAlt, Key: "1"},
	}}
	data, err := json.Marshal(games)
	if err != nil {
		t.Fatal(err)
	}
	var back []Profile
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 || back[0].Name != "Games" || back[0].Shortcut == nil || *back[0].Shortcut != *games[0].Shortcut {
		t.Errorf("round trip = %+v, want %+v", back, games)
	}
	if got := Label(*games[0].Shortcut, "Ctrl"); got != "Ctrl+Alt+1" {
		t.Errorf("Label() = %q, want Ctrl+Alt+1", got)
	}
	f1 := Shortcut{VK: 0x70, Mods: ModControl, Key: "f1"}
	if got := Label(f1, "Ctrl"); got != "Ctrl+F1" {
		t.Errorf("Label() = %q, want Ctrl+F1", got)
	}
}

func TestKnownAudioAppsAreListedOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range KnownApps {
		if seen[a.Exe] {
			t.Errorf("duplicate known app exe %q", a.Exe)
		}
		seen[a.Exe] = true
	}
}

func TestDecodeSettingsDefaultsAndFallbacks(t *testing.T) {
	s, err := DecodeSettings(nil, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Profiles) != 1 || s.Profiles[0].Name != "Default" {
		t.Errorf("default profiles = %+v", s.Profiles)
	}
	if !s.ShowProfiles || s.Icon != IconMixer || s.Speed != SpeedSlow || s.UpdateEvery != 604800 || !s.ShowDataInMenu {
		t.Errorf("defaults not applied: %+v", s)
	}

	s2, err := DecodeSettings([]byte(`{"profile": 99, "trayIcon": "bogus", "speed": "bogus", "updateEvery": 12345}`), "Default")
	if err != nil {
		t.Fatal(err)
	}
	if s2.Active != 0 {
		t.Errorf("Active = %d, want clamped to 0", s2.Active)
	}
	if s2.Icon != IconMixer {
		t.Errorf("Icon = %v, want fallback to mixer", s2.Icon)
	}
	if s2.Speed != SpeedSlow {
		t.Errorf("Speed = %v, want fallback to slow", s2.Speed)
	}
	if s2.UpdateEvery != 604800 {
		t.Errorf("UpdateEvery = %d, want fallback to 604800", s2.UpdateEvery)
	}
}

func TestEncodeSettingsColumnsNull(t *testing.T) {
	s := DefaultSettings("Default")
	s.Columns = []int{0, -1, 2}
	data, err := EncodeSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	cols, ok := raw["columns"].([]interface{})
	if !ok || len(cols) != 3 || cols[0] != float64(0) || cols[1] != nil || cols[2] != float64(2) {
		t.Errorf("columns = %v, want [0, null, 2]", raw["columns"])
	}
}

func TestSettingsSurviveRoundTrip(t *testing.T) {
	s := DefaultSettings("Default")
	s.Columns = []int{0, -1, 2}
	s.Profiles = []Profile{{Name: "Desk", Jobs: [][]Job{{{Kind: JobMaster}}, {}, {{Kind: JobBrightness, Screen: 0}}}}}
	s.Active = 0
	s.Invert = true
	s.Language = "en"

	data, err := EncodeSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeSettings(data, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if back.Invert != s.Invert || back.Language != s.Language || len(back.Profiles) != 1 || back.Profiles[0].Name != "Desk" {
		t.Errorf("round trip = %+v, want %+v", back, s)
	}
	for i, c := range s.Columns {
		if back.Columns[i] != c {
			t.Errorf("Columns[%d] = %d, want %d", i, back.Columns[i], c)
		}
	}
}
