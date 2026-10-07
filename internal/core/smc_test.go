package core

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func note(n, velocity int) uint32 { return 0x90 | uint32(n)<<8 | uint32(velocity)<<16 }

func TestSMCButtonsAreTheSameInBothModes(t *testing.T) {
	m := NewMixerState()
	press := func(msg uint32) int {
		_, pressed := m.Feed(msg)
		return pressed
	}
	for i := 0; i < 8; i++ {
		if a, b := press(cc(20+i, 127)), press(note(16+i, 127)); a != b || a != MixerNoteButton(16+i) {
			t.Errorf("M%d = %d in CC mode, %d in DAW mode, want both %d", i+1, a, b, MixerNoteButton(16+i))
		}
		if a, b := press(cc(smcSCCs[i], 127)), press(note(8+i, 127)); a != b {
			t.Errorf("S%d = %d in CC mode, %d in DAW mode", i+1, a, b)
		}
	}
	for i, n := range smcBottomNotes {
		if a, b := press(cc(52+i, 127)), press(note(n, 127)); a != b {
			t.Errorf("bottom button %d = %d in CC mode, %d in DAW mode", i+1, a, b)
		}
	}
	if got := press(cc(50, 127)); got != MixerNoteButton(14) {
		t.Errorf("CC 50 = %d, want S7", got)
	}
}

func TestSMCButtonCCsRoundTrip(t *testing.T) {
	withCC := 0
	for _, id := range SMCButtonOrder() {
		c, ok := SMCButtonCC(id)
		if !ok {
			continue
		}
		withCC++
		if back, ok := SMCButtonID(c); !ok || back != id {
			t.Errorf("button %d sends CC %d, which reads back as %d", id, c, back)
		}
	}
	if withCC != 27 {
		t.Errorf("%d buttons send a CC, want M, S and the bottom row: 27", withCC)
	}
	if _, ok := SMCButtonCC(MixerNoteButton(0)); ok {
		t.Error("R1 has a CC, want none")
	}
}

func TestSMCControlsAreFixed(t *testing.T) {
	for _, name := range []string{"SMC-Mixer", "MIDIIN2 (SMC-Mixer)", "SMC-Mixer-bt", "smc-mixer"} {
		if !IsSMCName(name) {
			t.Errorf("%q not an SMC-Mixer", name)
		}
	}
	if IsSMCName("nanoKONTROL2") {
		t.Error("another mixer read as an SMC-Mixer")
	}

	s := Setup{Port: MidiPort("SMC-Mixer"), Columns: []int{0, 1}, MixerColumns: []int{31, 40}, ButtonOrder: []int{144}}
	if got := s.ForMixer().Columns; !reflect.DeepEqual(got, smcColumns) {
		t.Errorf("columns = %v, want faders then knobs whatever was calibrated", got)
	}
	if got := s.MixerButtonOrder(); !reflect.DeepEqual(got, SMCButtonOrder()) {
		t.Errorf("button order = %v, want every button", got)
	}
	s.Port = MidiPort("nanoKONTROL2")
	if got := s.ForMixer().Columns; !reflect.DeepEqual(got, []int{31, 40}) {
		t.Errorf("another mixer's columns = %v, want its calibration", got)
	}
}

func TestSMCModeFollowsTheMixer(t *testing.T) {
	var mode SMCMode
	steps := []struct {
		msg uint32
		daw bool
	}{
		{0xE3 | 64<<16, true},
		{cc(40, 64), false},
		{note(16, 127), true},
		{cc(20, 127), false},
		{cc(20, 65), true},
		{cc(52, 0), false},
		{0x80 | 16<<8, true},
	}
	for i, s := range steps {
		mode.Seen(s.msg)
		if mode.DAW() != s.daw {
			t.Errorf("step %d (%#x): DAW = %v, want %v", i, s.msg, mode.DAW(), s.daw)
		}
	}
}

func TestSMCLEDMessages(t *testing.T) {
	if msg, ok := SMCButtonLED(MixerNoteButton(16), true, true); !ok || msg != note(16, 127) {
		t.Errorf("M1 on in DAW mode = %#x, %v", msg, ok)
	}
	if msg, ok := SMCButtonLED(MixerNoteButton(16), false, false); !ok || msg != cc(20, 0) {
		t.Errorf("M1 off in CC mode = %#x, %v", msg, ok)
	}
	if _, ok := SMCButtonLED(MixerNoteButton(24), true, false); ok {
		t.Error("Square 1 lit in CC mode, where it sends nothing")
	}
	if got := SMCStripBlink(2, 10); got != 0xE2|127<<16 {
		t.Errorf("blink for a low fader = %#x, want a pitch bend to the top", got)
	}
	if got := SMCStripBlink(2, 100); got != 0xE2 {
		t.Errorf("blink for a high fader = %#x, want a pitch bend to the bottom", got)
	}
	if got := SMCStripRestore(7, 3, 99); got != 0xE7|3<<8|99<<16 {
		t.Errorf("restore = %#x, want the fader's own pitch bend", got)
	}
}

