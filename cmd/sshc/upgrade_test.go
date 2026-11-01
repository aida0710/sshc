package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"sshc/internal/application"
	"sshc/internal/isolatedhome"
)

// previousReleaseEnvironment は、前のリリースの sshc のパスを渡す環境変数である。CI の
// 「Upgrade from the previous release」の段が、GitHub Releases から落とし、checksums.txt で
// 確かめてから渡す（scripts/ci/previous-release.sh）。
const previousReleaseEnvironment = "SSHC_PREVIOUS_RELEASE_BINARY"

// upgradeStepLimit は、sshc エンジンが起動する、止まる、Vault を作る・開くのを待つ上限である。
// Vault の作成と解除は本物の鍵の導出（Argon2id）を通るので、遅いランナーにも余裕を見る。
const upgradeStepLimit = 90 * time.Second

// upgradeSpeedLimitKiB は、前のリリースで保存し、今のビルドで残っているかを見る設定の値である。
const upgradeSpeedLimitKiB = 2048

// 前のリリースで、マスターパスワードのある Vault と設定を持つマシンを作り、今のビルドへ上げる。
// sshc エンジンは Vault がロックされたまま起動し、ロックを解除でき、設定が残っていて、設定を変えて
// 保存できる（保存は世代バックアップを Vault の鍵で封じる道を通る）。
// 前のリリースが書いたデータで起動する道は、ほかの試験では通らない（v0.59.1 はここで止まった）。
func TestUpgradingFromThePreviousReleaseStartsUnlocksAndKeepsTheSettings(t *testing.T) {
	previous := os.Getenv(previousReleaseEnvironment)
	if previous == "" {
		t.Skipf("set %s to the previous release's sshc to run this check", previousReleaseEnvironment)
	}
	current := buildCurrentSSHC(t)
	home := t.TempDir()
	machine := upgradeMachine{
		t: t, environment: isolatedhome.Environment(home), stateDir: filepath.Join(home, ".ssh", "sshc"),
		home: home, port: unusedLocalPort(t),
	}
	password := []byte("upgrade-check-password")

	engine := machine.startEngine(previous)
	machine.runVault("create", password, password)
	machine.run(previous, "sftp", "settings", "--speed-limit", strconv.Itoa(upgradeSpeedLimitKiB))
	machine.stopEngine(previous, engine)

	engine = machine.startEngine(current)
	machine.runVault("unlock", password)
	var settings struct {
		Result sftpCLISettingsResult `json:"result"`
	}
	if err := json.Unmarshal(machine.run(current, "sftp", "settings", "--json"), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Result.SpeedLimitKiB != upgradeSpeedLimitKiB {
		t.Fatalf("speed limit after the upgrade = %g KiB/s, want %d", settings.Result.SpeedLimitKiB, upgradeSpeedLimitKiB)
	}
	machine.saveKeepScreenAwake(engine)
	machine.stopEngine(current, engine)
}

// upgradeMachine は、一時ディレクトリをホームにした1台のマシンである。
type upgradeMachine struct {
	t           *testing.T
	environment []string
	home        string
	stateDir    string
	port        int
}

// upgradeEngine は、起動した sshc エンジンのプロセスと、その出力を書くファイルである。
type upgradeEngine struct {
	exited <-chan struct{}
	log    string
}

func (machine upgradeMachine) startEngine(binary string) upgradeEngine {
	t := machine.t
	t.Helper()
	log := filepath.Join(t.TempDir(), "engine.log")
	output, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = output.Close() })
	process := exec.Command(binary, "engine", "--port", strconv.Itoa(machine.port))
	process.Env = machine.environment
	process.Stdout, process.Stderr = output, output
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		_ = process.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			_ = process.Process.Kill()
			<-exited
		}
	})

	engine := upgradeEngine{exited: exited, log: log}
	// sshc service install と同じ待ち方で待つ。止まったら待つのをやめる。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-exited:
			cancel()
		case <-ctx.Done():
		}
	}()
	if _, err := waitForReadyEngine(ctx, machine.stateDir, engineReadiness{timeout: upgradeStepLimit, client: &http.Client{}}); err != nil {
		select {
		case <-exited:
			t.Fatalf("%s stopped while starting:\n%s", filepath.Base(binary), engine.output())
		default:
			t.Fatalf("%s did not become ready: %v\n%s", filepath.Base(binary), err, engine.output())
		}
	}
	return engine
}

func (machine upgradeMachine) stopEngine(binary string, engine upgradeEngine) {
	machine.t.Helper()
	machine.run(binary, "stop")
	select {
	case <-engine.exited:
	case <-time.After(upgradeStepLimit):
		machine.t.Fatalf("%s did not exit after sshc stop:\n%s", filepath.Base(binary), engine.output())
	}
}

