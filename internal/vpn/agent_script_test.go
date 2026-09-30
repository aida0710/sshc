package vpn

import (
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// agentFunctions は、container/agent.sh から、名前を挙げた関数の定義を取り出す。
// agent.sh は読み込むと経路を張り始めるので、確かめたい関数だけを別の script に並べる。
func agentFunctions(t *testing.T, names ...string) string {
	t.Helper()
	contents, err := container.ReadFile("container/agent.sh")
	if err != nil {
		t.Fatal(err)
	}
	var definitions []string
	for _, name := range names {
		definition := regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(name) + `\(\) \{\n.*?^\}\n`).Find(contents)
		if definition == nil {
			t.Fatalf("agent.sh に %s が無い", name)
		}
		definitions = append(definitions, string(definition))
	}
	return strings.Join(definitions, "\n")
}

// 締め切りまでの残りは、コンテナの壁時計がずれても変わらない。Docker Desktop などの
// VM の時計は、スリープ明けにホストとずれる。
func TestTheAgentDeadlineIgnoresTheWallClock(t *testing.T) {
	if _, err := os.Stat("/proc/uptime"); err != nil {
		t.Skip("/proc/uptime が無い（コンテナの中は Linux）")
	}
	const attemptSeconds = 35
	script := agentFunctions(t, "seconds_since_boot", "remaining_seconds") + `
# 壁時計が大きく進んでいる（または遅れている）ことにする。
date() { echo 9999999999; }
deadline=$(($(seconds_since_boot) + ` + strconv.Itoa(attemptSeconds) + `))
remaining_seconds
`
	output := strings.TrimSpace(runBackendScript(t, script, t.TempDir()))

	remaining, err := strconv.Atoi(output)
	if err != nil {
		t.Fatalf("output = %q", output)
	}
	if remaining < attemptSeconds-1 || remaining > attemptSeconds {
		t.Fatalf("残り = %d 秒, want %d 秒ほど", remaining, attemptSeconds)
	}
}

// 相手を待っている最中（承認待ちや、応えない相手への再送）に止める合図を受けても、
// 待ちが終わるのを待たずに、待っているコマンドを止めてから相手へ切断を伝えて終わる。
func TestStoppingWhileWaitingForTheServerDoesNotWaitForTheStep(t *testing.T) {
	if _, err := os.Stat("/proc/uptime"); err != nil {
		t.Skip("/proc/uptime が無い（コンテナの中は Linux）")
	}
	const promptly = 3 * time.Second
	script := agentFunctions(t, "seconds_since_boot", "wait_for_step", "start_shutdown_budget",
		"shutdown_seconds_left", "stop_waiting_step", "shutdown") + `
runtime=$1
shutdown_seconds=5
step_interrupt_seconds=2
waiting_pid=
backend_down() { echo backend_down; }
trap shutdown TERM INT
# 承認待ちのように長く待つコマンド。SIGINT を受けると後始末をしてから終わる。
timeout 30 sh -c 'trap "echo logged-off; exit 1" INT; while :; do sleep 0.1; done' &
echo waiting
wait_for_step $!
echo not-reached
`
	agent := exec.Command("sh", "-c", script, "agent-test", t.TempDir())
	var output lockedBuffer
	agent.Stdout, agent.Stderr = &output, &output
	if err := agent.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Process.Kill() })
	waitForOutput(t, &output, "waiting\n")
	// wait_for_step が待ち始めるまで待つ。
	time.Sleep(300 * time.Millisecond)

	started := time.Now()
	if err := agent.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = agent.Wait()

	if took := time.Since(started); took > promptly {
		t.Fatalf("止める合図から終わるまで %v かかった（上限 %v）:\n%s", took, promptly, output.String())
	}
	text := output.String()
	if !strings.Contains(text, "logged-off\n") || !strings.Contains(text, "backend_down\n") ||
		strings.Contains(text, "not-reached") {
		t.Fatalf("output:\n%s", text)
	}
}

// lockedBuffer は、別の goroutine から書かれる出力を読めるようにする。
type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (buffer *lockedBuffer) Write(payload []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.buffer.Write(payload)
}

func (buffer *lockedBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.buffer.String()
}

// waitForOutput は、output に want が現れるまで待つ。
func waitForOutput(t *testing.T, output *lockedBuffer, want string) {
	t.Helper()
	const patience = 5 * time.Second
	deadline := time.Now().Add(patience)
	for !strings.Contains(output.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("%q が出ない:\n%s", want, output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