func TestStripLightsStayOnBrieflyAfterAMove(t *testing.T) {
	var l StripLights
	if !l.Move(3, 1.0) || l.Move(3, 1.25) {
		t.Fatal("want the first move to light the strip and the next to keep it lit")
	}
	if off := l.Due(1.54); off != nil || !l.Any() {
		t.Errorf("off %v at 0.29 s after the last move, want still lit", off)
	}
	if off := l.Due(1.55); !reflect.DeepEqual(off, []int{3}) || l.Any() {
		t.Errorf("off %v at 0.3 s, want strip 3 out", off)
	}
	l.Move(0, 2)
	l.Move(5, 2)
	if off := l.AllOff(); !reflect.DeepEqual(off, []int{0, 5}) || l.Any() {
		t.Errorf("AllOff = %v, want strips 0 and 5", off)
	}
}

func TestMixerStateKeepsTheFaderPitch(t *testing.T) {
	m := NewMixerState()
	if _, _, ok := m.Pitch(2); ok {
		t.Error("a fader that never moved has a position")
	}
	m.Feed(0xE2 | 5<<8 | 70<<16)
	if lsb, msb, ok := m.Pitch(2); !ok || lsb != 5 || msb != 70 || m.LastChanged() != 42 {
		t.Errorf("pitch = %d, %d, %v, last %d, want 5, 70 on column 42", lsb, msb, ok, m.LastChanged())
	}
	m.Feed(note(16, 127))
	if m.LastChanged() != -1 {
		t.Errorf("a press changed column %d", m.LastChanged())
	}
	m.Feed(cc(16, 1))
	if m.LastChanged() != 30 {
		t.Errorf("knob 1's step changed column %d, want 30", m.LastChanged())
	}
}

func TestMigrateSMCMovesCCButtonsToTheirIDs(t *testing.T) {
	s := Setup{Profiles: []Profile{{Buttons: ButtonMap{
		20:                  {ActionStop},
		MixerNoteButton(16): {ActionMuteMic, ActionStop},
		59:                  {ActionNextProfile},
		MixerNoteButton(0):  {ActionLockPC},
	}}}, ButtonOrder: []int{20, MixerNoteButton(16), 52}}
	MigrateSMC(&s)
	want := ButtonMap{
		MixerNoteButton(16): {ActionMuteMic, ActionStop},
		MixerNoteButton(96): {ActionNextProfile},
		MixerNoteButton(0):  {ActionLockPC},
	}
	if !reflect.DeepEqual(s.Profiles[0].Buttons, want) {
		t.Errorf("buttons = %v, want %v", s.Profiles[0].Buttons, want)
	}
	if !reflect.DeepEqual(s.ButtonOrder, []int{MixerNoteButton(16), MixerNoteButton(94)}) {
		t.Errorf("another mixer's order = %v, want its CCs as ids", s.ButtonOrder)
	}
}

func TestMigrateSMCKeepsJobsOnTheirControl(t *testing.T) {
	master := []Job{{Kind: JobMaster}}
	mic := []Job{{Kind: JobMicrophone}}
	s := Setup{
		Port:         MidiPort("SMC-Mixer"),
		MixerColumns: []int{31, 40, -1},
		ButtonOrder:  []int{144},
		Profiles: []Profile{{
			MixerJobs: [][]Job{master, mic, {{Kind: JobZoom}}},
			Buttons:   ButtonMap{144: {MuteAction(0), ActionStop}, 145: {MuteAction(1)}, 146: {MuteAction(2)}},
		}},
	}
	MigrateSMC(&s)
	p := s.Profiles[0]
	if len(p.MixerJobs) != 16 || !reflect.DeepEqual(p.MixerJobs[9], master) || !reflect.DeepEqual(p.MixerJobs[0], mic) {
		t.Errorf("mixer jobs = %v, want master on knob 2 (control 9) and the mic on fader 1", p.MixerJobs)
	}
	for i, row := range p.MixerJobs {
		if i != 0 && i != 9 && len(row) != 0 {
			t.Errorf("control %d = %v, want empty", i, row)
		}
	}
	want := ButtonMap{144: {MuteAction(9), ActionStop}, 145: {MuteAction(0)}}
	if !reflect.DeepEqual(p.Buttons, want) {
		t.Errorf("buttons = %v, want the mutes following their controls", p.Buttons)
	}
	if s.MixerColumns != nil || s.ButtonOrder != nil {
		t.Errorf("calibration left: columns %v, order %v", s.MixerColumns, s.ButtonOrder)
	}
	again := s
	again.Profiles = []Profile{{MixerJobs: p.MixerJobs, Buttons: ButtonMap{}}}
	for id, a := range p.Buttons {
		again.Profiles[0].Buttons[id] = slices.Clone(a)
	}
	MigrateSMC(&again)
	if !reflect.DeepEqual(again.Profiles[0].Buttons, want) || !reflect.DeepEqual(again.Profiles[0].MixerJobs, p.MixerJobs) {
		t.Error("a second run changed something")
	}
}

