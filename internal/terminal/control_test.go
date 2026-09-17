package terminal

import (
	"testing"
	"time"
)

func TestControlStateUsesOnlyExplicitLifecycleState(t *testing.T) {
	now := time.Now()
	session := &Session{
		generation: 1, state: StateConnected, buffer: NewRing(32),
		now: func() time.Time { return now },
	}
	if got, _ := session.ReadControl(0, 0); got.State != ControlConnected {
		t.Fatalf("initial state = %q", got.State)
	}
	// Titles and notifications are presentation only; they never change the
	// machine-readable lifecycle state automation waits on.
	session.acceptTitle(1, "claude — working")
	session.acceptNotification(1, "", "waiting for input", now)
	if got, _ := session.ReadControl(0, 0); got.State != ControlConnected {
		t.Fatalf("state after title/notification = %q", got.State)
	}
	session.state = StateReconnecting
	if got, _ := session.ReadControl(0, 0); got.State != ControlReconnecting {
		t.Fatalf("reconnecting state = %q", got.State)
	}
}
