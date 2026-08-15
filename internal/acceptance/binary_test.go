package acceptance_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// 旧版の launchd / systemd unit が使った -open=false を受け付けると、unit が
// 新しいバイナリを起こした時点で engine.lock を先に握り、デスクトップの子が
// 上がれなくなる。旧版からの移行を持たない以上、黙って常駐するより未定義の
// フラグとして直ちに拒む。
func TestLegacyOpenFlagIsRejected(t *testing.T) {
	repository := filepath.Join("..", "..")
	binary := filepath.Join(t.TempDir(), "sshc")
	build := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/sshc")
	build.Dir = repository
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build = %v\n%s", err, output)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, binary, "-open=false")
	process.Env = []string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH")}
	output, err := process.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("-open=false did not exit immediately: %v", ctx.Err())
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("-open=false exit = %v; want status 2\n%s", err, output)
	}
	if !strings.Contains(string(output), `unknown command "-open=false"`) {
		t.Fatalf("-open=false output did not explain the rejected flag:\n%s", output)
	}
}

// TestBuiltBinaryServesTheEmbeddedUIAndStopsOnSIGTERM は本物の
// artefact をビルドして実行する。
//
// このリポジトリの中で、このプロジェクトが生成した
// プログラムを実行する唯一の場所である。テストの実行自体に
// すでに必要なローカルの Go ツールチェインを使い、HOME を一時ディレクトリに向けるため、
// 本物の ~/.ssh は決して読まれない。ここはネットワークには一切触れない。
func TestBuiltBinaryServesTheEmbeddedUIAndStopsOnSIGTERM(t *testing.T) {
	repository := filepath.Join("..", "..")
	binary := filepath.Join(t.TempDir(), "sshc")

	build := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/sshc")
	build.Dir = repository
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build = %v\n%s", err, output)
	}

	embedded, err := os.ReadFile(filepath.Join(repository, "internal", "ui", "dist", "index.html"))
	if err != nil {
		t.Fatalf("the committed UI distribution is missing: %v", err)
	}
	if len(embedded) == 0 {
		t.Fatal("the committed UI distribution is empty")
	}

	home := t.TempDir()
	// acceptance harness 自身が foreground engine の所有者である。bare `sshc` は
	// desktop を起動・前面化する公開入口なので、画面のない検査は明示的な headless
	// owner として起動し、SIGTERM まで自分で寿命を管理する。
	process := exec.Command(binary, "headless")
	process.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
	stdout, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	process.Stderr = &stderr
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	// goroutine が 1 つプロセスを回収し、結果を受け取る読み手も 1 つだけである。
	// Cleanup は、body がすでに読み干したチャネルを
	// 待ってはならない。そこで body がプロセスを回収済みか
	// を確認し、そうでなければ自前のデッドラインで kill する。
	exit := make(chan error, 1)
	go func() { exit <- process.Wait() }()
	reaped := false
	t.Cleanup(func() {
		if reaped {
			return
		}
		_ = process.Process.Signal(syscall.SIGKILL)
		select {
		case <-exit:
		case <-time.After(5 * time.Second):
			t.Error("the binary did not exit after SIGKILL")
		}
	})

	lines := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(stdout)
		line, _ := reader.ReadString('\n')
		lines <- strings.TrimSpace(line)
	}()

	var announced string
	select {
	case announced = <-lines:
	case <-time.After(15 * time.Second):
		t.Fatalf("the binary printed no URL within 15s; stderr:\n%s", stderr.String())
	}
	if !strings.HasPrefix(announced, "http://127.0.0.1:") || !strings.Contains(announced, "/#bootstrap=") {
		t.Fatalf("announced URL = %q", announced)
	}
	base, fragment, _ := strings.Cut(announced, "/#bootstrap=")
	host := strings.TrimPrefix(base, "http://")
	if len(fragment) != 43 {
		t.Fatalf("bootstrap fragment length = %d, want 43", len(fragment))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = host
	request.Header.Set("Accept", "text/html")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET / = %v", err)
	}
	responseStatus := response.StatusCode
	served := readBody(t, response)
	if responseStatus != http.StatusOK {
		t.Fatalf("GET / = %d", responseStatus)
	}
	if served != string(embedded) {
		t.Fatal("the binary served something other than the UI it embedded")
	}

	// listener はループバックのみである。このマシンの
	// 経路可能なアドレス上の同じポートへの接続は受け付けてはならない。
	assertBoundToLoopbackOnly(t, host)

	bootstrap, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/api/v1/session/bootstrap", nil)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.Host = host
	bootstrap.Header.Set("Origin", base)
	bootstrap.Header.Set("Sec-Fetch-Site", "same-origin")
	bootstrap.Header.Set("X-SSHC-Bootstrap", fragment)
	exchanged, err := client.Do(bootstrap)
	if err != nil {
		t.Fatalf("bootstrap = %v", err)
	}
	exchangedStatus := exchanged.StatusCode
	readBody(t, exchanged)
	if exchangedStatus != http.StatusOK {
		t.Fatalf("bootstrap = %d", exchangedStatus)
	}

	if _, err := os.Stat(filepath.Join(home, ".ssh")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("stat of the temporary home failed: %v", err)
	}

	if err := process.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exit:
		reaped = true
		if err != nil {
			t.Fatalf("the binary exited with %v after SIGTERM; stderr:\n%s", err, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("the binary did not exit within 10s of SIGTERM; stderr:\n%s", stderr.String())
	}

	// プロセスが終了すればポートは解放されていなければならない。
	// プロセスより長生きした listener はポートを保持し続け、開いた API を漏洩させる。
	assertPortIsFree(t, host)

	if combined := stderr.String(); strings.Contains(combined, fragment) {
		t.Fatal("the binary logged the bootstrap token on standard error")
	}
	if strings.Count(announced, fragment) != 1 {
		t.Fatal("the bootstrap token appeared more than once in the announced URL")
	}
}

