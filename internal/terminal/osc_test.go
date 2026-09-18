package terminal

import (
	"strings"
	"testing"
)

type observed struct {
	titles        []string
	notifications [][2]string
}

func newTestObserver() (*oscObserver, *observed) {
	seen := &observed{}
	observer := newOSCObserver(
		func(title string) { seen.titles = append(seen.titles, title) },
		func(title, body string) { seen.notifications = append(seen.notifications, [2]string{title, body}) },
	)
	return observer, seen
}

func TestOSCObserverRecordsTitles(t *testing.T) {
	observer, seen := newTestObserver()
	observer.Observe([]byte("prompt$ \x1b]0;vim ~/notes.md\a"))
	observer.Observe([]byte("\x1b]2;second\x1b\\"))
	observer.Observe([]byte("\x1b]1;icon\a"))
	if len(seen.titles) != 3 || seen.titles[0] != "vim ~/notes.md" || seen.titles[1] != "second" || seen.titles[2] != "icon" {
		t.Fatalf("titles = %q", seen.titles)
	}
	if len(seen.notifications) != 0 {
		t.Fatalf("unexpected notifications %q", seen.notifications)
	}
}

func TestOSCObserverSurvivesChunkBoundaries(t *testing.T) {
	observer, seen := newTestObserver()
	stream := "before\x1b]0;split title\aafter"
	for index := range stream {
		observer.Observe([]byte(stream[index : index+1]))
	}
	if len(seen.titles) != 1 || seen.titles[0] != "split title" {
		t.Fatalf("titles = %q", seen.titles)
	}
}

func TestOSCObserverCleansAndBoundsTitles(t *testing.T) {
	observer, seen := newTestObserver()
	observer.Observe([]byte("\x1b]0;\tsp\x01aced​ title  \a"))
	observer.Observe([]byte("\x1b]0;" + strings.Repeat("あ", MaxTitle+10) + "\a"))
	observer.Observe([]byte("\x1b]0;\a"))
	if len(seen.titles) != 3 || seen.titles[0] != "spaced title" {
		t.Fatalf("titles = %q", seen.titles)
	}
	if got := []rune(seen.titles[1]); len(got) != MaxTitle {
		t.Fatalf("long title kept %d runes", len(got))
	}
	if seen.titles[2] != "" {
		t.Fatalf("empty title should report a reset, got %q", seen.titles[2])
	}
}

func TestOSCObserverNotifications(t *testing.T) {
	observer, seen := newTestObserver()
	observer.Observe([]byte("\x1b]9;Claude is waiting for your input\a"))
	observer.Observe([]byte("\x1b]777;notify;Build finished;main.go compiled\x1b\\"))
	observer.Observe([]byte("\x1b]99;Task complete\a"))
	observer.Observe([]byte("\x1b]99;i=7:d=0:p=title;Deploy\a\x1b]99;i=7:p=body;staging is live\a"))
	observer.Observe([]byte("\x1b]99;i=8:e=1;SGVsbG8=\a"))
	want := [][2]string{
		{"", "Claude is waiting for your input"},
		{"Build finished", "main.go compiled"},
		{"", "Task complete"},
		{"Deploy", "staging is live"},
		{"", "Hello"},
	}
	if len(seen.notifications) != len(want) {
		t.Fatalf("notifications = %q", seen.notifications)
	}
	for index := range want {
		if seen.notifications[index] != want[index] {
			t.Fatalf("notification %d = %q, want %q", index, seen.notifications[index], want[index])
		}
	}
}

func TestOSCObserverIgnoresControlsThatShareNumbers(t *testing.T) {
	observer, seen := newTestObserver()
	observer.Observe([]byte("\x1b]9;4;1;50\a"))              // ConEmu progress
	observer.Observe([]byte("\x1b]777;something-else;x\a"))  // unknown 777 command
	observer.Observe([]byte("\x1b]7;file://host/tmp\a"))     // cwd
	observer.Observe([]byte("\x1b]133;A\a"))                 // prompt marks
	observer.Observe([]byte("\x1b]52;c;aGVsbG8=\a"))         // clipboard
	observer.Observe([]byte("\x1b]9;\a\x1b]777;notify;;\a")) // empty notifications
	observer.Observe([]byte("\x1b]0;still parsing\a"))
	if len(seen.notifications) != 0 {
		t.Fatalf("unexpected notifications %q", seen.notifications)
	}
	if len(seen.titles) != 1 || seen.titles[0] != "still parsing" {
		t.Fatalf("titles = %q", seen.titles)
	}
}

