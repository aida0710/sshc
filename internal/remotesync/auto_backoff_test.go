package remotesync

import (
	"testing"
	"time"
)

func TestBackoffDelayDoublesFromTheIntervalAndStopsAtTheMaximum(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		interval       time.Duration
		failedAttempts int
		want           time.Duration
	}{
		{name: "first failure waits one interval", interval: time.Minute, failedAttempts: 1, want: time.Minute},
		{name: "second failure doubles", interval: time.Minute, failedAttempts: 2, want: 2 * time.Minute},
		{name: "third failure doubles again", interval: time.Minute, failedAttempts: 3, want: 4 * time.Minute},
		{name: "doubling past the maximum stops at it", interval: time.Minute, failedAttempts: 4, want: autoBackoffMax},
		{name: "many failures stay at the maximum", interval: time.Minute, failedAttempts: 100, want: autoBackoffMax},
		{name: "an interval longer than the maximum waits the maximum", interval: 2 * autoBackoffMax, failedAttempts: 1, want: autoBackoffMax},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := backoffDelay(test.interval, test.failedAttempts); got != test.want {
				t.Fatalf("backoffDelay(%s, %d) = %s, want %s", test.interval, test.failedAttempts, got, test.want)
			}
		})
	}
}
