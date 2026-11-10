package terminal

import (
	"testing"
	"time"
)

func TestProcessStatGivesTheNameAndStartEvenWhenTheNameHasParentheses(t *testing.T) {
	// 22番目の欄（starttime）は 12345 刻みで、起動から 123.45 秒後である。
	stat := "4242 (my (odd) cmd) S 1 4242 4242 34816 4242 4194304 100 0 0 0 0 0 0 0 20 0 1 0 12345 2000000 100 0\n"
	name, started, ok := parseProcessStat(stat)
	if !ok || name != "my (odd) cmd" || started != 123450*time.Millisecond {
		t.Fatalf("parseProcessStat() = %q, %v, %v", name, started, ok)
	}
	if _, _, ok := parseProcessStat("4242 (sleep) S 1"); ok {
		t.Fatal("a cut-off stat line was read")
	}
}

func TestUptimeIsTheFirstNumber(t *testing.T) {
	if uptime, ok := parseUptime("350735.47 234388.90\n"); !ok || uptime != 350735470*time.Millisecond {
		t.Fatalf("parseUptime() = %v, %v", uptime, ok)
	}
}
