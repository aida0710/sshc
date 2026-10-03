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
// 待ちが終わるのを待たずに、待っているコマンドへ SIGINT を送り（openconnect は装置から
// ログオフする）、終わってから相手へ切断を伝えて終わる。
func TestStoppingWhileWaitingForTheServerDoesNotWaitForTheStep(t *testing.T) {
	if _, err := os.Stat("/proc/uptime"); err != nil {
		t.Skip("/proc/uptime が無い（コンテナの中は Linux）")
	}
	const promptly = 3 * time.Second
	fakeOpenConnect, environment := fakeOpenConnectProgram(t)
	script := agentFunctions(t, "seconds_since_boot", "wait_for_step", "start_shutdown_budget",
		"shutdown_seconds_left", "signal_waiting_step", "stop_waiting_step", "shutdown") + `
runtime=$1
fake_openconnect=$2
shutdown_seconds=5
step_interrupt_seconds=2
waiting_pid=
backend_down() { echo backend_down; }
trap shutdown TERM INT
# 承認待ちの openconnect。backend-openconnect.sh と同じく timeout の下で起動する。
timeout 30 "$fake_openconnect" &
echo waiting
wait_for_step $!
echo not-reached
`
	took, text := stopWhileWaiting(t, waitingAgent{
		script:      script,
		arguments:   []string{t.TempDir(), fakeOpenConnect},
		environment: environment,
		readyLines:  []string{"waiting\n", "openconnect-ready\n"},
	})

	if took > promptly {
		t.Fatalf("止める合図から終わるまで %v かかった（上限 %v）:\n%s", took, promptly, text)
	}
	if !strings.Contains(text, "logged-off\n") || !strings.Contains(text, "backend_down\n") ||
		strings.Contains(text, "not-reached") {
		t.Fatalf("output:\n%s", text)
	}
}

// SIGINT では止まらない、相手を待っているコマンドは、step_interrupt_seconds のあとに
// SIGTERM で止める。包んでいる timeout が合図を下のコマンドへ渡さなくても（uutils の
// timeout は最初の合図しか渡さない）、timeout の process group ごと送るので届く。
func TestStoppingWhileWaitingTerminatesAStepThatIgnoresSIGINT(t *testing.T) {
	if _, err := os.Stat("/proc/uptime"); err != nil {
		t.Skip("/proc/uptime が無い（コンテナの中は Linux）")
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid が無い")
	}
	const promptly = 3 * time.Second
	script := agentFunctions(t, "seconds_since_boot", "wait_for_step", "start_shutdown_budget",
		"shutdown_seconds_left", "signal_waiting_step", "stop_waiting_step", "shutdown") + `
runtime=$1
shutdown_seconds=5
step_interrupt_seconds=1
waiting_pid=
backend_down() { echo backend_down; }
trap shutdown TERM INT
# SIGINT では止まらないコマンド（uutils の timeout の下の getent や ipsec up）を、合図を
# 下へ渡さないプロセスで包む。包むプロセスは timeout と同じく自分の process group を作る。
# setsid は、process group の先頭でないプロセスからは fork せずに起動するので、$! が包む
# プロセスになる。
setsid sh -c 'trap : TERM; sh -c "$1" & while kill -0 $! 2>/dev/null; do wait $!; done' wrapper \
	'trap "" INT; trap "echo terminated; exit 1" TERM; echo step-ready; sleep 30 & wait' &
echo waiting
wait_for_step $!
echo not-reached
`
	took, text := stopWhileWaiting(t, waitingAgent{
		script:     script,
		arguments:  []string{t.TempDir()},
		readyLines: []string{"waiting\n", "step-ready\n"},
	})

	if took > promptly {
		t.Fatalf("止める合図から終わるまで %v かかった（上限 %v）:\n%s", took, promptly, text)
	}
	if !strings.Contains(text, "terminated\n") || !strings.Contains(text, "backend_down\n") ||
		strings.Contains(text, "not-reached") {
		t.Fatalf("output:\n%s", text)
	}
}

// waitingAgent は、相手を待っている最中の agent の代わりに起動する script である。
type waitingAgent struct {
	script string
	// arguments は、script の $1 から順に渡す。
	arguments []string
	// environment が空なら、検査の環境変数をそのまま渡す。
	environment []string
	// readyLines は、出力に現れたら待ち始めたとみなす行である。
	readyLines []string
}

// stopWhileWaiting は、agent を起動し、待ち始めたら止める合図（SIGTERM）を送る。合図を
// 送ってから終わるまでにかかった時間と、出力を返す。
func stopWhileWaiting(t *testing.T, agent waitingAgent) (time.Duration, string) {
	t.Helper()
	// 合図が届かずに残ったプロセスが出力を握っていても、これより長くは待たない。
	const outputDrainLimit = time.Second
	// wait_for_step が待ち始めるまでの猶予。待ち始めたことは出力からは分からない。
	const waitStartGrace = 300 * time.Millisecond
	command := exec.Command("sh", append([]string{"-c", agent.script, "agent-test"}, agent.arguments...)...)
	command.Env = agent.environment
	var output lockedBuffer
	command.Stdout, command.Stderr = &output, &output
	command.WaitDelay = outputDrainLimit
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	for _, line := range agent.readyLines {
		waitForOutput(t, &output, line)
	}
	time.Sleep(waitStartGrace)

	started := time.Now()
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	return time.Since(started), output.String()
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
