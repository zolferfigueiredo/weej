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
	if NextLightPattern("") != "on" || NextLightPattern("wave") != "sparkle" || NextLightPattern("clock") != "" {
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

func TestSpectrumFindsATone(t *testing.T) {
	s := NewSpectrum()
	tone := make([]float32, 4096)
	for i := range tone {
		tone[i] = float32(0.5 * math.Sin(2*math.Pi*3000*float64(i)/48000))
	}
	s.Add(tone)
	bands := s.Bands(1)
	for b, v := range bands {
		if b == 5 && v < 0.9 || b != 5 && b != 4 && b != 6 && v > 0.5 {
			t.Errorf("a 3 kHz tone gives bands %.2f", bands)
			break
		}
	}
	s.Add(make([]float32, 4096))
	if bands := s.Bands(3); bands != [8]float64{} {
		t.Errorf("silence two seconds later gives %.2f, want nothing", bands)
	}
}
