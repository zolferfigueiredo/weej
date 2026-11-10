package core

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDebounceKeepsTheLastValue(t *testing.T) {
	var mu sync.Mutex
	var landed []int
	d := NewDebouncer()
	for v := 1; v <= 3; v++ {
		v := v
		d.Debounce("brightness:0", 50*time.Millisecond, func() {
			mu.Lock()
			landed = append(landed, v)
			mu.Unlock()
		})
	}
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(landed, []int{3}) {
		t.Errorf("landed = %v, want [3]", landed)
	}
}

func TestSpeedsStayAboveTheWiperDropout(t *testing.T) {
	speeds := []Speed{SpeedSlow, SpeedMedium, SpeedFast, SpeedSuperFast}
	var last time.Duration = -1
	for _, s := range speeds {
		settle := s.Settle()
		if settle <= 110*time.Millisecond {
			t.Errorf("%v.Settle() = %v, want > 110ms", s, settle)
		}
		if last >= 0 && settle >= last {
			t.Errorf("%v.Settle() = %v, want less than the previous speed's %v", s, settle, last)
		}
		last = settle
	}
}

type fakeApplier struct {
	mu      sync.Mutex
	applied []Job
	huds    []Job
	// displays overrides where a kind of job shows its HUD.
	displays map[JobKind]string
	levels   map[JobKind]float64
}

func (f *fakeApplier) Level(job Job) (float64, bool) {
	s, ok := f.levels[job.Kind]
	return s, ok
}

func (f *fakeApplier) Display(job Job) string {
	if d, ok := f.displays[job.Kind]; ok {
		return d
	}
	switch job.Kind {
	case JobBrightness, JobContrast:
		return "screen:" + strconv.Itoa(job.Screen)
	default:
		return "primary"
	}
}

func (f *fakeApplier) Apply(job Job, s float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, job)
}

func (f *fakeApplier) HUD(job Job, s float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.huds = append(f.huds, job)
}

func TestEngineImmediateJobsApplyAtOnce(t *testing.T) {
	a := &fakeApplier{}
	e := NewEngine(a)
	setup := Setup{
		Columns:  []int{0},
		Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}}}},
		Speed:    SpeedSuperFast,
	}
	e.Handle([]int{1023}, setup, false)
	e.Handle([]int{0}, setup, false)
	e.Handle([]int{1023}, setup, false)

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.applied) != 2 || a.applied[0].Kind != JobMaster {
		t.Errorf("applied = %v, want master twice (each move, not the first reading)", a.applied)
	}
	if len(a.huds) != 2 {
		t.Errorf("huds = %v, want one HUD per change", a.huds)
	}
}

func TestEngineOneHUDPerDisplayPerKnobChange(t *testing.T) {
	a := &fakeApplier{}
	e := NewEngine(a)
	setup := Setup{
		Columns: []int{0},
		Profiles: []Profile{{Jobs: [][]Job{{
			{Kind: JobBrightness, Screen: 0},
			{Kind: JobContrast, Screen: 0},
		}}}},
		Speed: SpeedSuperFast,
	}
	e.Handle([]int{1023}, setup, false)
	e.Handle([]int{0}, setup, false)
	e.Handle([]int{1023}, setup, false)

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.huds) != 2 || a.huds[0].Kind != JobBrightness || a.huds[1].Kind != JobBrightness {
		t.Errorf("huds = %v, want one per change, brightness claiming screen 0 first", a.huds)
	}
}

func TestEngineJobsOnOneMonitorShareItsHUD(t *testing.T) {
	a := &fakeApplier{displays: map[JobKind]string{JobBrightness: "primary"}}
	e := NewEngine(a)
	setup := Setup{
		Columns:  []int{0},
		Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}, {Kind: JobBrightness, Screen: 0}}}}},
		Speed:    SpeedSuperFast,
	}
	e.Handle([]int{1023}, setup, false)
	e.Handle([]int{0}, setup, false)

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.huds) != 1 || a.huds[0].Kind != JobMaster {
		t.Errorf("huds = %v, want master's alone, its screen being the primary", a.huds)
	}
}

