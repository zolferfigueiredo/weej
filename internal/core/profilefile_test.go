package core

import (
	"reflect"
	"testing"
)

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

	got, ok := DecodeProfileFile(data, d)
	if !ok || got.Name != "Games" || got.Shortcut != nil || !reflect.DeepEqual(got.Jobs, p.Jobs) || !reflect.DeepEqual(got.Buttons, p.Buttons) {
		t.Fatalf("same board = %+v, want the profile back without its shortcut", got)
	}

	// Another type keys its buttons its own way, so they stay behind; jobs go by control.
	smc := NewDevice("d2", "SMC", DeviceSMC, 0, 0, 0, "Default")
	got, ok = DecodeProfileFile(data, smc)
	if !ok || len(got.Jobs) != len(smc.Controls) || got.Jobs[0][0].Kind != JobMaster || !reflect.DeepEqual(got.Buttons, DefaultMixerButtons()) {
		t.Errorf("an SMC-Mixer = %+v", got)
	}

	if _, ok := DecodeProfileFile([]byte("slider_mapping:\n  0: master\n"), d); ok {
		t.Error("a deej config read as a WeeJ profile")
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