// assertBoundToLoopbackOnly は、バイナリが選んだ
// ポートで 127.0.0.1 以外は応答しないことを確認する。
//
// このマシンの経路可能なアドレスは、存在する場合にのみ
// 調べる。ループバック以外の IPv4 アドレスを持たない
// マシンでは証明すべきものがなく、検査は成功と報告せず黙ってスキップされる。
func assertBoundToLoopbackOnly(t testing.TB, hostPort string) {
	t.Helper()
	_, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatal(err)
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok || network.IP.IsLoopback() || network.IP.To4() == nil {
			continue
		}
		connection, err := net.DialTimeout("tcp4", net.JoinHostPort(network.IP.String(), port), 500*time.Millisecond)
		if err == nil {
			connection.Close()
			t.Fatalf("the binary accepted a connection on %s, which is not loopback", network.IP)
		}
	}
}

func assertPortIsFree(t testing.TB, hostPort string) {
	t.Helper()
	connection, err := net.DialTimeout("tcp4", hostPort, 500*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatalf("%s still accepts connections after the process exited", hostPort)
	}
}

// TestNoTestOnlyPackageReachesTheShippedBinary は、hardening
// suite を artefact の外に保つ。internal/acceptance は構造上
// test-only だが、将来ヘルパーが非テストファイルへ移されれば、それは黙って崩れる。
func TestNoTestOnlyPackageReachesTheShippedBinary(t *testing.T) {
	list := exec.Command("go", "list", "-deps", "./cmd/sshc")
	list.Dir = filepath.Join("..", "..")
	output, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("go list = %v\n%s", err, output)
	}
	seen := 0
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			seen++
		}
		switch trimmed {
		case "sshc/internal/acceptance":
			t.Error("the hardening suite is linked into the shipped binary")
		case "testing", "net/http/httptest":
			t.Errorf("%s is linked into the shipped binary", trimmed)
		}
	}
	if seen == 0 {
		t.Fatal("go list reported no dependency at all; this check is not looking at the binary")
	}
}
