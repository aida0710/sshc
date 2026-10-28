package terminal

import "time"

// The end of a long command counts as a notification, so a session nobody is
// looking at still gets an unread mark and a desktop notification. The engine
// times it rather than the browser: a session in another workspace or outside
// the split view has no terminal drawn in any browser.

// LongCommand is the shortest command whose end is notified. Below it the
// user is most likely still watching, and every quick command would mark the
// session unread.
const LongCommand = 30 * time.Second

// FinishedCommand is a command of at least LongCommand that the shell
// reported finished.
type FinishedCommand struct {
	Duration time.Duration
	// ExitCode is nil when the shell did not report one.
	ExitCode *int
}

func (s *Session) acceptCommandStarted(generation uint64, at time.Time) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.commandStartedAt = at
}

// acceptCommandFinished notifies the end of a long command. An end without a
// start, such as the first prompt of a shell, is not a command.
func (s *Session) acceptCommandFinished(generation uint64, exitCode *int, at time.Time) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	started := s.commandStartedAt
	s.commandStartedAt = time.Time{}
	if started.IsZero() || at.Sub(started) < LongCommand {
		return
	}
	s.notificationVersion++
	s.lastNotification = &Notification{
		OccurredAt:      at,
		FinishedCommand: &FinishedCommand{Duration: at.Sub(started), ExitCode: exitCode},
	}
}
