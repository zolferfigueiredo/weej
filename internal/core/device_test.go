package core

import (
	"bytes"
	"reflect"
	"slices"
	"testing"
)

func TestNewDeviceLaysOutItsControls(t *testing.T) {
	d := NewDevice("d1", "Desk", DeviceDIY, 2, 1, 2, "Default")
	kinds := []ControlKind{KindKnob, KindKnob, KindFader, KindButton, KindButton}
	for i, c := range d.Controls {
		if c.Kind != kinds[i] || c.Input != -1 || c.Min != 0 || c.Max != 1023 {
			t.Errorf("control %d = %+v, want a %s not found yet", i, c, kinds[i])
		}
	}
	if !reflect.DeepEqual(d.Layout, [][]int{{0, 1}, {2}, {3, 4}}) {
		t.Errorf("layout = %v, want knobs, faders, buttons on rows of their own", d.Layout)
	}
	if len(d.Profiles) != 1 || d.Profiles[0].Name != "Default" || len(d.Profiles[0].Jobs) != 5 || d.Calibrated() {
		t.Errorf("new board = %+v", d)
	}
	if !reflect.DeepEqual(d.ButtonKeys(), []int{3, 4}) {
		t.Errorf("a DIY board's buttons go by control index, got %v", d.ButtonKeys())
	}

	smc := NewDevice("d2", "SMC", DeviceSMC, 9, 9, 9, "Default")
	if len(smc.Controls) != 16 || smc.Layout != nil || !smc.Calibrated() || smc.Controls[0].Input != 40 || smc.Controls[8].Kind != KindKnob {
		t.Errorf("an SMC-Mixer has its fixed controls, got %+v", smc.Controls)
	}
	if smc.Profiles[0].Buttons[MixerNoteButton(16)] == nil || len(smc.ButtonKeys()) != len(SMCButtonOrder()) {
		t.Error("an SMC-Mixer's profile starts with its usual buttons")
	}
}

func TestNormalizeScalesEachControlToItsTravel(t *testing.T) {
	d := Device{Controls: []Control{
		{Kind: KindKnob, Input: 2, Min: 100, Max: 900},
		{Kind: KindFader, Input: 0, Reverse: true, Max: 1023},
		{Kind: KindButton, Input: 1, Max: 1023},
		{Kind: KindKnob, Input: -1, Max: 1023},
		{Kind: KindKnob, Input: 3, Max: 1023},
	}}
	got := d.Normalize([]int{200, 1023, 500, -1})
	if got[0] != 511 || got[2] != -1 || got[3] != -1 || got[4] != -1 {
		t.Errorf("Normalize = %v, want the middle of 100..900, and -1 for a button, a lost control and a silent input", got)
	}
	if want := 1023 - (200-20)*1023/(1003-20); got[1] != want {
		t.Errorf("a reversed fader at 200 reads %d, want %d", got[1], want)
	}
	if ends := d.Normalize([]int{1015, 0, 110, 0}); ends[0] != 0 || ends[1] != 0 {
		t.Errorf("near an end is that end, got %v", ends)
	}
}

type recordedApply struct {
	job Job
	u   float64
}

type recordApplier struct{ calls []recordedApply }

func (r *recordApplier) Apply(job Job, u float64) { r.calls = append(r.calls, recordedApply{job, u}) }
func (r *recordApplier) HUD(Job, float64)         {}

func TestEngineSetupFeedsTheEngineByControl(t *testing.T) {
	master := Job{Kind: JobMaster}
	mic := Job{Kind: JobMicrophone}
	d := NewDevice("d1", "Desk", DeviceDIY, 2, 0, 1, "Default")
	d.Controls[0].Input, d.Controls[1].Input, d.Controls[2].Input = 4, 0, 1
	d.Profiles[0].Jobs = [][]Job{{master}, {mic}, {Job{Kind: JobSystemSounds}}}
	rec := &recordApplier{}
	e := NewEngine(rec)
	e.Handle(d.Normalize([]int{0, 1023, 0, 0, 1023}), d.EngineSetup(), false)
	want := map[JobKind]float64{JobMaster: 1, JobMicrophone: 0}
	if len(rec.calls) != 2 {
		t.Fatalf("applied %v, want the two pots and never the button", rec.calls)
	}
	for _, c := range rec.calls {
		if want[c.job.Kind] != c.u {
			t.Errorf("%s got %v, want %v", c.job.Kind, c.u, want[c.job.Kind])
		}
	}
}

