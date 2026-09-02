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
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

const (
	addressVariable     = "SSHC_TEST_SSH_ADDR"
	userVariable        = "SSHC_TEST_SSH_USER"
	passwordVariable    = "SSHC_TEST_SSH_PASSWORD"
	keyVariable         = "SSHC_TEST_SSH_KEY"
	passphraseVariable  = "SSHC_TEST_SSH_KEY_PASSPHRASE"
	forwardDestVariable = "SSHC_TEST_FORWARD_DEST_ADDR"
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

func (s server) passwordDialer(t *testing.T) sshclient.Dialer {
	t.Helper()
	if s.password == "" {
		t.Skipf("%s is not set", passwordVariable)
	}
	known := s.knownHosts(t)
	return sshclient.Dialer{
		Auth: sshclient.Auth{Password: func(target sshclient.Target) (string, bool) {
			return s.password, target.Alias == "integration"
		}},
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return known, nil }},
	}
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

func TestConcurrentCommandsAgainstRealOpenSSH(t *testing.T) {
	remote := integrationServer(t)
	dialer := remote.keyDialer(t, nil)
	target := remote.keyTarget()
	const sessions = 16
	errorsFound := make(chan error, sessions)
	var group sync.WaitGroup
	for index := 0; index < sessions; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			want := fmt.Sprintf("concurrent-%02d", index)
			output, err := dialer.Run(t.Context(), target, "printf "+want, nil)
			if err != nil {
				errorsFound <- fmt.Errorf("session %d: %w", index, err)
				return
			}
			if got := string(output.Stdout); got != want {
				errorsFound <- fmt.Errorf("session %d output = %q, want %q", index, got, want)
			}
		}()
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
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
	// 実際の PTY を通し、Enter 前にも各文字が即座に伏せ字として返ることを確認する。
	// 最後に余計な一文字と Backspace も送り、表示と入力値の双方が戻ることを検証する。
	for _, character := range remote.password {
		if _, err := io.WriteString(process, string(character)); err != nil {
			t.Fatalf("typing a password character: %v", err)
		}
		readUntil(t, process, "*")
	}
	if _, err := io.WriteString(process, "x"); err != nil {
		t.Fatalf("typing the character to erase: %v", err)
	}
	readUntil(t, process, "*")
	if _, err := process.Write([]byte{0x7f}); err != nil {
		t.Fatalf("erasing a password character: %v", err)
	}
	readUntil(t, process, "\b \b")
	if _, err := io.WriteString(process, "\n"); err != nil {
		t.Fatalf("submitting the password prompt: %v", err)
	}

	// PTY の入力 echo とコマンド出力を区別するため、入力にはシェルが除去する
	// 引用符を含める。
	if _, err := io.WriteString(process, "echo inter''active-canary\n"); err != nil {
		t.Fatalf("writing to the shell: %v", err)
	}
	readUntil(t, process, "interactive-canary")
}

