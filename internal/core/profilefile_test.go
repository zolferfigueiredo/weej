package core

import (
	"reflect"
	"strings"
	"testing"
)

func testTr(key string, _ map[string]string) string { return key }

func TestProfileFileRoundTrip(t *testing.T) {
	d := NewDevice("d1", "Desk", DeviceDIY, 2, 0, 1, "Default")
	p := d.NewProfile("Games")
	p.Jobs[0] = []Job{{Kind: JobMaster}}
	p.Jobs[1] = []Job{{Kind: JobApp, Exe: "discord.exe"}}
	p.Buttons = ButtonMap{2: {ActionPlayPause}}
	p.Shortcut = &Shortcut{VK: 0x70, Mods: 2, Key: "F1"}
	data, err := EncodeProfileFile(d.Type, p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `"platform": "windows"`) {
		t.Errorf("export = %s, want it to say it was made on Windows", data)
	}

	got, skipped, dropped, ok := DecodeProfileFile(data, d, testTr)
	if skipped != nil || dropped != nil {
		t.Errorf("skipped %v and dropped %v from WeeJ's own file", skipped, dropped)
	}
	if !ok || got.Name != "Games" || got.Shortcut != nil || !reflect.DeepEqual(got.Jobs, p.Jobs) || !reflect.DeepEqual(got.Buttons, p.Buttons) {
		t.Fatalf("same board = %+v, want the profile back without its shortcut", got)
	}

	// Another type keys its buttons its own way, so they stay behind; jobs go by control.
	smc := NewDevice("d2", "SMC", DeviceSMC, 0, 0, 0, "Default")
	got, _, _, ok = DecodeProfileFile(data, smc, testTr)
	if !ok || len(got.Jobs) != len(smc.Controls) || got.Jobs[0][0].Kind != JobMaster || !reflect.DeepEqual(got.Buttons, DefaultMixerButtons()) {
		t.Errorf("an SMC-Mixer = %+v", got)
	}

	if _, _, _, ok := DecodeProfileFile([]byte("slider_mapping:\n  0: master\n"), d, testTr); ok {
		t.Error("a deej config read as a WeeJ profile")
	}
}

// TheeJ's export: its apps and keys are a Mac's, and its own jobs go under "theej".
const macProfileFile = `{
  "kind": "weej.profile", "version": 1, "type": "diy", "platform": "macos",
  "profile": {
    "name": "Mac",
    "jobs": [[{"kind": "master"}], [], []],
    "buttons": {"2": ["open:com.spotify.client", "media.next", "keys:1048576:0:a"]}
  },
  "theej": {"jobs": [[{"master": {}}, {"app": {"_0": "com.spotify.client"}}], [{"builtinKeyboard": {}}], []]}
}`

func TestProfileFileFromAMacLeavesItsAppsAndKeys(t *testing.T) {
	d := NewDevice("d1", "Desk", DeviceDIY, 2, 0, 1, "Default")
	p, skipped, dropped, ok := DecodeProfileFile([]byte(macProfileFile), d, testTr)
	if !ok || p.Name != "Mac" || !reflect.DeepEqual(p.Jobs, [][]Job{{{Kind: JobMaster}}, {}, {}}) {
		t.Fatalf("profile = %+v, want the shared jobs", p)
	}
	if !reflect.DeepEqual(p.Buttons, ButtonMap{2: {ActionNextTrack}}) {
		t.Errorf("buttons = %v, want only the media key", p.Buttons)
	}
	if !reflect.DeepEqual(skipped, []string{"com.spotify.client", "job.builtin_keyboard"}) {
		t.Errorf("skipped = %v, want the Mac's app and its own job", skipped)
	}
	if !reflect.DeepEqual(dropped, ButtonMap{2: {"open:com.spotify.client", "keys:1048576:0:a"}}) {
		t.Errorf("dropped = %v, want the app and the keys", dropped)
	}

	smc := NewDevice("d2", "SMC", DeviceSMC, 0, 0, 0, "Default")
	if _, _, dropped, _ := DecodeProfileFile([]byte(macProfileFile), smc, testTr); dropped != nil {
		t.Errorf("dropped = %v from another type, whose buttons stay behind anyway", dropped)
	}
}

func TestDeejProfileFollowsEachSlidersInput(t *testing.T) {
	d := NewDevice("d1", "Desk", DeviceDIY, 3, 0, 0, "Default")
	d.Controls[0].Input, d.Controls[1].Input, d.Controls[2].Input = 2, 0, 4
	imp := Import{Name: "deej", Columns: []int{0, 2, 7}, Jobs: [][]Job{{{Kind: JobMaster}}, {{Kind: JobMicrophone}}, {{Kind: JobSystemSounds}}}, Skipped: []string{"foo"}}
	p, skipped := DeejProfile(imp, d)
	if p.Name != "deej" || p.Jobs[1][0].Kind != JobMaster || p.Jobs[0][0].Kind != JobMicrophone || len(p.Jobs[2]) != 0 {
		t.Errorf("jobs = %+v, want slider 0 on input 0's knob and slider 2 on input 2's", p.Jobs)
	}
	if !reflect.DeepEqual(skipped, []string{"foo", "slider 7"}) {
		t.Errorf("skipped = %v", skipped)
	}
}
