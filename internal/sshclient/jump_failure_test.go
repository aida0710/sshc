package sshclient_test

import (
	"strings"
	"testing"

	"sshc/internal/sshclient"
)

func TestAFailureAtAJumpHostNamesTheJumpHostBeforeTheReason(t *testing.T) {
	refused := refusedDial(t)
	failure := &sshclient.JumpHostError{Name: "jump", Hop: 1, Hops: 2, Err: refused}

	sentence := sshclient.FailureSentence(failure)
	if !strings.HasPrefix(sentence, "踏み台のjump（1/2）に接続できませんでした。") {
		t.Errorf("FailureSentence = %q, want it to start with the jump host and its hop", sentence)
	}
	if !strings.HasSuffix(sentence, sshclient.FailureSentence(refused)) {
		t.Errorf("FailureSentence = %q, want it to end with the reason %q", sentence, sshclient.FailureSentence(refused))
	}
	if got := sshclient.ClassifyConnectFailure(failure); got != sshclient.ConnectFailureRefused {
		t.Errorf("ClassifyConnectFailure = %v, want the reason of the jump host's failure", got)
	}
}
