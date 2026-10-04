package core

import (
	"testing"
	"time"
)

func TestSupervisorDecide(t *testing.T) {
	s := NewSupervisor()
	now := time.Now()

	if restart, giveUp := s.Decide(0, false, now); restart || giveUp {
		t.Errorf("exit 0: restart=%v giveUp=%v, want both false", restart, giveUp)
	}
	if restart, giveUp := s.Decide(1, true, now); restart || giveUp {
		t.Errorf("shutting down: restart=%v giveUp=%v, want both false", restart, giveUp)
	}

	for i := 0; i < s.MaxCrashes; i++ {
		now = now.Add(time.Second)
		restart, giveUp := s.Decide(1, false, now)
		if !restart || giveUp {
			t.Fatalf("crash %d: restart=%v giveUp=%v, want restart", i, restart, giveUp)
		}
	}
	now = now.Add(time.Second)
	restart, giveUp := s.Decide(1, false, now)
	if restart || !giveUp {
		t.Errorf("crash past the limit: restart=%v giveUp=%v, want give up", restart, giveUp)
	}
}

func TestSupervisorWindowExpires(t *testing.T) {
	s := NewSupervisor()
	now := time.Now()
	for i := 0; i < s.MaxCrashes; i++ {
		now = now.Add(time.Second)
		s.Decide(1, false, now)
	}
	// Past the window: the old crashes should no longer count against the limit.
	now = now.Add(s.Window + time.Second)
	restart, giveUp := s.Decide(1, false, now)
	if !restart || giveUp {
		t.Errorf("crash after the window cleared: restart=%v giveUp=%v, want restart", restart, giveUp)
	}
}
