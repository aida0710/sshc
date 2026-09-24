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