// Shaped like a real SMC-Mixer user's file: three profiles, the board's knobs kept for later,
// a calibration in the default order and buttons saved in DAW mode.
const smcSettingsFixture = `{
  "columns": [0, 3, 2, 4, 1],
  "profiles": [
    {"name": "Desk",
     "jobs": [[{"kind": "master"}], [], [], [], []],
     "mixerJobs": [[{"kind": "master"}], [{"kind": "brightness", "screen": 0}], [{"kind": "brightness", "screen": 1}],
       [{"kind": "app", "exe": "game.exe"}, {"kind": "otherApps"}], [{"kind": "app", "exe": "browser.exe"}],
       [{"kind": "microphone"}], [{"kind": "nightLight"}], [{"kind": "externalKeyboard"}], [{"kind": "zoom"}],
       [{"kind": "contrast", "screen": 0}], [{"kind": "contrast", "screen": 1}], [], [], [], [], []],
     "buttons": {"144": ["mute:0"], "145": ["mute:1"], "151": ["mute:7"], "174": ["profile.previous"],
       "175": ["profile.next"], "219": ["media.previous"], "222": ["media.playpause"], "227": ["settings"]}},
    {"name": "Empty", "jobs": [[], [], [], [], []], "mixerJobs": [[], [], [], [], [], [], [], [], [], [], [], [], [], [], [], []],
     "buttons": {"174": ["profile.previous"], "175": ["profile.next"]}},
    {"name": "Board", "jobs": [[{"kind": "zoom"}], [], [], [], []], "mixerJobs": [[], [], [], [], [], [], [], [], [], [], [], [], [], [], [], []],
     "buttons": {"174": ["profile.previous"], "175": ["profile.next"]}}
  ],
  "profile": 0,
  "invertKnobs": false,
  "invertMixer": false,
  "port": "midi:SMC-Mixer",
  "baudRate": 9600,
  "mixerColumns": [40, 41, 42, 43, 44, 45, 46, 47, 30, 31, 32, 33, 34, 35, 36, 37],
  "mixerButtonOrder": [144, 136, 128, 152, 145, 137, 129, 153, 146, 138, 130, 154, 147, 139, 131, 155, 148, 140, 132, 156,
    149, 141, 133, 157, 150, 142, 134, 158, 151, 143, 135, 159, 222, 221, 223, 219, 220, 174, 175, 224, 225, 226, 227],
  "language": "en"
}`

func TestSMCSettingsSurviveTheMove(t *testing.T) {
	s, _ := DecodeSettings([]byte(smcSettingsFixture), "Default")
	if s.MixerColumns != nil || s.ButtonOrder != nil {
		t.Errorf("calibration kept: %v, %v", s.MixerColumns, s.ButtonOrder)
	}
	if !reflect.DeepEqual(s.Columns, []int{0, 3, 2, 4, 1}) {
		t.Errorf("board columns = %v", s.Columns)
	}
	desk := s.Profiles[0]
	if len(desk.MixerJobs) != 16 || desk.MixerJobs[3][1].Kind != JobOtherApps || desk.MixerJobs[10][0].Screen != 1 {
		t.Errorf("desk mixer jobs moved: %v", desk.MixerJobs)
	}
	if len(desk.Buttons) != 8 || desk.Buttons[151][0] != MuteAction(7) || desk.Buttons[227][0] != ActionOpenSettings {
		t.Errorf("desk buttons = %v", desk.Buttons)
	}
	if got := s.ForMixer().Mapping()[30]; len(got) != 1 || got[0].Kind != JobZoom {
		t.Errorf("knob 1 = %v, want zoom", got)
	}

	data, err := EncodeSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	back, _ := DecodeSettings(data, "Default")
	if !reflect.DeepEqual(back.Setup, s.Setup) {
		t.Errorf("round trip changed the setup:\n%+v\n%+v", back.Setup, s.Setup)
	}
}

