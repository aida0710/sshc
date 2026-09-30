package vpn

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/connectionlog"
)

// docker は、起動する環境の PATH で探す。engine 自身の PATH では探さない。
//
// launchd が起動した engine の PATH には、Docker Desktop の /usr/local/bin が無い。
func TestDockerIsFoundInThePathItWillRunWith(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	docker := filepath.Join(second, "docker")
	if runtime.GOOS == "windows" {
		// Windows では拡張子で実行できるかが決まる。docker.exe を探す。
		docker += ".exe"
	}
	if err := os.WriteFile(docker, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 実行できないファイルは飛ばす。Windows では拡張子の無い docker、ほかでは実行の
	// 許可の bit の無い docker が、実行できないファイルである。
	if err := os.WriteFile(filepath.Join(first, "docker"), []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := "relative" + string(filepath.ListSeparator) + first + string(filepath.ListSeparator) + second

	found, err := lookPathIn("docker", path)
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
	if err := runFailure("/dev/ppp", errors.New("conflict")); !errors.Is(err, ErrRouteFailed) {
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
// 返す。sh の script なので、Windows では検査を飛ばす。決まった出力を書くだけで
// よい検査は、Windows でも走る fakeDockerProgram を使う。
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
	docker := fakeDockerProgram(t, fakeDockerReply{
		Writes:   []fakeDockerWrite{{Stderr: true, Text: "Error response from daemon: No such container: sshc-vpn-lab\n"}},
		ExitCode: 1,
	})
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
	docker := fakeDockerProgram(t, fakeDockerReply{
		Writes:   []fakeDockerWrite{{Stderr: true, Text: "Cannot connect to the Docker daemon\n"}},
		ExitCode: 1,
	})
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
	docker := fakeDockerProgram(t, fakeDockerReply{Writes: []fakeDockerWrite{
		{Text: "1\n"}, {Stderr: true, Text: "2\n"}, {Text: "3\n"},
	}})

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
	manager := &Manager{directory: directory, now: time.Now}
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
			manager := &Manager{docker: fakeDocker(t, probe.script), imageName: defaultImageName}
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
// docker build の出力の最後の行は1回だけ写し、エラーの文には要点の1行だけを残す。
// （-vvv で見ている Terminal には、これとは別に途中の出力も流れる。）
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

	err := manager.Start(ctx, validProfile().Name, fixedRoute(validProfile(), validSecrets()))

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

// shownLines は、その場で接続を見ている書き先（Terminal や CLI）の代わりである。
// 途中の出力も受け取る。
type shownLines struct {
	mutex sync.Mutex
	lines []string
}

func (shown *shownLines) Enabled(connectionlog.Level) bool { return true }

func (shown *shownLines) Write(_ connectionlog.Level, message string) {
	shown.mutex.Lock()
	defer shown.mutex.Unlock()
	shown.lines = append(shown.lines, message)
}

// イメージを作るあいだ、docker build の行を書かれるたびに -vvv の接続ログへ流す。
// 経路の記録には流さない。数百行になり、記録の上限から準備の行を押し出す。
func TestTheImageBuildIsShownLineByLineButNotRecorded(t *testing.T) {
	docker := fakeDocker(t, `case "$1" in
image) echo 'Error response from daemon: No such image: x' >&2; exit 1 ;;
build) echo '#6 0.676 Get:1 http://archive.example noble InRelease' >&2
       echo '#6 9.405 Setting up iproute2' >&2
       printf 'writing image sha256:0123' ;;
esac
exit 0
`)
	manager := &Manager{docker: docker, routes: map[string]*routeState{}}
	var shown shownLines
	ctx := manager.recording(connectionlog.With(context.Background(), &shown), "tohoku")

	if _, err := manager.ensureImage(ctx, func(StartPhase) {}); err != nil {
		t.Fatalf("ensureImage = %v", err)
	}

	text := strings.Join(shown.lines, "\n")
	for _, want := range []string{"docker buildの出力：", "  #6 0.676 Get:1 http://archive.example noble InRelease", "  #6 9.405 Setting up iproute2", "  writing image sha256:0123"} {
		if !strings.Contains(text, want) {
			t.Errorf("接続ログに %q が無い:\n%s", want, text)
		}
	}
	if record := manager.state("tohoku").record.text(); strings.Contains(record, "Setting up iproute2") {
		t.Errorf("記録に docker build の行を残した:\n%s", record)
	}
}

// 経路の起動を取り消すと、docker が起動した子プロセス（docker build の docker-buildx の
// ような）が出力を持っていても、待たずに戻る。
func TestCancellingDockerDoesNotWaitForItsChildren(t *testing.T) {
	const promptly = 5 * time.Second
	// 子を起動したら印を置く。印より前に取り消すと、子の無いまま戻り、確かめたいことを
	// 確かめずに通ってしまう。
	childStarted := filepath.Join(t.TempDir(), "child-started")
	docker := fakeDocker(t, "sleep 60 &\ntouch '"+childStarted+"'\nsleep 60\n")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := docker.output(ctx, "build", "--tag", "sshc-vpn:test", ".")
		done <- err
	}()
	waitFor(t, "偽の docker が子を起動する", fileExists(childStarted))

	started := time.Now()
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("取り消したのに成功した")
		}
		if took := time.Since(started); took > promptly {
			t.Fatalf("取り消してから戻るまで %v かかった", took)
		}
	case <-time.After(dockerWaitDelay + promptly):
		t.Fatal("取り消しても戻らない")
	}
}

// docker logs が出力のあとで失敗しても、接続ログと経路の記録にシークレットを書かない。
// 読めた行は伏せてから返す。
func TestContainerLogsThatFailMidwayAreRedacted(t *testing.T) {
	const secretPassword = "hunter2-secret"
	secrets := Secrets{L2TP: &L2TPSecrets{Password: secretPassword, PreSharedKey: "psk-secret-value"}}
	for _, failing := range []struct {
		name    string
		script  string
		timeout time.Duration
	}{
		{name: "0以外で終わる", script: "echo 'line password=" + secretPassword + "'\nexit 1\n", timeout: 5 * time.Second},
		{name: "上限で打ち切られる", script: "echo 'line password=" + secretPassword + "'\nsleep 30\n", timeout: 300 * time.Millisecond},
	} {
		t.Run(failing.name, func(t *testing.T) {
			manager := &Manager{docker: fakeDocker(t, failing.script)}
			var record attemptRecord
			ctx, cancel := context.WithTimeout(connectionlog.With(context.Background(), &record), failing.timeout)
			defer cancel()

			logs := manager.containerLogs(ctx, "sshc-vpn-lab", secrets)

			if strings.Contains(record.text(), secretPassword) {
				t.Fatalf("記録にシークレットが出た:\n%s", record.text())
			}
			if !strings.Contains(record.text(), "コンテナのログを最後まで読めませんでした") {
				t.Fatalf("読めなかったことを書いていない:\n%s", record.text())
			}
			if logs != "line password="+redactedMark {
				t.Fatalf("containerLogs = %q", logs)
			}
		})
	}
}
