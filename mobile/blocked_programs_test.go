package mobile

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"sshc/internal/terminal"
)

const blockedProgramTestTimeout = 10 * time.Second

func blockedChange(sessionID string, status *terminal.ProgramStatus) terminal.BlockedProgramChange {
	return terminal.BlockedProgramChange{SessionID: sessionID, SessionAlias: "alias of " + sessionID, SessionTitle: "title of " + sessionID, Status: status}
}

func TestANewerChangeOfASessionReplacesTheOneNotTaken(t *testing.T) {
	queue := newBlockedProgramQueue()
	queue.add(blockedChange("one", &terminal.ProgramStatus{State: terminal.ProgramStatusBlocked}))
	queue.add(blockedChange("two", &terminal.ProgramStatus{State: terminal.ProgramStatusBlocked}))
	queue.add(blockedChange("one", nil))

	first := queue.take()
	second := queue.take()
	if first.SessionID != "one" || first.Status != nil {
		t.Errorf("first = %+v, want one that no longer waits", first)
	}
	if second.SessionID != "two" || second.Status == nil {
		t.Errorf("second = %+v, want two that waits", second)
	}
}

func TestTakeWaitsForAChange(t *testing.T) {
	queue := newBlockedProgramQueue()
	taken := make(chan terminal.BlockedProgramChange, 1)
	go func() { taken <- queue.take() }()

	queue.add(blockedChange("one", nil))
	select {
	case change := <-taken:
		if change.SessionID != "one" {
			t.Fatalf("change = %+v, want one", change)
		}
	case <-time.After(blockedProgramTestTimeout):
		t.Fatal("take did not return the change")
	}
}

func TestAChangeEncodesWhatTheNotificationSays(t *testing.T) {
	for _, test := range []struct {
		name   string
		status *terminal.ProgramStatus
		want   blockedProgramChangeJSON
	}{
		{
			name:   "the message goes before the title",
			status: &terminal.ProgramStatus{Kind: terminal.ProgramStatusPermission, Title: "title", Message: "approve"},
			want:   blockedProgramChangeJSON{Blocked: true, Kind: "permission", Said: "approve"},
		},
		{
			name:   "the title stands in for a missing message",
			status: &terminal.ProgramStatus{Title: "title"},
			want:   blockedProgramChangeJSON{Blocked: true, Said: "title"},
		},
		{name: "a program no longer waiting says nothing", want: blockedProgramChangeJSON{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got blockedProgramChangeJSON
			if err := json.Unmarshal([]byte(encodeBlockedProgramChange(blockedChange("one", test.status))), &got); err != nil {
				t.Fatal(err)
			}
			test.want.SessionID, test.want.SessionAlias, test.want.SessionTitle = "one", "alias of one", "title of one"
			if got != test.want {
				t.Fatalf("encoded = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestAChangeOfAnEngineThatHasStoppedStillArrives(t *testing.T) {
	url, err := Start(t.TempDir(), t.TempDir())
	if err != nil || url == "" {
		t.Fatalf("Start = %q, %v", url, err)
	}
	if err := Stop(); err != nil {
		t.Fatalf("Stop = %v", err)
	}

	blockedPrograms.add(blockedChange("left over", nil))
	if got := NextBlockedProgramChange(); !strings.Contains(got, `"left over"`) {
		t.Fatalf("NextBlockedProgramChange = %q, want the change left over", got)
	}
}

func TestDependenciesPassTheBlockedProgramChangesOn(t *testing.T) {
	heard := make(chan terminal.BlockedProgramChange, 1)
	dependencies, err := newDependencies(dependencyOptions{
		ctx: t.Context(), goos: "android", home: t.TempDir(), cache: t.TempDir(),
		programBlocked: func(change terminal.BlockedProgramChange) { heard <- change },
	})
	if err != nil {
		t.Fatal(err)
	}
	dependencies.ProgramBlocked(blockedChange("one", nil))
	if change := <-heard; change.SessionID != "one" {
		t.Fatalf("heard %+v, want one", change)
	}
}
