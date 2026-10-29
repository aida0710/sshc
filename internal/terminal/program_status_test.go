package terminal

import (
	"encoding/base64"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func encodedText(text string) string { return base64.StdEncoding.EncodeToString([]byte(text)) }

func percentOf(value int) *int { return &value }

// observeProgramStatus feeds sequences one byte at a time, which also covers
// a report split across reads, and returns what reached the session.
func observeProgramStatus(t *testing.T, output string) []programStatusReport {
	t.Helper()
	var reports []programStatusReport
	observer := newOSCObserver(oscReports{
		ProgramStatus: func(report programStatusReport) { reports = append(reports, report) },
		Notify: func(title, body string) {
			t.Fatalf("a program status emitted a notification: %q %q", title, body)
		},
	})
	for _, value := range []byte(output) {
		observer.Observe([]byte{value})
	}
	return reports
}

func TestProgramStatusReportsAreReadWithEitherTerminator(t *testing.T) {
	for _, terminator := range []string{"\x1b\\", "\a"} {
		reports := observeProgramStatus(t, "\x1b]7501;state=blocked:kind=question:app=make:msg="+encodedText("Overwrite config.toml?")+terminator)
		want := []programStatusReport{{status: ProgramStatus{
			State: ProgramStatusBlocked, Kind: ProgramStatusQuestion, App: "make", Message: "Overwrite config.toml?",
		}}}
		if !reflect.DeepEqual(reports, want) {
			t.Fatalf("terminator %q: reports = %+v, want %+v", terminator, reports, want)
		}
	}
}

func TestProgramStatusFollowsTheSpecForEachKey(t *testing.T) {
	cases := []struct {
		name, payload string
		want          programStatusReport
	}{
		{"progress on a working record", "state=working:progress=40", programStatusReport{status: ProgramStatus{State: ProgramStatusWorking, Progress: percentOf(40)}}},
		{"progress out of range is indeterminate", "state=working:progress=101", programStatusReport{status: ProgramStatus{State: ProgramStatusWorking}}},
		{"progress that is not a number is indeterminate", "state=blocked:progress=4.5", programStatusReport{status: ProgramStatus{State: ProgramStatusBlocked}}},
		{"progress only goes with working and blocked", "state=done:progress=40", programStatusReport{status: ProgramStatus{State: ProgramStatusDone}}},
		{"kind only goes with blocked", "state=working:kind=question", programStatusReport{status: ProgramStatus{State: ProgramStatusWorking}}},
		{"an unknown kind counts as none", "state=blocked:kind=coffee", programStatusReport{status: ProgramStatus{State: ProgramStatusBlocked}}},
		{"an app with a byte out of place counts as none", "state=idle:app=a,b", programStatusReport{status: ProgramStatus{State: ProgramStatusIdle}}},
		{"whitespace around keys and values is dropped", " state = done : id = build/test ", programStatusReport{id: "build/test", status: ProgramStatus{State: ProgramStatusDone}}},
		{"an unknown key is ignored", "state=idle:colour=red", programStatusReport{status: ProgramStatus{State: ProgramStatusIdle}}},
		{"a malformed pair is skipped", "state=idle:app=make:nonsense:Title=x:msg=a b", programStatusReport{status: ProgramStatus{State: ProgramStatusIdle, App: "make"}}},
		{"the last of a repeated key wins", "state=working:state=error", programStatusReport{status: ProgramStatus{State: ProgramStatusError}}},
		{"padding is optional", "state=done:title=" + strings.TrimRight(encodedText("ok!"), "="), programStatusReport{status: ProgramStatus{State: ProgramStatusDone, Title: "ok!"}}},
		{"direction overrides are dropped from text shown outside the terminal", "state=done:msg=" + encodedText("a‮bc"), programStatusReport{status: ProgramStatus{State: ProgramStatusDone, Message: "abc"}}},
		{"clear takes an id", "state=clear:id=build", programStatusReport{clear: true, id: "build"}},
		{"a query", "?", programStatusReport{query: true}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseProgramStatus(test.payload)
			if !ok || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseProgramStatus(%q) = %+v, %v; want %+v", test.payload, got, ok, test.want)
			}
		})
	}
}

