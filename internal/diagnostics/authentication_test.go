package diagnostics_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"sshc/internal/config"
	"sshc/internal/diagnostics"
	"sshc/internal/effective"
	"sshc/internal/platform"
	"sshc/internal/sshclient"
)

// scriptedProbe は、認証テストの継ぎ目を差し替える。
//
// 本物の握手を見るのは internal/sshclient の側である。ここが確かめるのは、
// 継ぎ目に届くまでの関門と、答えの符号への移し方である。
type scriptedProbe struct {
	calls  []string
	result sshclient.Probe
	err    error
}

func (p *scriptedProbe) dial(_ context.Context, alias string) (sshclient.Probe, error) {
	p.calls = append(p.calls, alias)
	return p.result, p.err
}

func reportFrom(t *testing.T, contents string) effective.Report {
	t.Helper()
	graph := &config.Graph{
		Root:  "/Users/tester/.ssh/config",
		Order: []string{"/Users/tester/.ssh/config"},
		Nodes: map[string]*config.Node{
			"/Users/tester/.ssh/config": {Path: "/Users/tester/.ssh/config", Editable: true, File: config.Parse([]byte(contents))},
		},
	}
	return effective.Scan(graph)
}

func TestAuthenticationTestReportsTheMethodThatWorked(t *testing.T) {
	probe := &scriptedProbe{result: sshclient.Probe{
		Method: "publickey", Tried: []string{"publickey"}, Elapsed: 12 * time.Millisecond,
	}}
	authentication := diagnostics.Authentication{Dial: probe.dial}

	result, err := authentication.Test(context.Background(), effective.Report{}, "bastion", false)
	if err != nil {
		t.Fatalf("Test = %v", err)
	}
	if !result.Authenticated || result.Outcome != diagnostics.OutcomeAuthenticated {
		t.Fatalf("result = %+v", result)
	}
	// **推測ではない。** 方式は順に試され、通った時点で握手が終わる。
	if result.Method != "publickey" {
		t.Errorf("method = %q", result.Method)
	}
	if len(probe.calls) != 1 || probe.calls[0] != "bastion" {
		t.Errorf("calls = %#v", probe.calls)
	}
}

// 実行を伴うディレクティブは、確認されるまで継ぎ目に届かない。
func TestAuthenticationTestRefusesUntilUnavoidableCommandsAreAcknowledged(t *testing.T) {
	probe := &scriptedProbe{}
	authentication := diagnostics.Authentication{Dial: probe.dial}
	report := reportFrom(t, "Host jump\n\tProxyCommand /usr/bin/nc %h %p\n")

	_, err := authentication.Test(context.Background(), report, "jump", false)
	var directiveError *diagnostics.ExecutableDirectiveError
	if !errors.As(err, &directiveError) {
		t.Fatalf("Test = %v, want *ExecutableDirectiveError", err)
	}
	if len(directiveError.Directives) != 1 || directiveError.Directives[0].Keyword != "ProxyCommand" {
		t.Fatalf("directives = %#v", directiveError.Directives)
	}
	if len(probe.calls) != 0 {
		t.Fatal("a refused authentication test reached the network")
	}

	if _, err := authentication.Test(context.Background(), report, "jump", true); err != nil {
		t.Fatalf("acknowledged Test = %v", err)
	}
	if len(probe.calls) != 1 {
		t.Fatalf("acknowledged test did not run: %#v", probe.calls)
	}

	overridable := reportFrom(t, "Host jump\n\tLocalCommand /usr/bin/say hi\n")
	if _, err := authentication.Test(context.Background(), overridable, "jump", false); err != nil {
		t.Fatalf("a directive this client does not have must not block the test: %v", err)
	}
}

// **型で判断する。** 出力の語句を読んでいたのは、外部のプログラムが理由を
// 型で返す手段を持たなかったからである。
func TestAuthenticationTestClassifiesFailuresByType(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{"host key changed", sshclient.ErrHostKeyChanged, diagnostics.OutcomeHostKeyChanged},
		{"host key unknown", sshclient.ErrHostKeyUnknown, diagnostics.OutcomeHostKeyUnknown},
		{"host key revoked", sshclient.ErrHostKeyRevoked, diagnostics.OutcomeHostKeyUnknown},
		{"nothing to offer", sshclient.ErrNoAuthMethod, diagnostics.OutcomeDenied},
		{"denied", errors.New("ssh: unable to authenticate, attempted methods [none publickey]"), diagnostics.OutcomeDenied},
		{"dns", &net.DNSError{Err: "no such host", Name: "nowhere.invalid"}, diagnostics.OutcomeDNSFailure},
		{"deadline", context.DeadlineExceeded, diagnostics.OutcomeTimeout},
		{"refused", errors.New("dial tcp 127.0.0.1:1: connect: connection refused"), diagnostics.OutcomeRefused},
		{"anything else", errors.New("something nobody has seen"), diagnostics.OutcomeFailed},
	} {
		probe := &scriptedProbe{err: test.err, result: sshclient.Probe{Tried: []string{"publickey"}}}
		authentication := diagnostics.Authentication{Dial: probe.dial}

		result, err := authentication.Test(context.Background(), effective.Report{}, "bastion", false)
		if err != nil {
			t.Fatalf("%s: Test = %v", test.name, err)
		}
		if result.Outcome != test.want {
			t.Errorf("%s: outcome = %q, want %q", test.name, result.Outcome, test.want)
		}
		if result.Authenticated {
			t.Errorf("%s: a failure reported authentication", test.name)
		}
		// 失敗の説明には、試した方式が入る。何も試していないのか、試して
		// 断られたのかは別の答えである。
		if !strings.Contains(result.Detail, "publickey") {
			t.Errorf("%s: detail = %q", test.name, result.Detail)
		}
	}
}

func TestAuthenticationTestRejectsUnsafeAliasesAndCapsReportedOutput(t *testing.T) {
	probe := &scriptedProbe{}
	authentication := diagnostics.Authentication{Dial: probe.dial}

	if _, err := authentication.Test(
		context.Background(), effective.Report{}, "-oProxyCommand=id", false,
	); !errors.Is(err, platform.ErrUnsafeAlias) {
		t.Fatalf("unsafe alias = %v, want ErrUnsafeAlias", err)
	}
	if len(probe.calls) != 0 {
		t.Fatal("an unsafe alias reached the network")
	}

	long := strings.Repeat("b", diagnostics.MaxReportedOutput*2)
	probe.result = sshclient.Probe{Method: "publickey", Banner: long}
	result, err := authentication.Test(context.Background(), effective.Report{}, "bastion", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Detail) > diagnostics.MaxReportedOutput || !result.Truncated {
		t.Fatalf("detail = %d bytes, truncated = %v", len(result.Detail), result.Truncated)
	}
}

// 手段が無いなら、試したふりをしない。
func TestAuthenticationTestWithoutAProbeRefuses(t *testing.T) {
	if _, err := (diagnostics.Authentication{}).Test(
		context.Background(), effective.Report{}, "bastion", false,
	); !errors.Is(err, diagnostics.ErrNoAuthenticator) {
		t.Fatalf("Test = %v, want ErrNoAuthenticator", err)
	}
}
