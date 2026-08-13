package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"

	"sshc/internal/application"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/storage"
)

// EngineSubcommand は、常駐そのものを起こしたり止めたりする。
//
// **これはデスクトップの外殻のための語である。** 人が打つこともできるが、
// 打つ理由はほとんど無い——`sshc` を引数なしで起こせば、必要なものは起きる。
const EngineSubcommand = "engine"

// engine のサブコマンド。
const (
	engineStart = "start"
	engineStop  = "stop"
	engineQuit  = "quit"
)

// engineReadyTimeout は、起こしたエンジンが handoff を書くまで待つ上限である。
//
// **待ち方を知っているのは Go 側だけにする。** 外殻がこれを持つと、handoff の
// 形と場所を知る場所が二つになる。
const engineReadyTimeout = 20 * time.Second

// enginePollInterval は、handoff を読み直す間隔である。
const enginePollInterval = 100 * time.Millisecond

func engineInvocation(argv []string) ([]string, bool) {
	if len(argv) >= 2 && argv[1] == EngineSubcommand {
		return argv[2:], true
	}
	return nil, false
}

// runEngineCommand は `sshc engine …` を実行する。
func runEngineCommand(
	ctx context.Context, arguments []string, home, stateDir string,
	client *http.Client, spawn func() error, stdout, stderr io.Writer,
) int {
	if len(arguments) != 1 {
		fmt.Fprintln(stderr, engineUsage)
		return 2
	}
	switch arguments[0] {
	case engineStart:
		return runEngineStart(ctx, stateDir, client, spawn, stdout, stderr)
	case engineStop:
		return runEngineStop(ctx, stateDir, client, stderr)
	case engineQuit:
		return runEngineQuit(ctx, home, stateDir, client, stderr)
	default:
		fmt.Fprintln(stderr, engineUsage)
		return 2
	}
}

const engineUsage = "sshc: engine takes start, stop or quit"

// runEngineQuit は「アプリが終了する」という意思を受ける。
//
// **止めるかどうかを決めるのは設定である。** 外殻がそれを読むと、metadata の
// 形を知る場所が二つになる——だからここで読む。読めなければ止める側に倒す。
// 動かし続けるのは明示的な選択である。
func runEngineQuit(
	ctx context.Context, home, stateDir string, client *http.Client, stderr io.Writer,
) int {
	if keepEngineRunning(home) {
		return 0
	}
	return runEngineStop(ctx, stateDir, client, stderr)
}

// keepEngineRunning は、設定が「閉じても動かし続ける」と言っているかを読む。
func keepEngineRunning(home string) bool {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		return false
	}
	metadata, _, err := application.NewMetadataStore(workspace).Load()
	if err != nil {
		return false
	}
	return metadata.KeepEngineRunning()
}

// runEngineStart は、エンジンが応答している状態にして戻る。
//
// **既に居るなら起こさない。** 二つ動くと、どちらが handoff を書いたかで
// 接続先が変わる。生死は /api/v1/health で確かめる——**あれは token も
// ゲートも要求しない唯一の経路**であり、そのために既にそうなっている。
func runEngineStart(
	ctx context.Context, stateDir string, client *http.Client,
	spawn func() error, stdout, stderr io.Writer,
) int {
	if url, alive := liveEngine(ctx, stateDir, client); alive {
		fmt.Fprintln(stdout, url)
		return 0
	}
	// 死んだ handoff は消してから起こす。残しておくと、次に読む者が
	// 誰も居ないアドレスへ向かう。
	_ = handoff.Remove(stateDir)

	if err := spawn(); err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}

	deadline := time.Now().Add(engineReadyTimeout)
	for time.Now().Before(deadline) {
		if url, alive := liveEngine(ctx, stateDir, client); alive {
			fmt.Fprintln(stdout, url)
			return 0
		}
		select {
		case <-ctx.Done():
			fmt.Fprintln(stderr, "sshc: cancelled")
			return 1
		case <-time.After(enginePollInterval):
		}
	}
	fmt.Fprintln(stderr, "sshc: the engine did not start answering")
	return 1
}

// runEngineStop は、走っているエンジンへ終了を頼む。
func runEngineStop(ctx context.Context, stateDir string, client *http.Client, stderr io.Writer) int {
	found, err := handoff.Read(stateDir)
	if err != nil {
		// 居ないことは、止める要求としては成功である。
		return 0
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, found.URL+httpserver.StopPath, bytes.NewReader([]byte("{}")))
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(handoff.HeaderName, found.Secret)

	response, err := client.Do(request)
	if err != nil {
		// 答えないものは、もう止まっている。
		return 0
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		fmt.Fprintln(stderr, "sshc: refused")
		return 1
	}
	return 0
}

// liveEngine は、handoff が指す先が答えるかを確かめる。
func liveEngine(ctx context.Context, stateDir string, client *http.Client) (string, bool) {
	found, err := handoff.Read(stateDir)
	if err != nil {
		return "", false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, found.URL+"/api/v1/health", nil)
	if err != nil {
		return "", false
	}
	response, err := client.Do(request)
	if err != nil {
		return "", false
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	if response.StatusCode != http.StatusOK {
		return "", false
	}
	return found.URL, true
}

// spawnEngine は、このバイナリを detached で起こす。
//
// **標準出力は捨てる。** あれには bootstrap トークン付きの URL が乗るので、
// ログの置き場所として不適切である（`-open=false` が既にそう決めている）。
// detached にするのは、親が死ぬときに道連れにしないためである——「閉じても
// 動かし続ける」はそれが無いと成立しない。
func spawnEngine(executable string) func() error {
	return func() error {
		command := exec.Command(executable, "-open=false")
		command.Stdout = nil
		command.Stderr = nil
		command.Stdin = nil
		detach(command)
		if err := command.Start(); err != nil {
			return err
		}
		// 待たない。**待つのは handoff が書かれることであって、この
		// プロセスの終了ではない。**
		return command.Process.Release()
	}
}