func TestProgramStatusReportsTheSpecThrowsAwayAreDropped(t *testing.T) {
	for name, payload := range map[string]string{
		"no state":                       "app=make",
		"an unknown state":               "state=sleeping",
		"an id with a byte out of place": "state=idle:id=build test",
		"an id with an empty segment":    "state=idle:id=build//test",
		"an id nine segments deep":       "state=idle:id=a/b/c/d/e/f/g/h/i",
		"an id segment too long":         "state=idle:id=" + strings.Repeat("a", maxProgramStatusSegmentBytes+1),
		"a key too long":                 "state=idle:" + strings.Repeat("k", maxProgramStatusKeyBytes+1) + "=1",
		"an app too long":                "state=idle:app=" + strings.Repeat("a", maxProgramStatusAppBytes+1),
		"a message that is not base64":   "state=done:msg=a",
		"a message with a newline":       "state=done:msg=" + encodedText("two\nlines"),
		"a message with a tab":           "state=done:msg=" + encodedText("a\tb"),
		"a message with a C1 control":    "state=done:msg=" + encodedText("a\u009bb"),
		"a message that is not UTF-8":    "state=done:msg=" + base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe}),
		"a message too long":             "state=done:msg=" + encodedText(strings.Repeat("x", maxProgramStatusMessageBytes+1)),
		"a title too long":               "state=done:title=" + encodedText(strings.Repeat("x", maxProgramStatusTitleBytes+1)),
	} {
		t.Run(name, func(t *testing.T) {
			if report, ok := parseProgramStatus(payload); ok {
				t.Fatalf("parseProgramStatus(%q) = %+v, want it dropped", payload, report)
			}
		})
	}
}

func TestASequencePastTheSizeLimitIsDropped(t *testing.T) {
	long := "\x1b]7501;state=done:msg=" + encodedText(strings.Repeat("x", 2048)) + ":title=" + strings.Repeat("A", MaxOSCPayload) + "\x1b\\"
	if reports := observeProgramStatus(t, long); len(reports) != 0 {
		t.Fatalf("reports = %+v, want the oversized sequence dropped", reports)
	}
}

func apply(records *programStatusRecords, payloads ...string) {
	for _, payload := range payloads {
		report, ok := parseProgramStatus(payload)
		if !ok {
			panic("bad payload in test: " + payload)
		}
		records.apply(report)
	}
}

func TestAReportReplacesItsRecordWhole(t *testing.T) {
	var records programStatusRecords
	apply(&records, "state=working:app=cargo:progress=10:msg="+encodedText("Building"), "state=working")
	summary := records.summary()
	if summary == nil || summary.App != "" || summary.Progress != nil || summary.Message != "" {
		t.Fatalf("summary = %+v, want the keys the second report left out gone", summary)
	}
}

func TestTheSummaryIsTheRecordThatMostNeedsTheUser(t *testing.T) {
	var records programStatusRecords
	apply(&records,
		"state=idle:app=claude",
		"state=done:id=lint",
		"state=working:id=build",
		"state=blocked:id=build/deploy:kind=permission",
	)
	summary := records.summary()
	if summary.State != ProgramStatusBlocked || summary.Kind != ProgramStatusPermission {
		t.Fatalf("summary = %+v, want the blocked record", summary)
	}
	// The child takes the name of its nearest ancestor that has one, here the root.
	if summary.App != "claude" {
		t.Fatalf("summary app = %q, want it from the root", summary.App)
	}

	apply(&records, "state=clear:id=build")
	if got := records.summary().State; got != ProgramStatusDone {
		t.Fatalf("after clearing build and what is under it, summary = %q, want done", got)
	}
	apply(&records, "state=done:id=test")
	if summary := records.summary(); summary.Version != 5 {
		t.Fatalf("summary version = %d, want the latest of the two done records (5)", summary.Version)
	}
	apply(&records, "state=clear")
	if summary := records.summary(); summary != nil {
		t.Fatalf("after a clear without an id, summary = %+v, want none", summary)
	}
}

func TestPastTheRecordLimitTheLeastRecentlyUpdatedMakesWay(t *testing.T) {
	var records programStatusRecords
	apply(&records, "state=error:id=oldest")
	for index := range maxProgramStatusRecords {
		apply(&records, "state=idle:id=task"+strconv.Itoa(index))
	}
	if len(records.byID) != maxProgramStatusRecords {
		t.Fatalf("records = %d, want %d", len(records.byID), maxProgramStatusRecords)
	}
	if _, kept := records.byID["oldest"]; kept {
		t.Fatal("the least recently updated record was kept past the limit")
	}
}

func TestAPromptAndAResetReachTheSessionFromTheOutput(t *testing.T) {
	var prompts, resets int
	observer := newOSCObserver(oscReports{
		PromptStarted: func() { prompts++ },
		Reset:         func() { resets++ },
	})
	// The RIS right after an aborted OSC string still counts.
	observer.Observe([]byte("\x1b]133;A\a$ \x1b]0;cut\x1bc"))
	if prompts != 1 || resets != 1 {
		t.Fatalf("prompts = %d, resets = %d, want 1 and 1", prompts, resets)
	}
}
