package vpn

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sshc/internal/connectionlog"
)

// docker は、起動する環境の PATH で探す。engine 自身の PATH では探さない。
//
// launchd が起動した engine の PATH には、Docker Desktop の /usr/local/bin が無い。
func TestDockerIsFoundInThePathItWillRunWith(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("実行の許可の bit で確かめる")
	}
	first, second := t.TempDir(), t.TempDir()
	docker := filepath.Join(second, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 実行できないファイルは飛ばす。
	if err := os.WriteFile(filepath.Join(first, "docker"), []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	variables := []string{"PATH=/nowhere", "HOME=/home/user", "PATH=relative" + string(filepath.ListSeparator) +
		first + string(filepath.ListSeparator) + second}

	found, err := lookPathIn("docker", pathVariable(variables))
	if err != nil || found != docker {
		t.Fatalf("lookPathIn = %q, %v", found, err)
	}
	if _, err := lookPathIn("docker", "/nowhere"); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("lookPathIn(無い) = %v", err)
	}
}

// トンネルのデバイスを渡せなかった docker run は、デバイスが無いことを理由にする。
func TestARunThatCouldNotPassTheDeviceSaysSo(t *testing.T) {
	refused := errors.New(`docker: Error response from daemon: error gathering device information while adding custom device "/dev/ppp": no such file or directory`)

	if err := runFailure("/dev/ppp", refused); !errors.Is(err, ErrTunnelDevice) {
		t.Fatalf("runFailure = %v", err)
	}
	if err := runFailure("/dev/ppp", errors.New("conflict")); !errors.Is(err, ErrSessionFailed) {
		t.Fatalf("runFailure = %v", err)
	}
}

// 見せるログは上限の中に収め、文字の途中で切らない。
func TestShownLogsKeepTheirNewestPartWithinTheLimit(t *testing.T) {
	text := "古い行\n新しい行"

	if got := lastBytes(text, 100); got != text {
		t.Fatalf("lastBytes = %q", got)
	}
	// 上限が「古い行」の「行」の途中に当たっても、文字の頭から始める。
	got := lastBytes(text, len("\n新しい行")+1)
	if got != "\n新しい行" {
		t.Fatalf("lastBytes = %q", got)
	}
}

// fakeDocker は、script を本体とする docker を置き、それを起動する dockerCommand を
// 返す。
func fakeDocker(t *testing.T, script string) dockerCommand {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sh の script で docker の代わりをする")
	}
	path := filepath.Join(t.TempDir(), "docker")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dockerCommand{path: path, environment: os.Environ()}
}

// コンテナが無いことは問い合わせの答えである。失敗として返さず、接続ログにも
// 失敗と書かない。
func TestAnAbsentContainerIsAnAnswerNotAFailure(t *testing.T) {
	docker := fakeDocker(t, "echo 'Error response from daemon: No such container: sshc-vpn-lab' >&2\nexit 1\n")
	var record attemptRecord
	ctx := connectionlog.With(context.Background(), &record)

	_, present, err := docker.probe(ctx, "コンテナ", "container", "inspect", "sshc-vpn-lab")

	if err != nil || present {
		t.Fatalf("probe = %v, %v", present, err)
	}
	text := record.text()
	if !strings.Contains(text, "docker container inspect sshc-vpn-lab（") || !strings.Contains(text, "）：そのコンテナはありません") ||
		strings.Contains(text, "失敗") {
		t.Fatalf("record:\n%s", text)
	}
}

// docker そのものの失敗は、問い合わせでも失敗として返し、その出力を接続ログに書く。
func TestADaemonFailureDuringAProbeIsStillAFailure(t *testing.T) {
	docker := fakeDocker(t, "echo 'Cannot connect to the Docker daemon' >&2\nexit 1\n")
	var record attemptRecord
	ctx := connectionlog.With(context.Background(), &record)

	_, _, err := docker.probe(ctx, "コンテナ", "container", "inspect", "sshc-vpn-lab")

	if err == nil {
		t.Fatal("docker の失敗を「無い」と読んだ")
	}
	text := record.text()
	if !strings.Contains(text, "は失敗しました") || !strings.Contains(text, "Cannot connect to the Docker daemon") {
		t.Fatalf("record:\n%s", text)
	}
}

// 標準出力と標準エラーは、書かれた順に合わせる。つなぎ直すと前後が入れ替わる。
func TestCombinedOutputKeepsTheOrderItWasWritten(t *testing.T) {
	docker := fakeDocker(t, "echo 1\necho 2 >&2\necho 3\n")

	got, err := docker.combined(context.Background(), "logs", "sshc-vpn-lab")

	if err != nil || got != "1\n2\n3\n" {
		t.Fatalf("combined = %q, %v", got, err)
	}
}

