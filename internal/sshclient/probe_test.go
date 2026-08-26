package sshclient_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
)

func TestProbeNamesTheMethodThatWorked(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	probe, err := dialerFor(t, server, auth).Probe(context.Background(), targetWith(server, path))
	if err != nil {
		t.Fatalf("Probe = %v", err)
	}
	if probe.Method != "publickey" {
		t.Errorf("method = %q, want the one that worked", probe.Method)
	}
	if len(probe.Tried) == 0 {
		t.Error("the probe reports nothing it tried")
	}
}

// 何も尋ねない。上限つきで非対話という約束は、外部の ssh に BatchMode を
// 渡していたときと同じである。渡す相手が変わっただけで、約束は変わらない。
func TestProbeNeverAsksTheUser(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	// パスワードを知っている問い手を用意しても、Probe はそれを使わない。
	auth := sshclient.Auth{}

	_, err := dialerFor(t, server, auth).Probe(context.Background(), targetWith(server))
	if !errors.Is(err, sshclient.ErrNoAuthMethod) {
		t.Fatalf("Probe = %v, want ErrNoAuthMethod", err)
	}
	if attempts := server.Attempts(); attempts != 0 {
		t.Fatalf("a probe with nothing to offer still tried %d time(s)", attempts)
	}
}

func TestProbeUsesAStoredPasswordWithoutAskingTheUser(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	auth := sshclient.Auth{Password: func(sshclient.Target) (string, bool) {
		return "hunter2", true
	}}

	probe, err := dialerFor(t, server, auth).Probe(context.Background(), targetWith(server))

	if err != nil {
		t.Fatalf("Probe = %v", err)
	}
	if probe.Method != "password" {
		t.Fatalf("method = %q, want password", probe.Method)
	}
}

func TestProbeReportsAuthenticationThatDidNotPass(t *testing.T) {
	path, contents, _ := keyPair(t)
	// サーバーは別の鍵しか受け付けない。
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{newHostKey(t).PublicKey()}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	probe, err := dialerFor(t, server, auth).Probe(context.Background(), targetWith(server, path))
	if err == nil {
		t.Fatal("a key the server does not accept still authenticated")
	}
	if probe.Method != "" {
		t.Errorf("method = %q, want none", probe.Method)
	}
	if len(probe.Tried) == 0 {
		t.Error("the probe reports nothing it tried, so the refusal proves nothing")
	}
}

// 検査のために信頼を増やさない。未知のホストは StrictHostKeyChecking=yes
// 相当で断る。設定が no と言っていても、こちらは断る。
func TestProbeRefusesAnUnknownHostEvenWhenTheConfigurationWouldNot(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	var written int
	dialer := sshclient.Dialer{
		Auth: auth,
		HostKeys: sshclient.HostKeys{
			Read: func() ([]byte, error) { return nil, nil },
			Add:  func(knownhosts.Candidate) error { written++; return nil },
		},
	}
	target := targetWith(server, path)
	target.Strict = "no"

	if _, err := dialer.Probe(context.Background(), target); !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("Probe = %v, want ErrHostKeyUnknown", err)
	}
	if written != 0 {
		t.Fatal("a probe wrote a host key into known_hosts")
	}
}

func TestProbeRefusesAnUnknownProxyJumpWithoutPersistingIt(t *testing.T) {
	path, contents, public := keyPair(t)
	inner := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	edge := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public}, AllowDirectTCPIP: true,
	})
	edge.allow(inner.Address())
	written := 0
	dialer := sshclient.Dialer{
		Auth: sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{
			Read: func() ([]byte, error) {
				return []byte(knownHostsLine("["+inner.Host()+"]:"+inner.Port(), inner.HostKey.PublicKey())), nil
			},
			Add: func(knownhosts.Candidate) error { written++; return nil },
		},
	}
	target := targetWith(inner, path)
	jump := targetWith(edge, path)
	jump.Strict = "no"
	target.Jump = []sshclient.Target{jump}

	_, err := dialer.Probe(context.Background(), target)
	if !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("Probe = %v, want ErrHostKeyUnknown", err)
	}
	if written != 0 {
		t.Fatal("Probe persisted an unknown ProxyJump host key")
	}
}

func TestProbeIncludesAProxyCommandsComplaint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX shell command")
	}
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	target := targetWith(server, path)
	target.ProxyCommand = "echo zzz-probe-route-failed >&2; exit 1"
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	_, err := dialerFor(t, server, auth).Probe(context.Background(), target)
	if err == nil || !strings.Contains(err.Error(), "zzz-probe-route-failed") {
		t.Fatalf("Probe = %v, want ProxyCommand complaint", err)
	}
}

func TestProbeCarriesTheServerBanner(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		Banner:     "this host is monitored",
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	probe, err := dialerFor(t, server, auth).Probe(context.Background(), targetWith(server, path))
	if err != nil {
		t.Fatalf("Probe = %v", err)
	}
	if probe.Banner != "this host is monitored" {
		t.Errorf("banner = %q", probe.Banner)
	}
}
