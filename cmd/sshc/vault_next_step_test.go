package main

import (
	"context"
	"errors"
	"testing"
)

// fixedEngineStatus は、どの home を尋ねられたかを記録し、決めた状態を返す。
type fixedEngineStatus struct {
	answer statusAnswer
	err    error
	asked  []string
}

func (status *fixedEngineStatus) read(_ context.Context, home string) (statusAnswer, error) {
	status.asked = append(status.asked, home)
	return status.answer, status.err
}

var (
	missingVault      = statusAnswer{}
	lockedVault       = statusAnswer{Vault: true}
	passwordlessVault = statusAnswer{Vault: true, Unlocked: true, Passwordless: true}
)

func TestVaultNextStepAsksForUnlockOnlyWhileTheVaultIsLocked(t *testing.T) {
	cases := []struct {
		name   string
		status fixedEngineStatus
		want   string
	}{
		{name: "no vault", status: fixedEngineStatus{answer: missingVault}, want: vaultMissingAdvice},
		{name: "locked vault", status: fixedEngineStatus{answer: lockedVault}, want: vaultLockedAdvice},
		{name: "passwordless vault", status: fixedEngineStatus{answer: passwordlessVault}, want: ""},
		{name: "unreadable engine", status: fixedEngineStatus{err: errors.New("no handoff")}, want: vaultStateUnknownAdvice},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			status := testCase.status
			if got := vaultNextStep(context.Background(), status.read, "/home/test"); got != testCase.want {
				t.Fatalf("vaultNextStep = %q, want %q", got, testCase.want)
			}
			if len(status.asked) != 1 || status.asked[0] != "/home/test" {
				t.Fatalf("asked homes = %q, want the service's home once", status.asked)
			}
		})
	}
}
