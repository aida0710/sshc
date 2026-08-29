package sshclient

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestConnectionFailureMessageTranslatesCommonContextErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "timeout", err: context.DeadlineExceeded, want: "SSH ハンドシェイクがタイムアウトしました。"},
		{name: "cancelled", err: context.Canceled, want: "SSH ハンドシェイクをキャンセルしました。"},
		{name: "detail", err: errors.New("connection reset"), want: "SSH ハンドシェイクに失敗しました：connection reset"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := connectionFailureMessage("SSH ハンドシェイク", test.err); got != test.want {
				t.Fatalf("connectionFailureMessage() = %q, want %q", got, test.want)
			}
		})
	}
}

// 接続が終われば、プログラムも終わる。
//
// 終わらなければ、繋いだ回数だけプロセスが増える。engine は何週間も走るので、
// それは静かに積み上がる。
//
// 内部から見るのは、外から「終わった」ことを確かめる portable な手が無い
// からである。プロセス表の読み方は OS ごとに違う。ここなら os/exec が
// 保持している ProcessState をそのまま読める。
func TestClosingTheTransportReapsTheCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 固有の表記は commandInterpreter のテストで検証する")
	}
	conn, err := startProxyCommand("sleep 60")
	if err != nil {
		t.Fatal(err)
	}
	command, ok := conn.(*commandConn)
	if !ok {
		t.Fatalf("startProxyCommand returned %T", conn)
	}
	if command.process.Process == nil {
		t.Fatal("プログラムが起きていない")
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	// Close は待ってから返る。返った時点で ProcessState が埋まっていなければ、
	// 待たずに手を離したということである。
	if command.process.ProcessState == nil {
		t.Fatal("Close returned before the command was reaped")
	}
	// 二度目も安全である。閉じるのは接続の側とセッションの側の両方でありうる。
	if err := conn.Close(); err != nil {
		t.Errorf("second Close = %v", err)
	}
}

// 締め切りは、どの OS でも効かなければならない。
//
// os.File の締め切りに任せると、Windows の匿名パイプはそれを支えないので、
// あちらでだけ効かない締め切りができる。効かない締め切りは、無い締め切りより
// 悪い。呼び出し側は掛けたつもりで待ち続ける。
func TestAReadDeadlineEndsTheWaitEvenOnAPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 固有の表記は commandInterpreter のテストで検証する")
	}
	conn, err := startProxyCommand("sleep 60")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline = %v", err)
	}
	started := time.Now()
	buffer := make([]byte, 16)
	_, err = conn.Read(buffer)
	if err == nil {
		t.Fatal("a command that says nothing returned bytes")
	}
	// ローカルの締め切りによる終了は deadline error として返す。
	if err != os.ErrDeadlineExceeded {
		t.Errorf("Read = %v, want os.ErrDeadlineExceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("the deadline took %s to arrive", elapsed)
	}
}

// stderr は覚えるが、覚えすぎない。
//
// 何時間も喋り続けるプログラムがあれば、それはこのプロセスのメモリになる。
func TestTheComplaintsBufferStopsAtItsLimit(t *testing.T) {
	buffer := &boundedBuffer{limit: 8}
	written, err := buffer.Write([]byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	// 捨てた分も書けたと返す。そうしないと os/exec は写しを止め、
	// プログラム側の書き込みが詰まる。
	if written != 16 {
		t.Errorf("Write = %d, want 16", written)
	}
	if buffer.String() != "01234567" {
		t.Errorf("kept %q", buffer.String())
	}
}
