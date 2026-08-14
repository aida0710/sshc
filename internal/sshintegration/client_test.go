// Package sshintegration は、このアプリケーションの SSH クライアントを、
// 本物の OpenSSH の sshd に対して走らせる。
//
// **単体テストは、このクライアントを Go で書かれたサーバーと比べているだけ
// である。** 同じ実装のもう半分と話しているのだから、両方が同じ勘違いをして
// いれば緑になる。B2〜B5 で `ssh(1)` を捨てて自分で SSH を話すようにした以上、
// 相手が本物の OpenSSH でも通じることを言えるのはここだけになった。
//
// アドレスが設定されていなければ丸ごとスキップするので、`go test ./...` は
// 密閉されたままである。`make integration` がコンテナで sshd を起動する。
package sshintegration_test

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

// target は、この sshd 一台分の Target である。
//
// **解決器は通さない。** ここが確かめるのは設定の読み方ではなく線の向こう側で
// あり、途中に自分のパーサーを挟むと、失敗したときにどちらの話か分からなくなる。
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

// knownHosts は、いま走っている sshd の鍵を集めて known_hosts 1 枚分にする。
//
// **集めるのもこのアプリケーションの実装である。** コンテナは起動のたびに鍵を
// 作り直すので、固定のフィクスチャは置けない——そしてそのおかげで、
// `ssh-keyscan` の代わりに書いたものが本物の sshd から鍵を取れることまで、
// 毎回ここで確かめられる。
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

// TestRunAuthenticatesWithAKeyAgainstRealSshd は、無人の実行が本物の sshd を
// 相手に通ることを言う。
//
// 鍵にはパスフレーズが掛かっている。**保存済みのパスフレーズで開けたものが、
// OpenSSH が受け取る署名になる**——鍵の読み出しと署名の両方が、ここで初めて
// 自分以外の相手に検算される。
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

// TestRunCarriesTheRemoteExitCode は、向こうの終了状態がそのまま返ることを言う。
//
// 診断はこの数字で判断する。0 に丸めてしまうと、失敗した点検が成功として並ぶ。
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

// TestRunRefusesAHostThatIsNotInKnownHosts は、無人の実行が信頼を増やさない
// ことを言う。
//
// **これは正のコントロールでもある。** 上の 2 つが緑なのは、鍵を集めて
// known_hosts を組み立てているからだと言うためには、組み立てなければ赤になる
// ことを見せなければならない。
func TestRunRefusesAHostThatIsNotInKnownHosts(t *testing.T) {
	remote := integrationServer(t)
	dialer := remote.keyDialer(t, nil)
	dialer.HostKeys = sshclient.HostKeys{}

	_, err := dialer.Run(t.Context(), remote.keyTarget(), "echo never", nil)
	if !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("error = %v, want %v", err, sshclient.ErrHostKeyUnknown)
	}
}

// TestOpenAsksForThePasswordAndRunsAShell は、対話の経路を端から端まで通す。
//
// パスワードは `Run` からは決して届かない——尋ねる相手が居ないからである。
// 届くのは端末を開いたときだけであり、その問いは端末の出力に現れ、答えは
// 端末への入力として戻る。**本物の sshd が PTY を割り当てるところまで**を、
// ここで初めて確かめる。
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

	readUntil(t, process, "Password: ")
	if _, err := io.WriteString(process, remote.password+"\n"); err != nil {
		t.Fatalf("answering the password prompt: %v", err)
	}

	// 打った行は PTY がそのまま echo して返すので、打った文字列と出力とが
	// 同じだと、**シェルが走ったのか自分の入力を見ているのかを区別できない。**
	// 引用を挟むと、線に乗るのは inter''active-canary で、返るのは
	// interactive-canary になる。後者が現れたなら、走らせたのは向こうである。
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