func TestBoardLayoutsStayDrawable(t *testing.T) {
	if got := CleanLayout(nil, 3); got != nil {
		t.Errorf("no layout = %v, want nil: one row of knobs", got)
	}
	got := CleanLayout([][]int{{2, 9, 0}, {}, {0, 4}}, 5)
	if !reflect.DeepEqual(got, [][]int{{2, 0}, {4, 1, 3}}) {
		t.Errorf("layout = %v, want bad and repeated knobs gone and the missing ones on the last row", got)
	}
}

func TestBoardButtonsOnlyPress(t *testing.T) {
	s := Setup{
		Columns:    []int{0, 1, -1},
		BoardKinds: []ControlKind{KindKnob, KindButton, KindButton},
		Profiles:   []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}, {{Kind: JobMicrophone}}, {}}}},
	}
	if m := s.Mapping(); len(m) != 1 || m[0][0].Kind != JobMaster {
		t.Errorf("mapping = %v, want only the knob: a button's old jobs never run", m)
	}
	if got := s.BoardButtonInputs(); !reflect.DeepEqual(got, map[int]int{1: 1}) {
		t.Errorf("buttons = %v, want knob 1 on input 1; knob 2 has no input yet", got)
	}
	if s.Kind(7) != KindKnob || ParseControlKind("dial") != KindKnob {
		t.Error("an unknown control should read as a knob")
	}
	if f := s.ForMixer(); f.BoardKinds != nil {
		t.Error("the mixer took the board's kinds")
	}
}

func TestButtonWatcherFindsPressesEitherWay(t *testing.T) {
	var w ButtonWatcher
	inputs := map[int]int{4: 0, 5: 1}
	press := func(a, b int, now float64) []int { return w.Pressed([]int{a, b}, inputs, now) }
	if got := press(0, 1023, 0); got != nil {
		t.Fatalf("first frame pressed %v, want it read as rest", got)
	}
	if got := press(1023, 1023, 1); !reflect.DeepEqual(got, []int{4}) {
		t.Errorf("pulled-down button pressed %v, want knob 4", got)
	}
	if got := press(0, 1023, 1.01); got != nil {
		t.Errorf("release pressed %v", got)
	}
	if got := press(1023, 1023, 1.02); got != nil {
		t.Errorf("a bounce right after pressed %v, want nothing", got)
	}
	press(0, 1023, 1.03)
	if got := press(1023, 0, 2); !reflect.DeepEqual(got, []int{4, 5}) {
		t.Errorf("pressed %v, want both, the pulled-up one going to 0", got)
	}
	if got := press(1023-150, 150, 2.1); got != nil {
		t.Errorf("still held pressed %v", got)
	}
	w.Reset()
	if got := press(1023, 0, 3); got != nil {
		t.Errorf("after Reset pressed %v, want the new rest learned", got)
	}
}

func TestBoardSettingsSurviveSaving(t *testing.T) {
	s := DefaultSettings("Default")
	s.Columns = []int{0, 2, 1}
	s.BoardKinds = []ControlKind{KindKnob, KindFader, KindButton}
	s.BoardLayout = [][]int{{1, 0}, {2}}
	s.Profiles[0].BoardButtons = ButtonMap{2: {MuteAction(1), ActionPlayPause}}
	data, err := EncodeSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	back, _ := DecodeSettings(data, "Default")
	if !reflect.DeepEqual(back.BoardKinds, s.BoardKinds) || !reflect.DeepEqual(back.BoardLayout, s.BoardLayout) {
		t.Errorf("kinds %v layout %v, want %v and %v", back.BoardKinds, back.BoardLayout, s.BoardKinds, s.BoardLayout)
	}
	if !reflect.DeepEqual(back.Profiles[0].BoardButtons, s.Profiles[0].BoardButtons) {
		t.Errorf("board buttons = %v", back.Profiles[0].BoardButtons)
	}
	plain, _ := EncodeSettings(DefaultSettings("Default"))
	if strings.Contains(string(plain), "board") {
		t.Error("a setup without a board layout saved one")
	}
}
