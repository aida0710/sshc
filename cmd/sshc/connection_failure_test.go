package main

import (
	"errors"
	"strings"
	"testing"

	"sshc/internal/sshclient"
)

// sshclient は AWS SSO の認証切れに、sshcエンジンのターミナル向けの日本語の案内を
// 付けて返す。CLI は対話接続でも非対話実行でも、それを英語の案内に置き換える。
func TestAnExpiredAWSSSOLoginIsExplainedInEnglishOnTheCommandLine(t *testing.T) {
	expired := &sshclient.ExplainedError{
		Sentence: "AWS SSOの認証が必要です。",
		Err: errors.Join(sshclient.ErrProxyAuthenticationRequired,
			errors.New("ProxyCommand said: Error when retrieving token from sso: Token has expired and refresh failed")),
	}
	for mode, failure := range map[string]error{
		"sshc ssh":                   describeConnectionFailure(expired),
		"sshc ssh --non-interactive": runAdvice(expired, "moon-tunnel"),
	} {
		sentence := failure.Error()
		if !strings.Contains(sentence, "aws sso login --profile") || !strings.Contains(sentence, "--use-device-code") {
			t.Errorf("%s: failure = %q, want the aws sso login command", mode, sentence)
		}
		if containsJapanese(sentence) {
			t.Errorf("%s: failure = %q, want English only", mode, sentence)
		}
	}
}

func TestConnectionFailuresWithoutAnEnglishSentenceAreReportedAsTheyAre(t *testing.T) {
	refused := errors.New("dial tcp 192.0.2.10:22: connect: connection refused")
	if got := describeConnectionFailure(refused); got != refused {
		t.Fatalf("failure = %v, want %v unchanged", got, refused)
	}
}
