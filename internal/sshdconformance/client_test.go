// Package sshdconformance は、このアプリケーションの SSH クライアントを、
// 本物の OpenSSH の sshd に対して走らせる。
//
// 単体テストでは Go 実装のサーバーを使用するため、OpenSSH との相互運用性は
// このテストで検証する。
//
// アドレスが設定されていなければ丸ごとスキップするので、`go test ./...` は
// 密閉されたままである。`make integration` がコンテナで sshd を起動する。
package sshdconformance_test

import (
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

const (
	addressVariable    = "SSHC_TEST_SSH_ADDR"
	userVariable       = "SSHC_TEST_SSH_USER"
	passwordVariable   = "SSHC_TEST_SSH_PASSWORD"
	keyVariable        = "SSHC_TEST_SSH_KEY"
	passphraseVariable = "SSHC_TEST_SSH_KEY_PASSPHRASE"
)

// server は、コンテナの sshd の住所と、そこへ入るための資格情報である。
type server struct {
	host       string
	port       string
	user       string
	password   string
	key        string
	passphrase string
}

func integrationServer(t *testing.T) server {
	t.Helper()
	address := os.Getenv(addressVariable)
	if address == "" {
		t.Skipf("%s is not set; start a server with `make integration` to run this", addressVariable)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("%s=%q is not host:port: %v", addressVariable, address, err)
	}
	return server{
		host:       host,
		port:       port,
		user:       os.Getenv(userVariable),
		password:   os.Getenv(passwordVariable),
		key:        os.Getenv(keyVariable),
		passphrase: os.Getenv(passphraseVariable),
	}
}

// target は、設定解決を介さずにテスト用 sshd へ接続する Target を返す。
func (s server) target() sshclient.Target {
	return sshclient.Target{
		Alias:    "integration",
		HostName: s.host,
		Port:     s.port,
		User:     s.user,
		Methods:  sshclient.DefaultMethods(),
		Timeout:  30 * time.Second,
	}
}

// knownHosts は、実行中の sshd からホスト鍵を取得して known_hosts を生成する。
func (s server) knownHosts(t *testing.T) []byte {
	t.Helper()
	keys, err := sshclient.ScanHostKeys(t.Context(), nil, net.JoinHostPort(s.host, s.port), 0)
	if err != nil {
		t.Fatalf("scanning the host keys of the running sshd: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("the running sshd offered no host key")
	}
	var lines strings.Builder
	for _, key := range keys {
		lines.WriteString(knownHostsLine(s.host, s.port, key))
	}
	return []byte(lines.String())
}

func knownHostsLine(host, port string, key ssh.PublicKey) string {
	field := host
	if port != "22" {
		field = "[" + host + "]:" + port
	}
	return field + " " + string(ssh.MarshalAuthorizedKey(key))
}

// keyDialer は、鍵ひとつだけで入る Dialer である。
func (s server) keyDialer(t *testing.T, observed *[]string) sshclient.Dialer {
	t.Helper()
	if s.key == "" {
		t.Skipf("%s is not set", keyVariable)
	}
	known := s.knownHosts(t)
	return sshclient.Dialer{
		Auth: sshclient.Auth{
			Stored: func(path string) (string, bool) {
				if path != s.key {
					return "", false
				}
				return s.passphrase, true
			},
			Observe: func(method string) {
				if observed != nil {
					*observed = append(*observed, method)
				}
			},
		},
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return known, nil }},
	}
}

// keyTarget は、その鍵だけを使う指定である。agent も既定の探索も使わない。
func (s server) keyTarget() sshclient.Target {
	target := s.target()
	target.Identities = []string{s.key}
	target.IdentitiesOnly = true
	return target
}

// TestRunAuthenticatesWithAKeyAgainstRealSshd は、非対話実行でパスフレーズ付き鍵を
// 復号し、OpenSSH sshd に公開鍵認証できることを検証する。
func TestRunAuthenticatesWithAKeyAgainstRealSshd(t *testing.T) {
	remote := integrationServer(t)
	var observed []string
	dialer := remote.keyDialer(t, &observed)

	output, err := dialer.Run(t.Context(), remote.keyTarget(), "echo integration-canary", nil)
	if err != nil {
		t.Fatalf("running a command over the real sshd: %v", err)
	}
	if got := strings.TrimSpace(string(output.Stdout)); got != "integration-canary" {
		t.Fatalf("stdout = %q, want %q (stderr %q)", got, "integration-canary", output.Stderr)
	}
	if output.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", output.ExitCode)
	}
	if len(observed) == 0 || observed[0] != "publickey" {
		t.Fatalf("methods tried = %v, want publickey first", observed)
	}
}

// TestRunCarriesTheRemoteExitCode は、リモートの終了コードが維持されることを検証する。
func TestRunCarriesTheRemoteExitCode(t *testing.T) {
	remote := integrationServer(t)
	dialer := remote.keyDialer(t, nil)

	output, err := dialer.Run(t.Context(), remote.keyTarget(), "exit 17", nil)
	if err != nil {
		t.Fatalf("running a command that exits non-zero: %v", err)
	}
	if output.ExitCode != 17 {
		t.Fatalf("exit code = %d, want 17", output.ExitCode)
	}
}

// TestRunRefusesAHostThatIsNotInKnownHosts は、非対話実行が未知のホスト鍵を
// 自動承認しないことを検証する。
func TestRunRefusesAHostThatIsNotInKnownHosts(t *testing.T) {
	remote := integrationServer(t)
	dialer := remote.keyDialer(t, nil)
	dialer.HostKeys = sshclient.HostKeys{}

	_, err := dialer.Run(t.Context(), remote.keyTarget(), "echo never", nil)
	if !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("error = %v, want %v", err, sshclient.ErrHostKeyUnknown)
	}
}

// TestOpenAsksForThePasswordAndRunsAShell は、パスワード入力から PTY 上の
// シェル実行までの対話経路を検証する。
func TestOpenAsksForThePasswordAndRunsAShell(t *testing.T) {
	remote := integrationServer(t)
	if remote.password == "" {
		t.Skipf("%s is not set", passwordVariable)
	}
	known := remote.knownHosts(t)
	dialer := sshclient.Dialer{
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return known, nil }},
	}

	// 鍵は渡さない。残る方式はパスワードだけである。
	target := remote.target()
	target.IdentitiesOnly = true

	process, err := dialer.Open(t.Context(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("opening a session: %v", err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "Password for ")
	if _, err := io.WriteString(process, remote.password+"\n"); err != nil {
		t.Fatalf("answering the password prompt: %v", err)
	}

	// PTY の入力 echo とコマンド出力を区別するため、入力にはシェルが除去する
	// 引用符を含める。
	if _, err := io.WriteString(process, "echo inter''active-canary\n"); err != nil {
		t.Fatalf("writing to the shell: %v", err)
	}
	readUntil(t, process, "interactive-canary")
}

// readUntil は、その断片が現れるまで読む。
//
// 出力の道は誰かが読み続けていなければ詰まるので、待つことと読むことを分けない。
func readUntil(t *testing.T, process terminal.Process, wanted string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var seen strings.Builder
	buffer := make([]byte, 4096)
	for time.Now().Before(deadline) {
		read, err := process.Read(buffer)
		seen.Write(buffer[:read])
		if strings.Contains(seen.String(), wanted) {
			return
		}
		if err != nil {
			t.Fatalf("the session ended before %q appeared: %v (saw %q)", wanted, err, seen.String())
		}
	}
	t.Fatalf("%q never appeared (saw %q)", wanted, seen.String())
}
