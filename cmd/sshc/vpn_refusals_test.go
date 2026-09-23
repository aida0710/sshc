package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"sshc/internal/vpn"
)

// engine の拒否は、理由の語を日本語の文にして見せる。
func TestVPNRefusalsAreExplainedInASentence(t *testing.T) {
	for _, test := range []struct {
		name    string
		called  vpnInvocation
		problem string
		want    []string
	}{
		{
			name:    "改名先の名前がある",
			called:  vpnInvocation{Action: vpnRename, Name: "old", Rename: "lab"},
			problem: `{"code":"vpn_profile_exists","message":"request rejected"}`,
			want:    []string{"lab という名前のVPNプロファイルはすでにあります"},
		},
		{
			name:    "項目の上限",
			called:  vpnInvocation{Action: vpnRename, Name: "old", Rename: "lab"},
			problem: `{"code":"vpn_profile_invalid","message":"request rejected","field":"name","reason":"too_long","limit":48}`,
			want:    []string{"name: 長すぎます（48文字まで）。"},
		},
		{
			name:    "経路を用意できなかった",
			called:  vpnInvocation{Action: vpnUp, Name: "lab"},
			problem: `{"code":"vpn_session_failed","message":"request rejected","reason":"handshake_timeout"}`,
			want:    []string{"WireGuardの相手と握手できませんでした", "sshc vpn logs lab"},
		},
		{
			name:    "接続先の食い違い",
			called:  vpnInvocation{Action: vpnUp, Name: "lab"},
			problem: `{"code":"vpn_target_mismatch","message":"request rejected"}`,
			want:    []string{"接続先と一致しません"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
				response.Header().Set("Content-Type", "application/problem+json")
				response.WriteHeader(http.StatusConflict)
				_, _ = response.Write([]byte(test.problem))
			})
			defer server.Close()
			var stdout, stderr strings.Builder

			code := runVPN(context.Background(), test.called, commandEnvironment{
				stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
			})

			if code != 1 {
				t.Fatalf("code = %d, stderr = %q", code, stderr.String())
			}
			for _, want := range test.want {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr omitted %q: %q", want, stderr.String())
				}
			}
		})
	}
}

// add で同じ名前があれば、消すか改名するかを案内する。
func TestAnExistingProfileNameSuggestsRemovingOrRenaming(t *testing.T) {
	sentence, known := describeVPNRefusal(engineProblem{Code: "vpn_profile_exists"},
		vpnInvocation{Action: vpnAdd, Name: "lab"})

	if !known || !strings.Contains(sentence, "sshc vpn remove lab") || !strings.Contains(sentence, "sshc vpn rename") {
		t.Fatalf("sentence = %q, %v", sentence, known)
	}
}

// engine が返しうる理由の語は、どれも CLI の言い方を持つ。
func TestEveryVPNReasonHasACommandLineSentence(t *testing.T) {
	for _, reason := range []vpn.Reason{
		vpn.ReasonRequired, vpn.ReasonFormat, vpn.ReasonTooLong, vpn.ReasonTooMany, vpn.ReasonOutOfRange,
		vpn.ReasonNotIPv4, vpn.ReasonUnroutable, vpn.ReasonNameNeedsDNS, vpn.ReasonUnsupported, vpn.ReasonUnexpected,
	} {
		if _, known := vpnFieldReasons[reason]; !known {
			t.Errorf("%s has no sentence", reason)
		}
	}
	for _, reason := range []vpn.FailureReason{
		vpn.FailureUnknown, vpn.FailureTimeout, vpn.FailureServerUnresolved, vpn.FailureIPsecNegotiation,
		vpn.FailurePPPAuthentication, vpn.FailureOpenConnect, vpn.FailureHandshakeTimeout,
		vpn.FailureTargetUnresolved, vpn.FailureTunnelLost,
	} {
		if _, known := vpnFailureReasons[reason]; !known {
			t.Errorf("%s has no sentence", reason)
		}
	}
}
