package core

import (
	"math"
	"slices"
	"testing"
	"time"
)

func TestLightFramesLightOnlyTheStripButtons(t *testing.T) {
	all := SMCStripButtons()
	for _, p := range LightPatterns {
		for step := 0; step < 40; step++ {
			frame := LightFrame(p, float64(step)*0.07)
			seen := map[int]bool{}
			for _, id := range frame {
				if !slices.Contains(all, id) || seen[id] {
					t.Fatalf("%s at step %d lights %d, a stranger or twice", p, step, id)
				}
				seen[id] = true
			}
		}
	}
	if n := len(LightFrame("on", 3)); n != len(all) {
		t.Errorf("on lights %d buttons, want all %d", n, len(all))
	}
	if LightFrame("", 1) != nil || LightFrame("off", 1) != nil {
		t.Error("off lights something")
	}
}

func TestChaseAndBounceLightOneColumn(t *testing.T) {
	column := func(frame []int) int {
		if len(frame) != 4 {
			t.Fatalf("frame %v, want one column of four", frame)
		}
		return (frame[0] - 128) % 8
	}
	if a, b := column(LightFrame("chase", 0.01)), column(LightFrame("chase", 0.13)); a != 0 || b != 1 {
		t.Errorf("chase columns %d then %d, want 0 then 1", a, b)
	}
	if c := column(LightFrame("chase", 0.12*8+0.01)); c != 0 {
		t.Errorf("chase after eight steps is on column %d, want back on 0", c)
	}
	if a, b := column(LightFrame("bounce", 0.09*7+0.01)), column(LightFrame("bounce", 0.09*8+0.01)); a != 7 || b != 6 {
		t.Errorf("bounce columns %d then %d, want 7 then back to 6", a, b)
	}
}

func TestParseLightPattern(t *testing.T) {
	for in, want := range map[string]string{"wave": "wave", "off": "", "": "", "disco": ""} {
		if got := ParseLightPattern(in); got != want {
			t.Errorf("ParseLightPattern(%q) = %q, want %q", in, got, want)
		}
	}
	if NextLightPattern("on") != "random" || NextLightPattern("wave") != "sparkle" || NextLightPattern("clock") != "" {
		t.Error("Next doesn't step through the patterns and back to off")
	}
	if Animated("on") || Animated("") || !Animated("sparkle") {
		t.Error("only patterns that change over time are animated")
	}
}

func TestLightGuardDropsTheLightsDriftButNotAHand(t *testing.T) {
	bend := func(strip, msb int) uint32 { return uint32(0xE0|strip) | uint32(msb)<<16 }
	var g LightGuard
	if !g.Pass(bend(1, 123), 0, -10) {
		t.Fatal("a fader's first reading was dropped")
	}
	// Seen on a real unit: every light comes on, and fader 2 reads 127 for a while.
	if g.Pass(bend(1, 127), 10.005, 10) || g.Pass(bend(1, 123), 10.4, 10) {
		t.Error("the lights' drift went through")
	}
	if !g.Pass(bend(1, 126), 11, 10) {
		t.Error("a small move long after the lights changed was dropped")
	}
	g.Pass(bend(1, 123), 11.1, 10)
	if !g.Pass(bend(1, 110), 20.01, 20) || !g.Pass(bend(1, 109), 20.05, 20) {
		t.Error("a hand's move while the lights changed was dropped")
	}
	if !g.Pass(cc(16, 1), 20.05, 20) {
		t.Error("a knob step was taken for a pot")
	}
}

func TestRandomChangesPatternEverySixSeconds(t *testing.T) {
	last := ""
	for n := range 50 {
		p := RandomPattern(float64(n)*randomEvery + 1)
		if p == last || !Animated(p) || p == "eq" || p == "clock" || p == "random" {
			t.Fatalf("random's pattern %d is %q after %q", n, p, last)
		}
		if RandomPattern(float64(n)*randomEvery+5.9) != p {
			t.Fatalf("random changed its pattern %d before six seconds", n)
		}
		last = p
	}
}

