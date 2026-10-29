package terminal_test

import (
	"bytes"
	"context"
	"regexp"
	"strconv"
	"testing"
	"time"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

func TestTheTimesTheShellStartedAndFinishedACommandFollowItsMarksInTheOutput(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return spy.count() >= 1 })
	before := time.Now().UnixMilli()
	spy.at(0).feed("$ make\r\n\x1b]133;C\x07building\r\n\x1b]133;D;2\x07$ ")
	testwait.Until(t, func() bool { return bytes.Contains(snapshotOf(session), []byte("\x1b]7742;D;")) })
	after := time.Now().UnixMilli()

	output := snapshotOf(session)
	requireInOrder(t, output, "\x1b]133;C\x07", "\x1b]133;D;2\x07", "\x1b]7742;C;", "\x1b]7742;D;")
	marks := regexp.MustCompile("\x1b]7742;([CD]);(\\d+)\a").FindAllSubmatch(output, -1)
	if len(marks) != 2 {
		t.Fatalf("time marks = %q", marks)
	}
	started, _ := strconv.ParseInt(string(marks[0][2]), 10, 64)
	finished, _ := strconv.ParseInt(string(marks[1][2]), 10, 64)
	if started < before || finished > after || finished < started {
		t.Fatalf("started %d, finished %d, not within %d..%d", started, finished, before, after)
	}
}
