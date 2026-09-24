package commandconn

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func startSleeping(t *testing.T) *Conn {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sleep の無い Windows では、ProxyCommand のテストが cmd.exe で確かめる")
	}
	conn, err := Start(exec.Command("sleep", "60"), "sleep 60")
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

// 接続が終われば、プログラムも終わる。
//
// 終わらなければ、繋いだ回数だけプロセスが増える。engine は何週間も走るので、
// それは静かに積み上がる。
func TestClosingTheConnectionReapsTheCommand(t *testing.T) {
	conn := startSleeping(t)
	if err := conn.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	// Close は待ってから返る。返った時点で ProcessState が埋まっていなければ、
	// 待たずに手を離したということである。
	if conn.process.ProcessState == nil {
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
	conn := startSleeping(t)
	defer func() { _ = conn.Close() }()

	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline = %v", err)
	}
	started := time.Now()
	_, err := conn.Read(make([]byte, 16))
	// ローカルの締め切りによる終了は deadline error として返す。
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("Read = %v, want os.ErrDeadlineExceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("the deadline took %s to arrive", elapsed)
	}
}

// 先に終わったプログラムの終わり方は、接続を閉じる前から読める。
//
// VPN の中継は、接続先へ届かなかった理由を、終わり方と標準エラーから読む。
func TestAnExitedCommandReportsHowItEndedBeforeClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh の無い Windows では確かめない")
	}
	conn, err := Start(exec.Command("sh", "-c", "echo refused >&2; exit 3"), "refuse")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("Read = %v, want EOF", err)
	}
	select {
	case <-conn.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the command never exited")
	}
	var exit *exec.ExitError
	if !errors.As(conn.ExitErr(), &exit) || exit.ExitCode() != 3 {
		t.Fatalf("ExitErr = %v, want exit status 3", conn.ExitErr())
	}
	if conn.Complaints() != "refused" {
		t.Fatalf("Complaints = %q", conn.Complaints())
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
