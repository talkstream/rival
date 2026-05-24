package session

import (
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

// startupGrace is how long a "running" session may have PID=0 before being
// considered orphaned. New() writes the session as running with PID=0; the PID
// is only recorded once the subprocess starts. Without this grace window a
// concurrent rival invocation could reap a session that is just coming up.
const startupGrace = 30 * time.Second

// ReapOrphans finds sessions stuck in "running" whose process is dead, and marks them failed.
func ReapOrphans() {
	sessions := LoadAll()
	for _, s := range sessions {
		if s.Status != "running" {
			continue
		}
		if s.PID <= 0 {
			if time.Since(s.StartTime) < startupGrace {
				continue // still in the startup window — leave it alone
			}
			log.Info().Str("session", s.ID).Msg("reaping session that never recorded a PID")
			_ = s.Fail(1, "orphaned (never started)")
			continue
		}
		if !processAlive(s.PID) {
			log.Info().Str("session", s.ID).Int("pid", s.PID).Msg("reaping orphaned session")
			_ = s.Fail(1, "orphaned (process dead)")
		}
	}
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