func TestEQFrameFillsEachColumnToItsBand(t *testing.T) {
	frame := EQFrame([8]float64{0, 1, 0.5, 0, 0, 0, 0, 0.05})
	want := []int{MixerNoteButton(16 + 1), MixerNoteButton(8 + 1), MixerNoteButton(0 + 1), MixerNoteButton(0 + 2), MixerNoteButton(24 + 1), MixerNoteButton(24 + 2)}
	slices.Sort(frame)
	slices.Sort(want)
	if !slices.Equal(frame, want) {
		t.Errorf("EQFrame = %v, want %v", frame, want)
	}
}

func TestClockFrameShowsTheTimeInBinary(t *testing.T) {
	// 12:34:56: the hours' 1 lights the bottom of column 1, the 2 the row above in column 2, and so on.
	frame := ClockFrame(time.Date(2026, 1, 1, 12, 34, 56, 0, time.Local))
	lit := func(row, col int) bool { return slices.Contains(frame, MixerNoteButton(lightRows[row]+col)) }
	if !lit(3, 1) || lit(2, 1) || !lit(2, 2) || lit(3, 2) || !lit(2, 6) || !lit(1, 6) || lit(3, 6) {
		t.Errorf("12:34:56 drawn as %v", frame)
	}
}

func TestSpectrumJumpsTheBandThatGetsLouder(t *testing.T) {
	s := NewSpectrum()
	tone := func(hz, amp float64) []float32 {
		out := make([]float32, 1920)
		for i := range out {
			out[i] = float32(amp * math.Sin(2*math.Pi*hz*float64(i)/48000))
		}
		return out
	}
	// Two quiet tones for a while, a bass and a high one, then the high one jumps.
	now := 0.0
	for range 100 {
		s.Add(tone(60, 0.05))
		s.Add(tone(3000, 0.02))
		now += 0.04
		s.Bands(now)
	}
	loud := tone(3000, 0.5)
	for i := range loud {
		loud[i] += tone(60, 0.05)[i]
	}
	s.Add(loud)
	s.Add(loud)
	now += 0.04
	bands := s.Bands(now)
	if bands[5] < 0.9 || bands[0] > 0.6 {
		t.Errorf("the high tone jumping gives bands %.2f, want column 6 full and the bass calm", bands)
	}
	s.Add(make([]float32, fftSize))
	if bands := s.Bands(now + 2); bands != [8]float64{} {
		t.Errorf("silence two seconds later gives %.2f, want nothing", bands)
	}
}

func TestPreviousLightPattern(t *testing.T) {
	if PreviousLightPattern("") != "clock" || PreviousLightPattern("on") != "" || PreviousLightPattern("random") != "on" {
		t.Error("Previous doesn't step back through the patterns")
	}
}

func TestMixerStateKeepsTheFaderPitch(t *testing.T) {
	m := NewMixerState()
	if _, _, ok := m.Pitch(2); ok {
		t.Error("a fader that never moved has a position")
	}
	m.SetPitch(3, 0, 99)
	if _, msb, ok := m.Pitch(3); !ok || msb != 99 || m.Values()[43] != -1 {
		t.Error("a fader's remembered position is lost, or taken for a move")
	}
	m.Feed(0xE2 | 5<<8 | 70<<16)
	if lsb, msb, ok := m.Pitch(2); !ok || lsb != 5 || msb != 70 || m.LastChanged() != 42 {
		t.Errorf("pitch = %d, %d, %v, last %d, want 5, 70 on column 42", lsb, msb, ok, m.LastChanged())
	}
	m.Feed(0x90 | 16<<8 | 127<<16)
	if m.LastChanged() != -1 {
		t.Errorf("a press changed column %d", m.LastChanged())
	}
	m.Feed(0xB0 | 16<<8 | 1<<16)
	if m.LastChanged() != 30 {
		t.Errorf("knob 1's step changed column %d, want 30", m.LastChanged())
	}
}
