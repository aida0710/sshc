package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"sshc/internal/effective"
	"sshc/internal/sshclient"
	"sshc/internal/validate"
)

const (
	// MaxReportedOutput は、表示用に返す文字列の量に上限を設ける。
	MaxReportedOutput = 8 << 10
	// DefaultAuthenticationTimeout は、認証テスト一回に上限を設ける。
	DefaultAuthenticationTimeout = 20 * time.Second
)

// 認証テストの結果。
const (
	OutcomeAuthenticated  = "authenticated"
	OutcomeDenied         = "authentication_denied"
	OutcomeHostKeyUnknown = "host_key_unknown"
	OutcomeHostKeyChanged = "host_key_changed"
	OutcomeDNSFailure     = "dns_failure"
	OutcomeRefused        = "connection_refused"
	OutcomeTimeout        = "timeout"
	OutcomeFailed         = "failed"
)

// ExecutableDirectiveError は、接続するとコマンドラインオプションでは無効化できない
// コマンドが実行されること、そしてユーザーがそれを確認していないことを報告する。
type ExecutableDirectiveError struct {
	Directives []effective.Executable
}

func (e *ExecutableDirectiveError) Error() string {
	return fmt.Sprintf("connecting would run %d configured command(s) that cannot be disabled", len(e.Directives))
}

// AuthenticationResult は、完了した認証テストひとつ分。
type AuthenticationResult struct {
	Outcome       string
	Authenticated bool
	// Method は認証に成功した方式。
	Method string
	// Detail は、結果をユーザーへ説明する文字列。上限つきで、秘密を含まない。
	Detail    string
	Truncated bool
	Elapsed   time.Duration
}

// Authentication は、外部プログラムを起動せずに SSH 認証を一度試行する。
type Authentication struct {
	// Dial は、接続ひとつ分を組み立てて認証だけを試す。
	Dial func(ctx context.Context, alias string) (sshclient.Probe, error)
	// Timeout は、テスト一回の上限である。
	Timeout time.Duration
}

// ErrNoAuthenticator は、認証を試す手段が配線されていないことを報告する。
var ErrNoAuthenticator = errors.New("no authentication probe is available")

// Test は alias に対して認証し、結果が判明した時点で止まる。
//
// acknowledged は、report.Unavoidable() のコマンドをそのまま表示したうえで消費
// されたアクショントークンから来なければならない。
//
// プロセス内クライアントが実行しない機能は、個別に無効化する必要がない。
func (a Authentication) Test(
	ctx context.Context, report effective.Report, alias string, acknowledged bool,
) (AuthenticationResult, error) {
	if err := validate.Alias(alias); err != nil {
		return AuthenticationResult{}, err
	}
	if blocking := report.Unavoidable(); len(blocking) > 0 && !acknowledged {
		return AuthenticationResult{}, &ExecutableDirectiveError{Directives: blocking}
	}
	if a.Dial == nil {
		return AuthenticationResult{}, ErrNoAuthenticator
	}

	timeout := a.Timeout
	if timeout <= 0 {
		timeout = DefaultAuthenticationTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	probe, err := a.Dial(ctx, alias)
	result := AuthenticationResult{Method: probe.Method, Elapsed: probe.Elapsed}
	if err == nil {
		result.Outcome, result.Authenticated = OutcomeAuthenticated, true
		result.Detail = trimOutput(probe.Banner)
		result.Truncated = len(probe.Banner) > MaxReportedOutput
		return result, nil
	}

	result.Method = ""
	result.Outcome = classify(err)
	detail := describe(err, probe)
	result.Detail = trimOutput(detail)
	result.Truncated = len(detail) > MaxReportedOutput
	return result, nil
}

// classify は、エラーの型から安定した結果コードを返す。
func classify(err error) string {
	var dns *net.DNSError
	switch {
	case errors.Is(err, sshclient.ErrHostKeyChanged):
		return OutcomeHostKeyChanged
	case errors.Is(err, sshclient.ErrHostKeyUnknown), errors.Is(err, sshclient.ErrHostKeyRevoked):
		return OutcomeHostKeyUnknown
	case errors.Is(err, sshclient.ErrNoAuthMethod):
		return OutcomeDenied
	case errors.As(err, &dns):
		return OutcomeDNSFailure
	case errors.Is(err, context.DeadlineExceeded), isTimeout(err):
		return OutcomeTimeout
	case isRefused(err):
		return OutcomeRefused
	case strings.Contains(err.Error(), "unable to authenticate"):
		return OutcomeDenied
	default:
		return OutcomeFailed
	}
}

func isTimeout(err error) bool {
	var timeout interface{ Timeout() bool }
	return errors.As(err, &timeout) && timeout.Timeout()
}

func isRefused(err error) bool {
	// syscall の符号を import せずに済ませる。net はこの語で報告する。
	return strings.Contains(err.Error(), "connection refused")
}

// describe は、秘密情報を含まないユーザー向け説明を組み立てる。
func describe(err error, probe sshclient.Probe) string {
	var builder strings.Builder
	builder.WriteString(err.Error())
	if len(probe.Tried) > 0 {
		builder.WriteString(" (tried ")
		builder.WriteString(strings.Join(probe.Tried, ", "))
		builder.WriteString(")")
	}
	if probe.Banner != "" {
		builder.WriteString("\n")
		builder.WriteString(probe.Banner)
	}
	return builder.String()
}

func trimOutput(text string) string {
	if len(text) <= MaxReportedOutput {
		return text
	}
	return text[:MaxReportedOutput]
}
