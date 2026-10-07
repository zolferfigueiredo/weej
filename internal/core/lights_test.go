package core

import (
	"slices"
	"testing"
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
	if NextLightPattern("") != "on" || NextLightPattern("wave") != "sparkle" || NextLightPattern("blink") != "" {
		t.Error("Next doesn't step through the patterns and back to off")
	}
	if Animated("on") || Animated("") || !Animated("sparkle") {
		t.Error("only patterns that change over time are animated")
	}
}
