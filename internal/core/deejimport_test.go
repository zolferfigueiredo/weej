package core

import (
	"reflect"
	"testing"
)

const deejFixture = `
slider_mapping:
  0: master
  3: monitor 1 (brightness)
  2: monitor 2 (brightness)
  4:
    - cs2.exe
    - rocketleague.exe
    - RocketLeague.exe
    - roblox.exe
    - steam.exe
    - steamwebhelper.exe
    - steamservice.exe
    - vlc.exe
    - deej.unmapped
  1:
    - chrome.exe
    - brave.exe
    - firefox.exe
    - opera.exe
    - edge.exe
    - msedge.exe
    - spotify.exe
  6: FxSound.exe
invert_sliders: true
com_port: COM6
baud_rate: 9600
noise_reduction: default
`

func TestImportDeej(t *testing.T) {
	imp, err := ImportDeej([]byte(deejFixture))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(imp.Columns, []int{0, 3, 2, 4, 1, 6}) {
		t.Fatalf("Columns = %v, want [0 3 2 4 1 6] (file order)", imp.Columns)
	}
	if imp.Invert {
		t.Error("Invert = true, want false (invert_sliders: true negates to false)")
	}
	if len(imp.Skipped) != 0 {
		t.Errorf("Skipped = %v, want none", imp.Skipped)
	}
	if len(imp.Jobs) != 6 {
		t.Fatalf("len(Jobs) = %d, want 6", len(imp.Jobs))
	}

	// Knob A: master.
	if !jobSlicesEqual(imp.Jobs[0], []Job{{Kind: JobMaster}}) {
		t.Errorf("knob A = %v, want [master]", imp.Jobs[0])
	}

	// Knob B: brightness screen 0 ("monitor 1"), listed second in the file.
	if !jobSlicesEqual(imp.Jobs[1], []Job{{Kind: JobBrightness, Screen: 0}}) {
		t.Errorf("knob B = %v, want brightness screen 0", imp.Jobs[1])
	}
	// Knob C: brightness screen 1 ("monitor 2").
	if !jobSlicesEqual(imp.Jobs[2], []Job{{Kind: JobBrightness, Screen: 1}}) {
		t.Errorf("knob C = %v, want brightness screen 1", imp.Jobs[2])
	}

	// Knob D: the games, deduplicated, plus otherApps.
	wantD := []Job{
		{Kind: JobApp, Exe: "cs2.exe"}, {Kind: JobApp, Exe: "rocketleague.exe"}, {Kind: JobApp, Exe: "roblox.exe"},
		{Kind: JobApp, Exe: "steam.exe"}, {Kind: JobApp, Exe: "steamwebhelper.exe"}, {Kind: JobApp, Exe: "steamservice.exe"},
		{Kind: JobApp, Exe: "vlc.exe"}, {Kind: JobOtherApps},
	}
	if !jobSlicesEqual(imp.Jobs[3], wantD) {
		t.Errorf("knob D = %v, want %v", imp.Jobs[3], wantD)
	}

	// Knob E: the browsers and Spotify.
	wantE := []Job{
		{Kind: JobApp, Exe: "chrome.exe"}, {Kind: JobApp, Exe: "brave.exe"}, {Kind: JobApp, Exe: "firefox.exe"},
		{Kind: JobApp, Exe: "opera.exe"}, {Kind: JobApp, Exe: "edge.exe"}, {Kind: JobApp, Exe: "msedge.exe"},
		{Kind: JobApp, Exe: "spotify.exe"},
	}
	if !jobSlicesEqual(imp.Jobs[4], wantE) {
		t.Errorf("knob E = %v, want %v", imp.Jobs[4], wantE)
	}

	// Knob F: fxsound.exe.
	if !jobSlicesEqual(imp.Jobs[5], []Job{{Kind: JobApp, Exe: "fxsound.exe"}}) {
		t.Errorf("knob F = %v, want fxsound.exe", imp.Jobs[5])
	}
}

func TestImportDeejSkipsUnknownEntries(t *testing.T) {
	yaml := `
slider_mapping:
  0:
    - master
    - Headset Microphone (Realtek)
invert_sliders: false
`
	imp, err := ImportDeej([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if !imp.Invert {
		t.Error("Invert = false, want true (invert_sliders: false negates to true)")
	}
	if !reflect.DeepEqual(imp.Skipped, []string{"Headset Microphone (Realtek)"}) {
		t.Errorf("Skipped = %v, want the device name", imp.Skipped)
	}
}
