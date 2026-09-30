package terminal

import (
	"fmt"
	"testing"
	"time"
)

func TestReconnectJitterIsStableAndBounded(t *testing.T) {
	for attempt := range ReconnectBackoff {
		base := ReconnectBase(attempt)
		got := jitteredReconnectDelay(attempt, "random-session-id")
		if again := jitteredReconnectDelay(attempt, "random-session-id"); again != got {
			t.Fatalf("attempt %d changed from %v to %v", attempt, got, again)
		}
		minimum := base * ReconnectJitterMinPercent / 100
		maximum := base * ReconnectJitterMaxPercent / 100
		if got < minimum || got > maximum {
			t.Errorf("attempt %d = %v, want %v..%v", attempt, got, minimum, maximum)
		}
	}
}

// 設定画面の文言は ReconnectJitterMaxPercent から最大の待ち時間を言う。実装の範囲が
// 定数より広がっても狭まっても、多数のセッションの最小と最大がずれて赤くなる。
func TestReconnectJitterAcrossSessionsSpansExactlyTheAdvertisedRange(t *testing.T) {
	attempt := len(ReconnectBackoff) - 1
	base := ReconnectBase(attempt)
	lowest, highest := jitteredReconnectDelay(attempt, "session-0"), time.Duration(0)
	for index := range 2000 {
		delay := jitteredReconnectDelay(attempt, fmt.Sprintf("session-%d", index))
		lowest, highest = min(lowest, delay), max(highest, delay)
	}
	if want := base * ReconnectJitterMinPercent / 100; lowest != want {
		t.Errorf("shortest delay = %v, want %v", lowest, want)
	}
	if want := base * ReconnectJitterMaxPercent / 100; highest != want {
		t.Errorf("longest delay = %v, want %v", highest, want)
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

func TestReconnectBaseRepeatsTheLastBackoffAfterTheTableEnds(t *testing.T) {
	last := ReconnectBackoff[len(ReconnectBackoff)-1]
	if got := ReconnectBase(len(ReconnectBackoff) + 3); got != last {
		t.Fatalf("ReconnectBase past the table = %v, want %v", got, last)
	}
	if got := ReconnectBase(0); got != ReconnectBackoff[0] {
		t.Fatalf("ReconnectBase(0) = %v, want %v", got, ReconnectBackoff[0])
	}
}
