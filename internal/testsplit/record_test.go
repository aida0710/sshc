package testsplit

import (
	"strings"
	"testing"
)

func TestRecordSecondsKeepsTheSlowestTimeOfEachPassedPackage(t *testing.T) {
	seconds := map[string]float64{"sshc/internal/keys": 80}
	output := strings.Join([]string{
		"2026-11-06T13:05:01.1Z ok  \tsshc/internal/keys\t70.372s",
		"ok  \tsshc/internal/sftp\t46.501s",
		"ok  \tsshc/internal/sftp\t50.000s",
		"?   \tsshc/internal/api\t[no test files]",
		"FAIL\tsshc/internal/broken\t3.000s",
		"--- PASS: TestSomething (0.01s)",
	}, "\n")
	if err := RecordSeconds(strings.NewReader(output), seconds); err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"sshc/internal/keys": 80, "sshc/internal/sftp": 50}
	if len(seconds) != len(want) {
		t.Fatalf("seconds = %v, want %v", seconds, want)
	}
	for name, value := range want {
		if seconds[name] != value {
			t.Errorf("seconds[%q] = %v, want %v", name, seconds[name], value)
		}
	}
}
