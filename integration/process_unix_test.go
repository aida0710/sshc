//go:build unix

package integration

import (
	"os/exec"
	"strconv"
	"testing"
)

// processCommandLine と processEnvironment は、走っているプロセスから外から
// 読めるものを返す。
//
// 読めなかったことを「見えなかった」にしない。読めなければ検査は
// 何も確かめておらず、それは通過ではない。
func processCommandLine(t *testing.T, pid int) string {
	t.Helper()
	output, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Fatalf("read the command line of %d: %v", pid, err)
	}
	return string(output)
}

// 環境変数は、この OS では別のユーザーのプロセスからは読めないのが普通である
// （Linux の /proc/<pid>/environ は同じ利用者に限られ、macOS では ps eww も
// 別のユーザーには出ない）。自分が起動した子なので読める。読めなければ、
// 確かめられなかったことを言う。
func processEnvironment(t *testing.T, pid int) string {
	t.Helper()
	if contents, err := readProcEnviron(pid); err == nil {
		return contents
	}
	output, err := exec.Command("ps", "-o", "command=", "-E", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Skipf("this host does not expose the environment of process %d", pid)
	}
	return string(output)
}
