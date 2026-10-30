//go:build !windows

package process

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// lateChildStopsWithin は、最初の SIGKILL のあとでグループに入った子が止まるまでの上限である。
// 送り直しの間隔（10ms）に比べて十分長く、止め直さなければ sleep 60 は終わらない。
const lateChildStopsWithin = 5 * time.Second

// 最初の SIGKILL のあとでグループに入った子も、グループが空になるまで止め直す。macOS で fork の
// 途中だった子は最初の SIGKILL を受けずに残る。その子と同じ立場を、止めたあとでグループへ
// 入れて作る。
func TestAChildThatJoinedTheGroupAfterTheFirstKillIsStoppedToo(t *testing.T) {
	leader := exec.Command("sleep", "60")
	leader.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	if err := signalGroup(leader.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	// 先頭はまだ回収していないので、グループは残っている。
	late := exec.Command("sleep", "60")
	late.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: leader.Process.Pid}
	if err := late.Start(); err != nil {
		_ = leader.Wait()
		t.Skipf("this OS does not let a process join the group of a leader that was killed: %v", err)
	}

	retried := make(chan struct{})
	go func() {
		killUntilGroupIsGone(leader.Process.Pid, time.Now().Add(lateChildStopsWithin))
		close(retried)
	}()
	_ = leader.Wait()
	exited := make(chan struct{})
	go func() {
		_ = late.Wait()
		close(exited)
	}()

	select {
	case <-exited:
	case <-time.After(lateChildStopsWithin):
		_ = late.Process.Kill()
		<-exited
		t.Fatal("the child that joined the group after the first SIGKILL is still running")
	}
	<-retried
}
