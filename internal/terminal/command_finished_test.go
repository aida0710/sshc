package terminal

import (
	"testing"
	"time"
)

func TestOnlyTheEndOfALongCommandIsNotified(t *testing.T) {
	session := newTitleTestSession("edge")
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	failed := 2

	// The first prompt of a shell reports an end with no command before it.
	session.acceptCommandFinished(1, nil, at)
	session.acceptCommandStarted(1, at)
	session.acceptCommandFinished(1, nil, at.Add(LongCommand-time.Second))
	if view := session.View(); view.NotificationVersion != 0 {
		t.Fatalf("a short command or a bare end was notified: version %d", view.NotificationVersion)
	}

	session.acceptCommandStarted(1, at)
	session.acceptCommandFinished(1, &failed, at.Add(LongCommand+time.Second))
	view := session.View()
	if view.NotificationVersion != 1 || view.LastNotification == nil || view.LastNotification.FinishedCommand == nil {
		t.Fatalf("the end of a long command = version %d %+v", view.NotificationVersion, view.LastNotification)
	}
	finished := view.LastNotification.FinishedCommand
	if finished.Duration != LongCommand+time.Second || finished.ExitCode == nil || *finished.ExitCode != 2 ||
		view.LastNotification.Title != "" || view.LastNotification.Body != "" {
		t.Fatalf("finished command = %+v, notification %+v", finished, view.LastNotification)
	}

	// The end counts once; the next end has no start of its own.
	session.acceptCommandFinished(1, nil, at.Add(2*LongCommand))
	if view := session.View(); view.NotificationVersion != 1 {
		t.Fatalf("one command was notified twice: version %d", view.NotificationVersion)
	}
}
