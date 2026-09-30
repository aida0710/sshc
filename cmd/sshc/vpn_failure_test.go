package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"unicode"
)

// VPN の失敗は、sync の文ではなく VPN の言い方で書き、状態は sshc vpn で見るよう案内する。
func TestVPNFailuresAreToldInTermsOfVPNNotSync(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
		exit int
	}{
		{
			name: "応答が途切れて結果が分からない",
			err:  engineProblem{Code: "transport_error", Retryable: true, OutcomeUnknown: true},
			want: "sshc: the connection to the engine ended before the VPN operation finished, " +
				"so its outcome is unknown; run sshc vpn to see the current state",
			exit: 1,
		},
		{
			name: "取り消した起動の結果が分からない",
			err:  engineProblem{Code: "transport_error", OutcomeUnknown: true, cause: context.Canceled},
			want: "run sshc vpn to see the current state",
			exit: 130,
		},
		{
			name: "取り消した",
			err:  context.Canceled,
			want: "sshc: the VPN operation was canceled",
			exit: 130,
		},
		{
			name: "engine の知らない失敗",
			err:  engineProblem{Status: 500, Code: "internal_error"},
			want: "sshc: the VPN operation failed (internal_error); run sshc vpn to see the current state",
			exit: 1,
		},
		{
			name: "engine が動いていない",
			err:  engineNotRunning{cause: os.ErrNotExist},
			want: "sshc: " + engineNotRunning{}.Error(),
			exit: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr strings.Builder

			code := finishVPNFailure(vpnInvocation{Action: vpnUp, Name: "lab"}, test.err,
				commandEnvironment{stdout: &stdout, stderr: &stderr})

			if code != test.exit || stdout.Len() != 0 {
				t.Fatalf("code = %d, stdout = %q", code, stdout.String())
			}
			if !strings.Contains(stderr.String(), test.want) || strings.Contains(stderr.String(), "sync") {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.want)
			}
		})
	}
}

// Vault のロックは、engine の拒否で知っても、engine へ繋ぐ前に知っても、同じ文で書く。
func TestALockedVaultReadsTheSameWhicheverWayItWasFound(t *testing.T) {
	var refused, beforeEngine strings.Builder
	called := vpnInvocation{Action: vpnUp, Name: "lab"}

	finishVPNFailure(called, engineProblem{Status: 423, Code: "vault_locked"},
		commandEnvironment{stdout: &strings.Builder{}, stderr: &refused})
	finishVPNFailure(called, errEngineVaultLocked, commandEnvironment{stdout: &strings.Builder{}, stderr: &beforeEngine})

	if refused.String() != beforeEngine.String() || !strings.Contains(refused.String(), "sshc vault unlock") {
		t.Fatalf("engine の拒否 = %q, engine へ繋ぐ前 = %q", refused.String(), beforeEngine.String())
	}
}

// ターミナルでないところから add を実行したら、sync setup ではなく VPN の入力を求めていると言う。
func TestAddingAProfileWithoutATerminalSaysWhatNeedsTheTerminal(t *testing.T) {
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnAdd, Name: "lab"},
		commandEnvironment{stateDir: t.TempDir(), stdout: &stdout, stderr: &stderr})

	if code != 1 || !strings.Contains(stderr.String(), "sshc vpn add") || strings.Contains(stderr.String(), "sync") {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
}

// --json では、共通の失敗の形で種類を返す。
func TestAVPNFailureInJSONKeepsTheCommonShape(t *testing.T) {
	var stdout, stderr strings.Builder

	code := finishVPNFailure(vpnInvocation{Action: vpnUp, Name: "lab", JSON: true},
		engineProblem{Code: "transport_error", OutcomeUnknown: true}, commandEnvironment{stdout: &stdout, stderr: &stderr})

	if code != 1 || stderr.Len() != 0 ||
		strings.TrimSpace(stdout.String()) != `{"schemaVersion":1,"success":false,"failure":{"kind":"outcome_unknown","retryable":false}}` {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}

// CLI が見つけた入力の誤りも、ほかの CLI の文と同じく英語で書く。
func TestInputMistakesAreWrittenInEnglish(t *testing.T) {
	for _, mistake := range []*vpnInputError{
		errVPNInputMissing, errVPNInputBackend, errVPNInputUnknown,
	} {
		if strings.ContainsFunc(mistake.sentence, func(character rune) bool { return character > unicode.MaxASCII }) {
			t.Errorf("英語でない文: %q", mistake.sentence)
		}
	}
}
