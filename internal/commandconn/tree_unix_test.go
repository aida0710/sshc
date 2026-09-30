//go:build !windows

package commandconn

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// useSeparateGroup は、このテストのあいだだけ、プロセスグループを分けるかを決める。
// 答えは go test を端末から走らせたかどうかに依らない。
func useSeparateGroup(t *testing.T, separate bool) {
	t.Helper()
	previous := separateGroup
	separateGroup = func() bool { return separate }
	t.Cleanup(func() { separateGroup = previous })
}

// exec しない中間のシェルの下で起きた子も、接続を閉じれば止まる。
//
// 直接の子（シェル）だけを止めると、子（aws ssm の下の session-manager-plugin の
// ようなもの）がトンネルを開いたまま残り、繋いだ回数だけ積み上がる。
func TestClosingStopsTheChildOfAShellThatDoesNotExec(t *testing.T) {
	useSeparateGroup(t, true)
	conn, err := Start(exec.Command("/bin/sh", "-c", "sleep 60 & echo $!; wait"), "sleep under sh")
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	child, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		_ = conn.Close()
		t.Fatalf("child pid = %q", line)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !processGone(child) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(child, syscall.SIGKILL)
			t.Fatalf("the child %d of the shell outlived the connection", child)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// 制御端末を持つプロセスでは、プログラムを前面のグループに残す。分けると、
// ProxyCommand が /dev/tty で尋ねたときに SIGTTIN で止まる。
func TestAProcessWithATerminalKeepsTheProgramInItsForegroundGroup(t *testing.T) {
	useSeparateGroup(t, false)
	conn, err := Start(exec.Command("sleep", "60"), "sleep 60")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if attributes := conn.process.SysProcAttr; attributes != nil && attributes.Setpgid {
		t.Fatal("the program was moved out of the terminal's foreground group")
	}
}

// processGone は、そのプロセスがもう走っていないかを返す。止めた孫は親を失い、
// init が回収するまでゾンビとして残ることがあるので、それも止まったと数える。
func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return true
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// 2 番目の欄（コマンド名）は括弧で囲まれ、空白を含みうる。状態はその後にある。
	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
	return len(fields) > 0 && fields[0] == "Z"
}