func boardFixture() Settings {
	s := DefaultSettings()
	diy := NewDevice("d1", "Desk", DeviceDIY, 2, 1, 1, "Default")
	diy.Port, diy.Baud, diy.Speed = "COM6", 115200, SpeedFast
	diy.Controls[0] = Control{Kind: KindKnob, Input: 3, Reverse: true, Min: 12, Max: 1000}
	diy.Controls[1].Input, diy.Controls[2].Input, diy.Controls[3].Input = 0, 1, 2
	diy.Profiles[0].Jobs[0] = []Job{{Kind: JobMaster}}
	diy.Profiles[0].Buttons = ButtonMap{3: {ActionPlayPause}}
	diy.Profiles = append(diy.Profiles, diy.NewProfile("Games"))
	diy.Active = 1
	diy.Next = &Shortcut{VK: 0x71, Mods: 2, Key: "F2"}
	diy.View = ViewList
	smc := NewDevice("d3", "SMC", DeviceSMC, 0, 0, 0, "Default")
	smc.Port, smc.Lights = "SMC-Mixer", "wave"
	smc.Profiles[0].Shortcut = &Shortcut{VK: 0x70, Mods: 2, Key: "F1"}
	s.Devices = []Device{diy, smc}
	s.Added = 3
	s.Language, s.ShowProfiles = "it", false
	return s
}

func TestSettingsRoundTrip(t *testing.T) {
	s := boardFixture()
	data, err := EncodeSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	back := DecodeSettings(data, "Default")
	if !reflect.DeepEqual(back, s) {
		t.Errorf("round trip changed the settings:\n got %+v\nwant %+v", back, s)
	}
	again, _ := EncodeSettings(back)
	if !bytes.Equal(again, data) {
		t.Error("decoding twice changes the file")
	}
}

func TestSettingsStartFreshFromAnOldFile(t *testing.T) {
	old := []byte(`{"columns":[0,1],"profiles":[{"name":"Default","jobs":[[],[]]}],"port":"COM6"}`)
	if s := DecodeSettings(old, "Default"); !reflect.DeepEqual(s, DefaultSettings()) {
		t.Errorf("a file from before boards gave %+v, want the defaults", s)
	}
	if s := DecodeSettings([]byte("not json"), "Default"); len(s.Devices) != 0 {
		t.Error("a broken file gave boards")
	}
}

func TestSettingsDecodeTolerantly(t *testing.T) {
	data := []byte(`{"version":2,"added":1,"devices":[
		{"id":"d1","type":"diy","controls":[{"kind":"fader","input":2,"min":900,"max":100},{"kind":"bogus","input":null}],
		 "layout":[[5,1],[0]],"profiles":[{"name":"A","jobs":[[{"kind":"master"}],null,[{"kind":"microphone"}]]}],"profile":7},
		{"id":"d1","type":"toaster"},
		{"id":"d1","type":"smc","controls":[],"lights":"disco","profiles":[]},
		{"type":"midi"}]}`)
	s := DecodeSettings(data, "Default")
	if len(s.Devices) != 3 {
		t.Fatalf("got %d boards, want 3: an unknown type is dropped", len(s.Devices))
	}
	diy, smc, midi := s.Devices[0], s.Devices[1], s.Devices[2]
	if diy.Controls[0].Min != 0 || diy.Controls[0].Max != 1023 || diy.Controls[1].Kind != KindKnob || diy.Controls[1].Input != -1 {
		t.Errorf("controls = %+v, want a bad range reset and an unknown kind read as a knob", diy.Controls)
	}
	if !reflect.DeepEqual(diy.Layout, [][]int{{1}, {0}}) || diy.Active != 0 || len(diy.Profiles[0].Jobs) != 2 {
		t.Errorf("diy = %+v, want the layout cleaned, the profile clamped and jobs trimmed to the controls", diy)
	}
	if len(smc.Controls) != 16 || smc.Lights != "" || len(smc.Profiles) != 1 || smc.ID == "d1" {
		t.Errorf("smc = %+v, want its fixed controls, no unknown pattern, a profile and an ID of its own", smc)
	}
	ids := []string{diy.ID, smc.ID, midi.ID}
	if len(slices.Compact(slices.Sorted(slices.Values(ids)))) != 3 || s.Added != 3 {
		t.Errorf("ids = %v, added %d, want three different IDs and the count of every board added", ids, s.Added)
	}
}
