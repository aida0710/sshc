package vpnrefusal

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"sshc/internal/secret"
	"sshc/internal/vpn"
)

// engine が返しうる理由の語は、どれも言い方を持つ。
func TestEveryReasonHasASentence(t *testing.T) {
	for _, reason := range []vpn.Reason{
		vpn.ReasonRequired, vpn.ReasonFormat, vpn.ReasonTooLong, vpn.ReasonTooMany, vpn.ReasonOutOfRange,
		vpn.ReasonNotIPv4, vpn.ReasonUnroutable, vpn.ReasonNameNeedsDNS, vpn.ReasonUnsupported, vpn.ReasonUnexpected,
	} {
		if _, known := fieldReasons[reason]; !known {
			t.Errorf("%s has no field sentence", reason)
		}
	}
	for _, reason := range []vpn.FailureReason{
		vpn.FailureUnknown, vpn.FailureTimeout, vpn.FailureServerUnresolved, vpn.FailureIPsecNegotiation,
		vpn.FailurePPPAuthentication, vpn.FailureOpenConnect, vpn.FailureHandshakeTimeout, vpn.FailureTunnelLost,
	} {
		if _, known := sessionReasons[reason]; !known {
			t.Errorf("%s has no session sentence", reason)
		}
	}
	for _, reason := range []vpn.FailureReason{
		vpn.FailureTargetUnresolved, vpn.FailureTargetNeedsDNS, vpn.FailureTargetIsServer,
		vpn.FailureTargetUnreachable, vpn.FailureTunnelLost, vpn.FailureTimeout,
	} {
		if _, known := targetReasons[reason]; !known {
			t.Errorf("%s has no target sentence", reason)
		}
	}
	for _, known := range kinds {
		refusal := Refusal{Code: known.code}
		if sentence := Sentence(refusal); sentence == "VPNの操作に失敗しました。" {
			t.Errorf("%s has no sentence", known.code)
		}
	}
}

// 失敗は、種類と理由の語に直る。
func TestFailuresBecomeTheirWords(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want Refusal
	}{
		{"シークレットが無い", fmt.Errorf("route: %w", secret.ErrUnknownCredential), Refusal{Code: CodeSecretsMissing}},
		{"Dockerが無い", fmt.Errorf("%w: exec: not found", vpn.ErrDockerMissing), Refusal{Code: CodeDockerMissing}},
		{"接続先の名前解決", &vpn.TargetFailure{Profile: "lab", Destination: "db:22", Reason: vpn.FailureTargetUnresolved},
			Refusal{Code: CodeTargetFailed, Reason: string(vpn.FailureTargetUnresolved)}},
		{"DNSの無い名前", &vpn.DestinationError{Address: "db:22", Reason: vpn.ReasonNameNeedsDNS},
			Refusal{Code: CodeDestinationInvalid, Reason: string(vpn.ReasonNameNeedsDNS)}},
		{"ハンドシェイク", &vpn.SessionFailure{Profile: "lab", Reason: vpn.FailureHandshakeTimeout},
			Refusal{Code: CodeSessionFailed, Reason: string(vpn.FailureHandshakeTimeout)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			refusal, known := Of(test.err)
			if !known || refusal != test.want {
				t.Fatalf("Of = %+v, %v, want %+v", refusal, known, test.want)
			}
		})
	}
	if _, known := Of(errors.New("something else")); known {
		t.Fatal("VPN と関係ない失敗を VPN の拒否にした")
	}
}

// 直さない限り同じ理由で断られるものだけ、再接続を繰り返さない。
func TestOnlyRefusalsThatNeedAFixStopTheReconnect(t *testing.T) {
	for _, test := range []struct {
		refusal Refusal
		want    bool
	}{
		{Refusal{Code: CodeSecretsMissing}, true},
		{Refusal{Code: CodeDockerNotRunning}, true},
		{Refusal{Code: CodeSessionFailed, Reason: string(vpn.FailureHandshakeTimeout)}, true},
		{Refusal{Code: CodeSessionFailed, Reason: string(vpn.FailureTunnelLost)}, false},
		{Refusal{Code: CodeTargetFailed, Reason: string(vpn.FailureTargetUnresolved)}, true},
		{Refusal{Code: CodeTargetFailed, Reason: string(vpn.FailureTargetUnreachable)}, false},
	} {
		if got := test.refusal.RequiresAction(); got != test.want {
			t.Errorf("RequiresAction(%+v) = %v, want %v", test.refusal, got, test.want)
		}
	}
}

// 接続先へ届かなかった文は、何に失敗したかを先に書く。
func TestATargetFailureSaysWhatFailedFirst(t *testing.T) {
	sentence := Sentence(Refusal{Code: CodeTargetFailed, Reason: string(vpn.FailureTargetUnresolved)})
	if !strings.HasPrefix(sentence, "VPN経由で接続先に接続できませんでした。") ||
		!strings.Contains(sentence, "名前解決に失敗しました") {
		t.Fatalf("sentence = %q", sentence)
	}
}
