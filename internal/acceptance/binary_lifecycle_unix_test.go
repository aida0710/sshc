//go:build unix

// 本物の artefact を起動し、止まるところまでを見届けるのは Unix 側だけである。
//
// Windows には SIGTERM が無い。os.Process.Signal はそこで EWINDOWS を返し、
// 行儀のよい停止を求めるにはプライベートなコンソール制御イベントと、その受け手の
// 登録が要る。engine 側のその契約は Windows Task 6 が、パッケージとしての起動・
// 停止の実証は Windows Task 9 の package smoke が持っている。ここで Kill に
// 置き換えれば、証明していないものを証明したことにしてしまう。

package acceptance_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// TestBuiltBinaryServesTheEmbeddedUIAndStopsOnSIGTERM は本物の
// artefact をビルドして実行する。
//
// このリポジトリの中で、このプロジェクトが生成した
// プログラムを実行する唯一の場所である。テストの実行自体に
// すでに必要なローカルの Go ツールチェインを使い、HOME を一時ディレクトリに向けるため、
// 本物の ~/.ssh は決して読まれない。ここはネットワークには一切触れない。
func TestBuiltBinaryServesTheEmbeddedUIAndStopsOnSIGTERM(t *testing.T) {
	repository := filepath.Join("..", "..")
	binary := filepath.Join(t.TempDir(), builtBinaryName)

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
	// bare `sshc` は接続 URL を出力するだけなので、harness が engine を明示的に起動し、
	// SIGTERM までプロセスを管理する。
	process := exec.Command(binary, "engine")
	process.Env = isolatedEnvironment(home)
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
		t.Fatalf("the binary announced nothing within 15s; stderr:\n%s", stderr.String())
	}
	// engine はアクセス URLを出さない。出せば、ワンタイムの資格情報が端末にも
	// ログにも残る。言うのは、次に何を打てばよいかだけである。
	if announced != "sshc: create the password vault with `sshc vault create`" {
		t.Fatalf("engine announcement = %q", announced)
	}
	for _, canary := range []string{"http://", "bootstrap", "127.0.0.1"} {
		if strings.Contains(announced, canary) || strings.Contains(stderr.String(), canary) {
			t.Fatalf("the engine leaked %q; stdout %q stderr:\n%s", canary, announced, stderr.String())
		}
	}

	// アクセス URLは名簿から読む。それが `sshc ssh <alias>` の通る道であり、engine が
	// 実際に受け付けていることの公開された証拠である。
	var document handoff.Handoff
	deadline := time.Now().Add(15 * time.Second)
	for {
		document, err = handoff.Read(filepath.Join(home, ".ssh", "sshc"))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no handoff was published within 15s: %v; stderr:\n%s", err, stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	base := document.URL
	if !strings.HasPrefix(base, "http://127.0.0.1:") {
		t.Fatalf("handoff URL = %q", base)
	}
	host := strings.TrimPrefix(base, "http://")

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

	// engine はブートストラップを出力しない。資格情報は名簿にあり、それを
	// 持つのは `sshc ssh <alias>` である。ここではその公開された経路を通す。
	status, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+httpserver.StatusPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	status.Host = host
	status.Header.Set(handoff.HeaderName, document.Secret)
	answered, err := client.Do(status)
	if err != nil {
		t.Fatalf("cli status = %v", err)
	}
	answeredStatus := answered.StatusCode
	readBody(t, answered)
	if answeredStatus != http.StatusOK {
		t.Fatalf("cli status = %d", answeredStatus)
	}

	// 資格情報なしでは、同じ経路が応答してはならない。
	unauthenticated, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+httpserver.StatusPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	unauthenticated.Host = host
	refused, err := client.Do(unauthenticated)
	if err != nil {
		t.Fatalf("unauthenticated cli status = %v", err)
	}
	refusedStatus := refused.StatusCode
	readBody(t, refused)
	if refusedStatus == http.StatusOK {
		t.Fatal("the command-line status answered without the handoff secret")
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

	// 名簿の秘密は、どこにも書き出されてはならない。
	if strings.Contains(stderr.String(), document.Secret) || strings.Contains(announced, document.Secret) {
		t.Fatal("the binary wrote the handoff secret to its own output")
	}
}

// assertBoundToLoopbackOnly は、バイナリが選んだ
// ポートで 127.0.0.1 以外は応答しないことを確認する。
//
// このマシンの経路可能なアドレスは、存在する場合にのみ
// 調べる。ループバック以外の IPv4 アドレスを持たない
// マシンでは証明すべきものがなく、検査は成功と報告せず暗黙にスキップされる。
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