// トンネルを待つあいだの状態の確認は、1回ずつは接続ログに書かない。0.2 秒ごとに
// 書くと、接続ログと記録が埋まる。
func TestWaitingForTheTunnelDoesNotLogEachCheck(t *testing.T) {
	directory := t.TempDir()
	profile := Profile{Name: "lab", Backend: WireGuard}
	manager := &Manager{directory: directory}
	status := filepath.Join(manager.routeDirectory(profile.Name), statusFileName)
	if err := os.MkdirAll(filepath.Dir(status), 0o700); err != nil {
		t.Fatal(err)
	}
	// 状態を確かめられた docker が、トンネルの様子を書き出す。1回確かめて終わる。
	manager.docker = fakeDocker(t, "touch '"+status+"'\necho true\n")
	var record attemptRecord
	ctx := connectionlog.With(context.Background(), &record)

	if err := manager.waitForTunnel(ctx, "sshc-vpn-lab", profile); err != nil {
		t.Fatalf("waitForTunnel = %v", err)
	}
	if text := record.text(); strings.Contains(text, "State.Running") {
		t.Fatalf("状態の確認を書いた:\n%s", text)
	}
}

// イメージが作成済みなら、イメージの段階を通らず、知らせも出さない。作るときだけ
// 段階を知らせ、設定に関係なく出す行で知らせる。
func TestTheImagePhaseIsReportedOnlyWhenTheImageIsBuilt(t *testing.T) {
	for _, probe := range []struct {
		name    string
		script  string
		phases  string
		noticed bool
	}{
		{name: "作成済み", script: "exit 0\n", phases: "", noticed: false},
		{
			name: "作る",
			script: "if [ \"$1\" = image ] && [ \"$2\" = inspect ]; then " +
				"echo 'Error response from daemon: No such image: sshc-vpn' >&2; exit 1; fi\nexit 0\n",
			phases: "image", noticed: true,
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			manager := &Manager{docker: fakeDocker(t, probe.script)}
			var record attemptRecord
			ctx := connectionlog.With(context.Background(), &record)
			var phases []string

			if _, err := manager.ensureImage(ctx, func(phase StartPhase) { phases = append(phases, string(phase)) }); err != nil {
				t.Fatalf("ensureImage = %v", err)
			}

			if got := strings.Join(phases, ","); got != probe.phases {
				t.Errorf("phases = %q, want %q", got, probe.phases)
			}
			if noticed := strings.Contains(record.text(), "[sshc] "+PhaseImage.Notice()); noticed != probe.noticed {
				t.Errorf("知らせ = %v:\n%s", noticed, record.text())
			}
		})
	}
}

// VPN経路の起動に失敗したとき、元のエラーは経路の記録にだけ残す。接続ログには
// 接続の側（sshclient の「失敗の詳細」）が書くので、ここからも書くと2回並ぶ。
// docker build の出力は接続ログに1回だけ写し、エラーの文には要点の1行だけを残す。
func TestAFailedImageBuildIsShownOnceAndSummarizedInTheError(t *testing.T) {
	docker := fakeDocker(t, `case "$1" in
info) echo 'linux/x86_64、Docker 29、Test'; exit 0 ;;
container) echo 'Error response from daemon: No such container: x' >&2; exit 1 ;;
image) echo 'Error response from daemon: No such image: x' >&2; exit 1 ;;
build) echo '#6 9.405 E: Unable to locate package iproute2' >&2
       echo 'ERROR: failed to build: failed to solve: exit code: 100' >&2
       echo 'View build details: docker-desktop://dashboard/build' >&2; exit 1 ;;
esac
exit 0
`)
	search := filepath.Dir(docker.path)
	manager := New(shortSocketDirectory(t), 1000, func(context.Context) ([]string, error) {
		return []string{"PATH=" + search}, nil
	})
	var connectionLog attemptRecord
	ctx := connectionlog.With(context.Background(), &connectionLog)

	err := manager.Start(ctx, validProfile(), Secrets{WireGuard: &WireGuardSecrets{PrivateKey: testPrivateKey}})

	if !errors.Is(err, ErrImageBuild) {
		t.Fatalf("Start = %v", err)
	}
	if strings.Contains(err.Error(), "Unable to locate") || !strings.Contains(err.Error(), "ERROR: failed to build") {
		t.Errorf("エラーの文 = %q", err.Error())
	}
	shown := connectionLog.text()
	if count := strings.Count(shown, "Unable to locate package iproute2"); count != 1 {
		t.Errorf("docker build の出力を %d 回写した:\n%s", count, shown)
	}
	if strings.Contains(shown, "失敗の詳細") {
		t.Errorf("接続ログに失敗の詳細を書いた:\n%s", shown)
	}
	if record := manager.state("tohoku").record.text(); !strings.Contains(record, "失敗の詳細：the vpn container image could not be built: ERROR: failed to build") {
		t.Errorf("記録に失敗の詳細が無い:\n%s", record)
	}
}