func (engine upgradeEngine) output() string {
	contents, err := os.ReadFile(engine.log)
	if err != nil {
		return err.Error()
	}
	return string(contents)
}

// run は、binary の CLI をこのマシンで実行し、標準出力を返す。
func (machine upgradeMachine) run(binary string, arguments ...string) []byte {
	t := machine.t
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), upgradeStepLimit)
	defer cancel()
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Env = machine.environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("%s %v = %v\nstdout:\n%s\nstderr:\n%s", filepath.Base(binary), arguments, err, stdout.String(), stderr.String())
	}
	return stdout.Bytes()
}

// runVault は、今の CLI の Vault の操作を、パスワードを打つ端末の代わりを付けて実行する。
// CLI はパスワードを端末からしか読まないので、子プロセスでは打てない。
//
// 答えは1つずつ写して渡す。CLI は読んだパスワードを使い終わると消すので、同じ領域を渡すと、
// 確認の入力を消したときに新しいパスワードも消え、Vault が 0 のパスワードで作られる。
func (machine upgradeMachine) runVault(action string, passwords ...[]byte) {
	t := machine.t
	t.Helper()
	answers := make([][]byte, len(passwords))
	for index, password := range passwords {
		answers[index] = bytes.Clone(password)
	}
	var stdout, stderr bytes.Buffer
	environment := commandEnvironment{
		home: machine.home, stateDir: machine.stateDir, client: &http.Client{},
		stdin: vaultTestInput(t), stdout: &stdout, stderr: &stderr,
		terminal: &fakePasswordTerminal{terminal: true, answers: answers},
	}
	ctx, cancel := context.WithTimeout(context.Background(), upgradeStepLimit)
	defer cancel()
	if code := runVault(ctx, action, environment); code != 0 {
		t.Fatalf("sshc vault %s = %d\nstdout:\n%s\nstderr:\n%s", action, code, stdout.String(), stderr.String())
	}
}

// saveKeepScreenAwake は、画面のターミナルのメニューと同じく、読んだ標準のプリセットを base にして
// 「画面を消さない」を入れて保存し、読み直して残っていることを確かめる。
func (machine upgradeMachine) saveKeepScreenAwake(engine upgradeEngine) {
	t := machine.t
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), upgradeStepLimit)
	defer cancel()
	api, err := openEngineAPI(ctx, machine.stateDir, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	base := machine.standardPreset(ctx, api)
	interfaceSettings := application.InterfaceSettings{}
	if base.Interface != nil {
		interfaceSettings = *base.Interface
	}
	interfaceSettings.KeepScreenAwake = true
	changed := base
	changed.Interface = &interfaceSettings
	path := "/api/v1/settings/presets/" + application.DefaultPresetID
	request := struct {
		Base   application.SettingsPreset `json:"base"`
		Preset application.SettingsPreset `json:"preset"`
	}{Base: base, Preset: changed}
	var response json.RawMessage
	if err := api.sendJSON(ctx, http.MethodPut, path, request, &response); err != nil {
		t.Fatalf("PUT %s = %v\nengine output:\n%s", path, err, engine.output())
	}
	saved := machine.standardPreset(ctx, api)
	if saved.Interface == nil || !saved.Interface.KeepScreenAwake {
		t.Fatalf("interface after saving = %#v, want keepScreenAwake", saved.Interface)
	}
}

// standardPreset は、設定の画面と同じ読み方で、標準のプリセットを返す。
func (machine upgradeMachine) standardPreset(ctx context.Context, api *engineAPI) application.SettingsPreset {
	t := machine.t
	t.Helper()
	var document application.SettingsDocument
	if err := api.getJSON(ctx, "/api/v1/settings", &document); err != nil {
		t.Fatal(err)
	}
	for _, preset := range document.Presets {
		if preset.ID == application.DefaultPresetID {
			return preset
		}
	}
	t.Fatalf("settings have no standard preset: %#v", document.Presets)
	return application.SettingsPreset{}
}

// buildCurrentSSHC は、このリポジトリの sshc を組み立てる。画面は組み立てていなくてもよい。
func buildCurrentSSHC(t *testing.T) string {
	t.Helper()
	name := "sshc"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-trimpath", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build = %v\n%s", err, output)
	}
	return binary
}

// unusedLocalPort は、いま空いている 127.0.0.1 のポートを返す。前のリリースと今のビルドの
// sshc エンジンに同じポートを渡し、ブラウザの登録と同じ条件で起動させる。
func unusedLocalPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
