package connectionattempt

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"sshc/internal/terminal"
)

const validID = "6f1c2b9e-0d4a-4c1e-9a77-3b2f5d8e1a40"

func TestAnAttemptReportsItsStageAndEveryCompleteLogLine(t *testing.T) {
	registry := NewRegistry()
	attempt, err := registry.Begin("web", validID)
	if err != nil {
		t.Fatal(err)
	}
	attempt.Progress(terminal.ConnectionProgress{Phase: terminal.ConnectionDialing, Alias: "web", Hop: 1, Hops: 2})
	// The tracer writes CRLF lines, and a write may end in the middle of a line.
	_, _ = attempt.Write([]byte("[sshc][debug1] 接続先：web\r\n[sshc][debug1] TCP"))
	_, _ = attempt.Write([]byte("接続を確立しました。\r\n"))

	snapshot, ok := registry.Snapshot("web", validID)
	if !ok {
		t.Fatal("the attempt was not found")
	}
	if snapshot.Progress == nil || snapshot.Progress.Phase != terminal.ConnectionDialing || snapshot.Progress.Hops != 2 {
		t.Errorf("progress = %+v", snapshot.Progress)
	}
	want := []string{"[sshc][debug1] 接続先：web", "[sshc][debug1] TCP接続を確立しました。"}
	if fmt.Sprint(snapshot.Log) != fmt.Sprint(want) || snapshot.Finished {
		t.Errorf("snapshot = %+v, want log %q and not finished", snapshot, want)
	}
}

func TestAFailedAttemptKeepsItsLastLineAndTheReason(t *testing.T) {
	registry := NewRegistry()
	attempt, _ := registry.Begin("web", validID)
	_, _ = attempt.Write([]byte("[sshc] 未完了の行"))
	attempt.Finish(errors.New("ssh: handshake failed"))

	snapshot, _ := registry.Snapshot("web", validID)
	want := []string{"[sshc] 未完了の行", "sshc: ssh: handshake failed"}
	if fmt.Sprint(snapshot.Log) != fmt.Sprint(want) || !snapshot.Finished {
		t.Errorf("snapshot = %+v, want log %q and finished", snapshot, want)
	}
}

func TestAnAttemptIsReadOnlyThroughTheAliasItWasBegunFor(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Begin("web", validID); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Snapshot("db", validID); ok {
		t.Error("another alias read the attempt")
	}
	if _, ok := registry.Snapshot("web", "00000000-0000-0000-0000-000000000000"); ok {
		t.Error("an unknown id was found")
	}
}

func TestAnIdentifierOutsideTheAgreedShapeIsRefused(t *testing.T) {
	for _, id := range []string{"", "short", "../../etc/passwd-0000000", "6f1c2b9e 0d4a 4c1e 9a77", string(make([]byte, 65))} {
		if _, err := NewRegistry().Begin("web", id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Begin(%q) = %v, want ErrInvalidID", id, err)
		}
	}
}

func TestAFinishedAttemptIsForgottenAfterTheRetention(t *testing.T) {
	registry := NewRegistry()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return now }
	attempt, _ := registry.Begin("web", validID)
	attempt.Finish(nil)

	now = now.Add(finishedRetention)
	if _, ok := registry.Snapshot("web", validID); !ok {
		t.Fatal("the attempt was forgotten before the web UI could read its last lines")
	}
	now = now.Add(time.Second)
	if _, ok := registry.Snapshot("web", validID); ok {
		t.Error("a finished attempt was kept past the retention")
	}
}

func TestTheRegistryKeepsABoundedNumberOfAttemptsAndLines(t *testing.T) {
	registry := NewRegistry()
	clock := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { clock = clock.Add(time.Millisecond); return clock }
	for index := 0; index <= maxAttempts; index++ {
		if _, err := registry.Begin("web", fmt.Sprintf("attempt-%016d", index)); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := registry.Snapshot("web", fmt.Sprintf("attempt-%016d", 0)); ok {
		t.Error("the oldest attempt was kept beyond the limit")
	}
	if _, ok := registry.Snapshot("web", fmt.Sprintf("attempt-%016d", maxAttempts)); !ok {
		t.Error("the newest attempt was not kept")
	}

	attempt, _ := registry.Begin("web", validID)
	for index := 0; index < maxLogLines+10; index++ {
		_, _ = fmt.Fprintf(attempt, "line %d\r\n", index)
	}
	snapshot, _ := registry.Snapshot("web", validID)
	if len(snapshot.Log) != maxLogLines || snapshot.Log[0] != "line 10" {
		t.Errorf("kept %d lines starting at %q, want the last %d", len(snapshot.Log), snapshot.Log[0], maxLogLines)
	}
}
