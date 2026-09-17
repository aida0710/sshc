package terminal

import (
	"testing"
	"time"
)

func newTitleTestSession(alias string) *Session {
	session := &Session{
		id: "s1", kind: KindSSH, alias: alias, title: alias, fallbackTitle: alias,
		generation: 1, state: StateConnected, buffer: NewRing(32),
	}
	session.recomputeTitleLocked()
	return session
}

func TestTerminalTitleDrivesPresentationUntilPinned(t *testing.T) {
	session := newTitleTestSession("edge")
	if view := session.View(); view.Title != "edge" || view.Presentation.TitleSource != TitleConnection {
		t.Fatalf("initial view = %+v", view.Presentation)
	}
	session.acceptTitle(1, "vim notes.md")
	view := session.View()
	if view.Title != "vim notes.md" || view.Presentation.DisplayTitle != "vim notes.md" || view.Presentation.TitleSource != TitleTerminal {
		t.Fatalf("terminal title view = %+v", view.Presentation)
	}
	if err := session.Rename("pinned"); err != nil {
		t.Fatal(err)
	}
	session.acceptTitle(1, "htop")
	if view := session.View(); view.Title != "pinned" || view.Presentation.TitleSource != TitleUser || !view.Presentation.TitlePinned {
		t.Fatalf("pinned view = %+v", view.Presentation)
	}
	session.UnpinTitle()
	if view := session.View(); view.Title != "htop" || view.Presentation.TitleSource != TitleTerminal {
		t.Fatalf("unpinned view = %+v", view.Presentation)
	}
	session.acceptTitle(1, "")
	if view := session.View(); view.Title != "edge" || view.Presentation.TitleSource != TitleConnection {
		t.Fatalf("cleared view = %+v", view.Presentation)
	}
}

func TestTerminalTitleIgnoresStaleGenerations(t *testing.T) {
	session := newTitleTestSession("edge")
	session.acceptTitle(0, "old shell")
	if view := session.View(); view.Title != "edge" {
		t.Fatalf("stale generation title applied: %q", view.Title)
	}
	session.acceptTitle(1, "current")
	session.mutex.Lock()
	session.resetTerminalTitleLocked()
	session.mutex.Unlock()
	if view := session.View(); view.Title != "edge" || view.Presentation.TitleSource != TitleConnection {
		t.Fatalf("title survived process replacement: %+v", view.Presentation)
	}
}

func TestNotificationsAdvanceVersion(t *testing.T) {
	session := newTitleTestSession("")
	at := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	if view := session.View(); view.NotificationVersion != 0 || view.LastNotification != nil {
		t.Fatalf("initial notifications = %+v", view)
	}
	session.acceptNotification(1, "Build", "done", at)
	session.acceptNotification(1, "Build", "done", at.Add(time.Second))
	view := session.View()
	if view.NotificationVersion != 2 || view.LastNotification == nil ||
		view.LastNotification.Title != "Build" || view.LastNotification.Body != "done" ||
		!view.LastNotification.OccurredAt.Equal(at.Add(time.Second)) {
		t.Fatalf("notification view = %+v last=%+v", view.NotificationVersion, view.LastNotification)
	}
	session.acceptNotification(2, "stale", "", at)
	if view := session.View(); view.NotificationVersion != 2 {
		t.Fatalf("stale generation notification counted: %d", view.NotificationVersion)
	}
	if view.Presentation.TitleSource != TitleFallback {
		t.Fatalf("shell session without alias should report fallback, got %q", view.Presentation.TitleSource)
	}
}