func TestOSCObserverBoundsPayloads(t *testing.T) {
	observer, seen := newTestObserver()
	observer.Observe([]byte("\x1b]0;" + strings.Repeat("x", MaxOSCPayload+1) + "\a\x1b]0;ok\a"))
	if len(seen.titles) != 1 || seen.titles[0] != "ok" {
		t.Fatalf("titles = %q", seen.titles)
	}
	if len(observer.payload) != 0 || observer.state != oscText {
		t.Fatalf("observer did not reset after discarding: state=%d payload=%d", observer.state, len(observer.payload))
	}
}

func TestOSCObserverRecoversFromMalformedSequences(t *testing.T) {
	observer, seen := newTestObserver()
	observer.Observe([]byte("\x1b]0;unterminated\x1bXnope\x1b]0;good\a"))
	observer.Observe([]byte("\x1b]abc;junk\a\x1b]2;after junk\a"))
	observer.Observe([]byte("\x1b[31mred\x1b]0;after csi\a"))
	if len(seen.titles) != 3 || seen.titles[0] != "good" || seen.titles[1] != "after junk" || seen.titles[2] != "after csi" {
		t.Fatalf("titles = %q", seen.titles)
	}
}

func FuzzOSCObserverChunkingIsInvariant(f *testing.F) {
	for _, seed := range []string{
		"plain", "\x1b]0;title\a", "\x1b]2;title\x1b\\", "\x1b]9;note\a", "\x1b]777;notify;t;b\a",
		"\x1b]99;i=1:d=0:p=title;T\a\x1b]99;i=1;B\a", "\x1b", "\x1b]", "\x1b]0;", "\x1b]abc;x\a",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		whole, wholeSeen := newTestObserver()
		whole.Observe(input)
		split, splitSeen := newTestObserver()
		for index := range input {
			split.Observe(input[index : index+1])
		}
		if len(whole.payload) > MaxOSCPayload || len(split.payload) > MaxOSCPayload {
			t.Fatalf("payload grew past the bound: %d / %d", len(whole.payload), len(split.payload))
		}
		if pending := pendingKittyBytes(whole); pending > 2*maxKittyChunkBytes {
			t.Fatalf("pending kitty chunks grew past the bound: %d", pending)
		}
		if strings.Join(wholeSeen.titles, "\x00") != strings.Join(splitSeen.titles, "\x00") {
			t.Fatalf("titles differ by chunking: %q vs %q", wholeSeen.titles, splitSeen.titles)
		}
		if len(wholeSeen.notifications) != len(splitSeen.notifications) {
			t.Fatalf("notifications differ by chunking: %q vs %q", wholeSeen.notifications, splitSeen.notifications)
		}
		for index := range wholeSeen.notifications {
			if wholeSeen.notifications[index] != splitSeen.notifications[index] {
				t.Fatalf("notification %d differs: %q vs %q", index, wholeSeen.notifications[index], splitSeen.notifications[index])
			}
		}
	})
}

func pendingKittyBytes(observer *oscObserver) int {
	total := 0
	for _, pending := range observer.chunks {
		total += len(pending.title) + len(pending.body)
	}
	return total
}

func TestKittyNotificationChunksStopAccumulatingAtTheDeliveredLimit(t *testing.T) {
	observer, seen := newTestObserver()
	chunk := "\x1b]99;i=1:d=0:p=body;" + strings.Repeat("x", MaxOSCPayload-24) + "\a"
	for range 1000 {
		observer.Observe([]byte(chunk))
	}
	pending := observer.chunks["1"]
	if pending == nil {
		t.Fatal("no pending chunk")
	}
	if len(pending.body) > maxKittyChunkBytes {
		t.Fatalf("pending body = %d bytes, want at most %d", len(pending.body), maxKittyChunkBytes)
	}
	observer.Observe([]byte("\x1b]99;i=1:p=body;end\a"))
	if len(seen.notifications) != 1 || len([]rune(seen.notifications[0][1])) != MaxNotificationBodyRunes {
		t.Fatalf("delivered notification = %q", seen.notifications)
	}
}
