package terminal

import (
	"testing"
	"time"
)

func TestReconnectJitterIsStableAndBounded(t *testing.T) {
	for attempt, base := range reconnectBackoff {
		got := jitteredReconnectDelay(attempt, "random-session-id")
		if again := jitteredReconnectDelay(attempt, "random-session-id"); again != got {
			t.Fatalf("attempt %d changed from %v to %v", attempt, got, again)
		}
		minimum := base * 80 / 100
		maximum := base * 120 / 100
		if got < minimum || got > maximum {
			t.Errorf("attempt %d = %v, want %v..%v", attempt, got, minimum, maximum)
		}
	}
}

func TestReconnectJitterSpreadsDifferentSessions(t *testing.T) {
	seen := map[time.Duration]bool{}
	for _, id := range []string{"one", "two", "three", "four", "five"} {
		seen[jitteredReconnectDelay(4, id)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("different sessions all received one delay: %#v", seen)
	}
}