// TestOpenPTYAllowsProgramsToEnterRawMode は、prompt_toolkit などの対話CLIが
// OpenSSH上のPTYをraw modeへ切り替え、元へ戻せることを検証する。
func TestOpenPTYAllowsProgramsToEnterRawMode(t *testing.T) {
	remote := integrationServer(t)
	process, err := remote.keyDialer(t, nil).Open(
		t.Context(),
		remote.keyTarget(),
		terminal.Size{Cols: 80, Rows: 24},
	)
	if err != nil {
		t.Fatalf("opening a session: %v", err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "$ ")
	command := "stty raw; status=$?; stty sane; printf 'raw-mode-status=%s\\n' \"$status\"\n"
	if _, err := io.WriteString(process, command); err != nil {
		t.Fatalf("asking the remote PTY to enter raw mode: %v", err)
	}
	readUntil(t, process, "raw-mode-status=0")
}

// Local Forwardのlistener、SSH direct-tcpip channel、container network、転送先を
// 一続きで検証する。転送先OpenSSHのbannerまで届けば、往復の経路が通っている。
func TestLocalForwardCarriesTrafficThroughRealOpenSSH(t *testing.T) {
	remote := integrationServer(t)
	destination := forwardDestination(t)
	port := freePort(t)
	target := remote.keyTarget()
	target.Forwards = []sshclient.ForwardSpec{{
		Kind: terminal.ForwardLocal, ListenPort: port, To: destination,
	}}
	process, err := remote.keyDialer(t, nil).Open(t.Context(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("opening a session with local forwarding: %v", err)
	}
	t.Cleanup(func() { _ = process.Close() })
	readUntil(t, process, "sshc: forwarding ")

	connection, err := dialEventually(net.JoinHostPort(sshclient.LoopbackHost, port), 10*time.Second)
	if err != nil {
		t.Fatalf("connecting to the local forward: %v", err)
	}
	defer func() { _ = connection.Close() }()
	if banner := readSSHBanner(t, connection); !strings.HasPrefix(banner, "SSH-2.0-") {
		t.Fatalf("destination banner = %q", banner)
	}
}

// Dynamic ForwardはSOCKS5のhandshakeから始め、OpenSSH server側でcontainer名を
// 解決して転送先へ接続するところまでを検証する。
func TestDynamicForwardCarriesSOCKSTrafficThroughRealOpenSSH(t *testing.T) {
	remote := integrationServer(t)
	destination := forwardDestination(t)
	port := freePort(t)
	target := remote.target()
	target.IdentitiesOnly = true
	target.Forwards = []sshclient.ForwardSpec{{Kind: terminal.ForwardDynamic, ListenPort: port}}
	process, err := remote.passwordDialer(t).Open(t.Context(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("opening a session with dynamic forwarding: %v", err)
	}
	t.Cleanup(func() { _ = process.Close() })
	readUntil(t, process, "sshc: forwarding ")

	connection, err := dialEventually(net.JoinHostPort(sshclient.LoopbackHost, port), 10*time.Second)
	if err != nil {
		t.Fatalf("connecting to the SOCKS listener: %v", err)
	}
	defer func() { _ = connection.Close() }()
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := connection.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(connection, greeting); err != nil {
		t.Fatal(err)
	}
	if greeting[0] != 0x05 || greeting[1] != 0x00 {
		t.Fatalf("SOCKS greeting = %v", greeting)
	}

	host, portText, err := net.SplitHostPort(destination)
	if err != nil {
		t.Fatal(err)
	}
	portNumber, err := strconv.Atoi(portText)
	if err != nil || len(host) > 255 {
		t.Fatalf("forward destination = %q", destination)
	}
	request := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	request = append(request, host...)
	encodedPort := make([]byte, 2)
	binary.BigEndian.PutUint16(encodedPort, uint16(portNumber))
	request = append(request, encodedPort...)
	if _, err := connection.Write(request); err != nil {
		t.Fatal(err)
	}
	readSOCKSReply(t, connection)
	if banner := readSSHBanner(t, connection); !strings.HasPrefix(banner, "SSH-2.0-") {
		t.Fatalf("destination banner through SOCKS = %q", banner)
	}
}

func forwardDestination(t *testing.T) string {
	t.Helper()
	destination := os.Getenv(forwardDestVariable)
	if destination == "" {
		t.Skipf("%s is not set", forwardDestVariable)
	}
	if _, _, err := net.SplitHostPort(destination); err != nil {
		t.Fatalf("%s=%q is not host:port: %v", forwardDestVariable, destination, err)
	}
	return destination
}

func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort(sshclient.LoopbackHost, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func dialEventually(address string, wait time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(wait)
	var last error
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			return connection, nil
		}
		last = err
		time.Sleep(20 * time.Millisecond)
	}
	return nil, last
}

func readSOCKSReply(t *testing.T, connection net.Conn) {
	t.Helper()
	header := make([]byte, 4)
	if _, err := io.ReadFull(connection, header); err != nil {
		t.Fatal(err)
	}
	if header[0] != 0x05 || header[1] != 0x00 {
		t.Fatalf("SOCKS reply = %v", header)
	}
	addressBytes := 0
	switch header[3] {
	case 0x01:
		addressBytes = net.IPv4len
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(connection, length); err != nil {
			t.Fatal(err)
		}
		addressBytes = int(length[0])
	case 0x04:
		addressBytes = net.IPv6len
	default:
		t.Fatalf("SOCKS address type = %d", header[3])
	}
	if _, err := io.ReadFull(connection, make([]byte, addressBytes+2)); err != nil {
		t.Fatal(err)
	}
}

func readSSHBanner(t *testing.T, connection net.Conn) string {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(10 * time.Second))
	banner, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil {
		t.Fatalf("reading destination SSH banner: %v", err)
	}
	return strings.TrimSpace(banner)
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
