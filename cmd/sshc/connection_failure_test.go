package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"sshc/internal/sshclient"
	"sshc/internal/storage"
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

// 中断した変更が残っていると、受け入れたホスト鍵を known_hosts へ書けずに接続が止まる。
// storage の文は別の sshc が動いているように読めるので、CLI は History で片付ける案内を出す。
func TestAnInterruptedChangeThatStopsAConnectionPointsToHistory(t *testing.T) {
	// golang.org/x/crypto/ssh は、ホスト鍵の検証の失敗を %w で包んで返す。
	refused := fmt.Errorf("ssh: handshake failed: %w", storage.ErrPendingTransaction)
	for mode, failure := range map[string]error{
		"sshc ssh":                   describeConnectionFailure(refused),
		"sshc ssh --non-interactive": runAdvice(refused, "moon-tunnel"),
	} {
		sentence := failure.Error()
		if sentence != pendingTransactionAdvice {
			t.Errorf("%s: failure = %q, want %q", mode, sentence, pendingTransactionAdvice)
		}
		if strings.Contains(sentence, "another sshc process") {
			t.Errorf("%s: failure = %q, still reads as if another sshc were running", mode, sentence)
		}
	}
}

func TestConnectionFailuresWithoutAnEnglishSentenceAreReportedAsTheyAre(t *testing.T) {
	refused := errors.New("dial tcp 192.0.2.10:22: connect: connection refused")
	if got := describeConnectionFailure(refused); got != refused {
		t.Fatalf("failure = %v, want %v unchanged", got, refused)
	}
}
