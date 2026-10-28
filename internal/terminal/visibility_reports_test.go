package terminal

import (
	"fmt"
	"testing"
)

func observedVisibilityRequests(chunks ...string) []visibilityRequest {
	var requests []visibilityRequest
	observer := newVisibilityRequestObserver(func(request visibilityRequest) { requests = append(requests, request) })
	for _, chunk := range chunks {
		observer.Observe([]byte(chunk))
	}
	return requests
}

func TestTheVisibilityRequestsAreReadFromTheOutput(t *testing.T) {
	got := observedVisibilityRequests(
		"\x1b[?2033h", "\x1b[?2033$p", "\x1b[?998n", "\x1b[?1;2033;1004l",
		// A reset ends reports as well.
		"\x1b[?2033h\x1bc",
	)
	want := []visibilityRequest{
		enableVisibilityReports, queryVisibilityMode, queryVisibility, disableVisibilityReports,
		enableVisibilityReports, disableVisibilityReports,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

func TestAVisibilityRequestSplitAcrossReadsIsStillRead(t *testing.T) {
	sequence := "text\x1b[?2033h"
	chunks := make([]string, len(sequence))
	for index := range sequence {
		chunks[index] = sequence[index : index+1]
	}
	if got := observedVisibilityRequests(chunks...); fmt.Sprint(got) != fmt.Sprint([]visibilityRequest{enableVisibilityReports}) {
		t.Fatalf("requests = %v", got)
	}
}

func TestOtherSequencesAreNotTakenForVisibilityRequests(t *testing.T) {
	got := observedVisibilityRequests(
		"\x1b[2033h",          // not a private mode
		"\x1b[?20330h",        // another mode
		"\x1b[?999;1n",        // a report, not a query
		"\x1b[?6n\x1b[?1049h", // cursor position, alternate screen
		"\x1b[?1;1;1;1;1;1;1;1;1;1;1;1;1;1;1;2033h", // past the bound, so something else
		"\x1b]0;\x1b[?2033h",                        // inside an OSC that the ESC ends, then a real request
	)
	want := []visibilityRequest{enableVisibilityReports}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("requests = %v, want only the one after the cut OSC", got)
	}
}
