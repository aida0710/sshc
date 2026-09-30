//go:build unix

package integration

import (
	"bufio"
	"net"
	"syscall"
	"testing"
	"time"
)

// 止め方で終わり方が変わる。Ctrl-C はユーザーが止めたので 130、SIGTERM は
// 監督者が止めたので 0 である。この違いは、`sshc engine` を supervisor の
// 下で走らせたユーザーにとって意味を持つ。130 で終わるものを「異常終了」と読んで
// 再起動し続ける監督者は珍しくない。
//
// 同じプロセスの中では確かめられない。信号はプロセスに届くものであり、
// 関数を呼び合っても届かない。
func TestHowTheEngineIsStoppedDecidesHowItEnds(t *testing.T) {
	for _, test := range []struct {
		name   string
		signal syscall.Signal
		want   int
	}{
		{name: "Ctrl-C is a person stopping it", signal: syscall.SIGINT, want: 130},
		{name: "SIGTERM is a supervisor stopping it", signal: syscall.SIGTERM, want: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := isolatedHome(t)
			engine := start(t, home, "engine")
			waitForFile(t, handoffPath(home), 30*time.Second, engine)

			if err := engine.Command.Process.Signal(test.signal); err != nil {
				t.Fatal(err)
			}

			if code := engine.wait(t, 30*time.Second); code != test.want {
				t.Errorf("exit = %d, want %d\n%s", code, test.want, engine.Stderr.String())
			}
			// 畳んでから終わる。次の owner のために席が空いていることが、
			// 片付けが最後まで走った証拠である。
			takeOverAsHeadless(t, home)
		})
	}
}

// automatedTelnetReadyTimeout は、起動した sshc telnet が、止めたい段（送信文を送った
// あと、または接続を待っている間）に着くまで待つ上限。バイナリの起動は遅い CI では
// 数秒かかるので、それより十分長くする。
const automatedTelnetReadyTimeout = 10 * time.Second

// stoppedTelnetExitTimeout は、シグナルを送ってから sshc telnet が結果を書いて終わる
// まで待つ上限。止めたあとは接続を閉じて書くだけなので、遅い CI でも十分な長さにする。
const stoppedTelnetExitTimeout = 10 * time.Second

// telnetStopSignals は、自動処理を止める監督者と、閉じたターミナルのシグナル。
var telnetStopSignals = []struct {
	name   string
	signal syscall.Signal
}{
	{name: "SIGTERM from a supervisor", signal: syscall.SIGTERM},
	{name: "SIGHUP from a closed terminal", signal: syscall.SIGHUP},
}

// Serial／Telnet の自動処理は、SIGTERM や SIGHUP で止められても 0 で終わらない。
// 結果が出る前に止められたので、Ctrl+C と同じ 130 を返し、呼び出し側の
// スクリプトに成功と読ませない。0 で終わるのは engine と対話接続だけである。
func TestAnAutomatedTelnetRunStoppedBySignalDoesNotEndInSuccess(t *testing.T) {
	for _, test := range telnetStopSignals {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			// 送信文を受け取ったら、接続を終えて自動処理が待っている状態である。
			// 接続の途中で止めた場合は、Linux だけのテストで確かめる。
			commandReceived := make(chan net.Conn, 1)
			go func() {
				connection, err := listener.Accept()
				if err != nil {
					return
				}
				if _, err := bufio.NewReader(connection).ReadString('\n'); err != nil {
					_ = connection.Close()
					return
				}
				commandReceived <- connection
			}()

			// 期待する prompt は送らないので、自動処理は --timeout まで待ち続ける。
			run := start(t, isolatedHome(t),
				"telnet", listener.Addr().String(), "--non-interactive",
				"--connect-timeout", "5s", "--timeout", "30s", "--settle", "0",
				"--expect", `never-sent# $`, "--", "show", "version",
			)
			select {
			case connection := <-commandReceived:
				t.Cleanup(func() { _ = connection.Close() })
			case <-time.After(automatedTelnetReadyTimeout):
				t.Fatalf("sshc telnet did not send the command\n%s", run.Stderr.String())
			}

			if err := run.Command.Process.Signal(test.signal); err != nil {
				t.Fatal(err)
			}

			if code := run.wait(t, stoppedTelnetExitTimeout); code != 130 {
				t.Errorf("exit = %d, want 130\n%s", code, run.Stderr.String())
			}
		})
	}
}
