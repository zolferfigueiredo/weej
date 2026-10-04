package core

import "time"

type Supervisor struct {
	Delay      time.Duration
	MaxCrashes int
	Window     time.Duration

	crashes []time.Time
}

func NewSupervisor() *Supervisor {
	return &Supervisor{Delay: 5 * time.Second, MaxCrashes: 5, Window: 2 * time.Minute}
}

// A clean exit (0) or a deliberate shutdown is never a crash. Otherwise this crash joins the
// window's tally, and too many within it gives up rather than restarting forever.
func (s *Supervisor) Decide(exitCode int, shuttingDown bool, now time.Time) (restart, giveUp bool) {
	if exitCode == 0 || shuttingDown {
		return false, false
	}
	cutoff := now.Add(-s.Window)
	kept := s.crashes[:0]
	for _, t := range s.crashes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	s.crashes = append(kept, now)
	if len(s.crashes) > s.MaxCrashes {
		return false, true
	}
	return true, false
}