func TestEngineJobWithNoDisplayLeavesItToTheNext(t *testing.T) {
	a := &fakeApplier{displays: map[JobKind]string{JobFocusedApp: ""}}
	e := NewEngine(a)
	setup := Setup{
		Columns:  []int{0},
		Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobFocusedApp}, {Kind: JobMaster}}}}},
		Speed:    SpeedSuperFast,
	}
	e.Handle([]int{1023}, setup, false)
	e.Handle([]int{0}, setup, false)

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.applied) != 2 {
		t.Errorf("applied = %v, want both jobs", a.applied)
	}
	if len(a.huds) != 1 || a.huds[0].Kind != JobMaster {
		t.Errorf("huds = %v, want master's alone", a.huds)
	}
}

func TestEngineTracesWhatItMakesOfEachReading(t *testing.T) {
	e := NewEngine(&fakeApplier{})
	var lines []string
	e.Trace = func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	setup := Setup{Columns: []int{0}, Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}}}}, Speed: SpeedSuperFast}
	for _, v := range []int{512, 512, 515, 700} {
		e.Handle([]int{v}, setup, false)
	}
	want := []string{"first reading", "under 1%", "applied to master"}
	if len(lines) != len(want) {
		t.Fatalf("trace = %q, want one line per reading that changed", lines)
	}
	for i, w := range want {
		if !strings.Contains(lines[i], "control 0") || !strings.Contains(lines[i], w) {
			t.Errorf("trace line %d = %q, want control 0 and %q", i, lines[i], w)
		}
	}
}

func TestEngineTakesAConnectsFirstReadingAsABaseline(t *testing.T) {
	a := &fakeApplier{}
	e := NewEngine(a)
	setup := Setup{
		Columns:  []int{0},
		Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}}}},
		Speed:    SpeedSuperFast,
	}
	e.Handle([]int{512}, setup, false)
	e.Handle([]int{600}, setup, false)
	e.Reset()
	e.Handle([]int{900}, setup, false)

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.applied) != 1 || len(a.huds) != 1 {
		t.Errorf("applied %v with HUDs %v, want only the move, not either first reading", a.applied, a.huds)
	}
}

func TestEngineNewJobWaitsForTheKnobToMove(t *testing.T) {
	a := &fakeApplier{}
	e := NewEngine(a)
	unmapped := Setup{Columns: []int{0}, Profiles: []Profile{{}}, Speed: SpeedSuperFast}
	mapped := Setup{
		Columns:  []int{0},
		Profiles: []Profile{{Jobs: [][]Job{{{Kind: JobMaster}}}}},
		Speed:    SpeedSuperFast,
	}
	e.Handle([]int{512}, unmapped, false)
	e.Handle([]int{512}, mapped, false)
	a.mu.Lock()
	if len(a.applied) != 0 {
		t.Errorf("applied = %v, want nothing until the knob moves", a.applied)
	}
	a.mu.Unlock()
	e.Handle([]int{600}, mapped, false)
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.applied) != 1 {
		t.Errorf("applied = %v, want one apply once the knob moved", a.applied)
	}
}

func TestEngineLinesFollowKnobOrder(t *testing.T) {
	a := &fakeApplier{}
	e := NewEngine(a)
	setup := Setup{
		Columns: []int{0, 3, 2, 4, 1},
		Profiles: []Profile{{Jobs: [][]Job{
			{{Kind: JobMaster}},
			{{Kind: JobBrightness, Screen: 0}},
			{{Kind: JobBrightness, Screen: 1}},
			{},
			{{Kind: JobBuiltinBrightness}},
		}}},
		Speed: SpeedSuperFast,
	}
	e.Handle([]int{1023, 0, 0, 0, 0}, setup, false)
	lines := e.Lines()
	if len(lines) != 4 {
		t.Fatalf("lines = %v, want 4 entries", lines)
	}
	want := []JobKind{JobMaster, JobBrightness, JobBrightness, JobBuiltinBrightness}
	for i, k := range want {
		if lines[i].Job.Kind != k {
			t.Errorf("lines[%d].Job.Kind = %v, want %v", i, lines[i].Job.Kind, k)
		}
	}
}
