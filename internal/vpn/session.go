package vpn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrSessionFailed は、コンテナが経路を用意できなかったことを表す。
	ErrSessionFailed = errors.New("the vpn session did not come up")
	// ErrSessionForeign は、同じ名前の別用途のコンテナに触れないことを表す。
	ErrSessionForeign = errors.New("a container of that name belongs to something else")
)

const (
	ownerLabel     = "io.sshc.vpn.owner"
	workspaceLabel = "io.sshc.vpn.workspace"
	profileLabel   = "io.sshc.vpn.profile"
	targetLabel    = "io.sshc.vpn.target"

	// relaySocketName は、コンテナが差し出す中継の名前である。ホスト側では
	// プロファイルごとのディレクトリの下に現れる。
	relaySocketName = "relay.sock"
	// statusFileName は、agent がトンネルの様子を書き出す先である。
	statusFileName = "status.json"
	// failureFileName は、agent が経路を用意できなかった理由の語を書き出す先である。
	failureFileName = "failure.json"
	// maxStatusBytes は、その様子を読む上限である。壊れたファイルで engine の
	// memory を埋めない。
	maxStatusBytes = 4 << 10

	// readyPollInterval は、中継のソケットが現れたかを見に行く間隔である。
	readyPollInterval = 200 * time.Millisecond
	// readyTimeout は、トンネルが成立するまで待つ上限である。IKEやDNSの
	// 遅い相手でも、この時間を超えるなら利用者へ理由を見せた方がよい。
	readyTimeout = 45 * time.Second
	// approvalReadyTimeout は、二段目の承認を電話で操作する経路で待つ上限で
	// ある。通知に気づいて、電話を開いて、承認するまでを見込む。
	approvalReadyTimeout = 2 * time.Minute
	// connectAttemptMargin は、engine が待つのをやめるより先に、コンテナが
	// 自分で諦めるための余裕である。
	//
	// 応えない相手に対して、strongSwan も openconnect も長く粘る。engine が
	// 先に打ち切ると、利用者が受け取るのは「成立しませんでした」だけで、
	// どの段階で何を待っていたのかがログに残らない。
	connectAttemptMargin = 10 * time.Second
	// stopTimeout は、コンテナへ止まる時間を与える長さである。
	stopTimeout = 10 * time.Second
	// cleanupTimeout は、呼び出し側が諦めたあとでも、コンテナを片付けきるまで
	// 待つ上限である。docker stop の猶予に、docker rm の分を足す。
	cleanupTimeout = stopTimeout + 5*time.Second
)

// socketDirectory は、このプロファイルの中継ソケットを置くホスト側の場所である。
func (manager *Manager) socketDirectory(profileName string) string {
	return filepath.Join(manager.directory, profileName)
}

func (manager *Manager) socketPath(profileName string) string {
	return filepath.Join(manager.socketDirectory(profileName), relaySocketName)
}

// start は、コンテナを一台立ち上げて中継が使えるようになるまでを行う。
//
// report は、いまどこまで進んだかを呼び出し側へ知らせる。初回はイメージの用意
// だけで分単位になることがあり、待っている人が何を待っているか分からない。
func (manager *Manager) start(ctx context.Context, profile Profile, secrets Secrets, report func(StartPhase)) error {
	// 形だけは先に見る。イメージを作ってから断るより、作る前に断る方が早い。
	if err := profile.Validate(); err != nil {
		return err
	}
	if err := profile.ValidateSecrets(secrets); err != nil {
		return err
	}
	chosen := backends[profile.Backend]
	if err := requireTunnelDevice(chosen.device()); err != nil {
		return err
	}
	report(PhaseImage)
	image, err := manager.ensureImage(ctx)
	if err != nil {
		return err
	}
	directory := manager.socketDirectory(profile.Name)
	if err := requireSocketPaths(directory); err != nil {
		return err
	}
	if err := prepareSocketDirectory(directory); err != nil {
		return err
	}
	report(PhaseContainer)
	name := manager.containerName(profile.Name)
	arguments := runArguments(containerRun{
		name: name, image: image, profile: profile, owner: manager.owner, workspace: manager.workspace,
		socketDirectory: directory, backend: chosen,
	})
	if _, err := manager.docker.output(ctx, arguments...); err != nil {
		return fmt.Errorf("%w: %w", ErrSessionFailed, err)
	}
	// 二段目のコードは 30 秒で変わる。イメージを作る時間を挟まないよう、渡す
	// 直前にこの文書を作る。
	if err := manager.configureContainer(ctx, name, profile, secrets, report); err != nil {
		// 片付けるとコンテナのログも消える。失敗の理由を読めるよう、先に
		// 秘密を伏せて残す。
		manager.state(profile.Name).keepFailureLogs(manager.containerLogs(context.WithoutCancel(ctx), name, secrets))
		// 呼び出し側が諦めた場合も片付ける。stopContainer は ctx の取り消しに
		// 引きずられない。
		manager.stopContainer(ctx, name)
		return err
	}
	return nil
}

// configureContainer は、起動したコンテナへ設定を渡し、中継が開くまで待つ。
func (manager *Manager) configureContainer(
	ctx context.Context, name string, profile Profile, secrets Secrets, report func(StartPhase),
) error {
	if err := sleepContext(ctx, secondFactorWait(profile, secrets, manager.now())); err != nil {
		return err
	}
	document, err := newAgentDocument(profile, secrets, manager.owner, manager.now())
	if err != nil {
		return err
	}
	if err := manager.sendDocument(ctx, name, document); err != nil {
		return err
	}
	report(tunnelPhase(profile))
	return manager.waitForRelay(ctx, name, profile)
}

// requireTunnelDevice は、backendが要るデバイスがこの機械にあるかを見る。
//
// 無いまま起動すると、コンテナの中の分かりにくい失敗になる。ここで断る方が、
// 利用者は何を用意すればよいかを知れる。
func requireTunnelDevice(device string) error {
	if _, err := os.Stat(device); err != nil {
		return fmt.Errorf("%w: %s", ErrTunnelDevice, device)
	}
	return nil
}

// sleepContext は、ctx が終わるまでのあいだ、長くても wait だけ待つ。
func sleepContext(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// prepareSocketDirectory は、中継ソケットを置く場所を利用者だけのものにする。
//
// 中継はTCPポートを開かない。ポートを開けば、この機械の他の利用者が誰でも
// そのVPN経路へ乗れてしまう。ソケットは0700のディレクトリの下にだけ置く。
func prepareSocketDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return err
	}
	// 前回のソケットと様子が残っていると、socatが掴めないか、止まった経路の
	// 様子や失敗の理由を今のものとして見せてしまう。
	return removeRouteFiles(directory)
}

// routeFiles は、経路ひとつがホスト側に置くファイルである。
var routeFiles = []string{statusFileName, failureFileName, relaySocketName, engineRelaySocketName}

// removeRouteFiles は、経路ひとつがホスト側に置いたファイルを消す。
func removeRouteFiles(directory string) error {
	for _, file := range routeFiles {
		if err := removeIfPresent(filepath.Join(directory, file)); err != nil {
			return err
		}
	}
	return nil
}

func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// containerRun は、コンテナひとつを起動するために決まっているものである。
type containerRun struct {
	name            string
	image           string
	profile         Profile
	owner           int
	workspace       string
	socketDirectory string
	// backend は、デバイスと権限を決める。
	backend backend
}

// runArguments は、コンテナを起動する引数である。
//
// --privileged と --network host は使わない。渡す権限は backend が要るもの、渡す
// デバイスはトンネルのものだけである。秘密は引数に載せない。
func runArguments(run containerRun) []string {
	name, image, profile, owner, socketDirectory := run.name, run.image, run.profile, run.owner, run.socketDirectory
	arguments := []string{
		"run", "--detach", "--name", name,
		"--label", ownerLabel + "=" + strconv.Itoa(owner),
		"--label", workspaceLabel + "=" + run.workspace,
		"--label", profileLabel + "=" + profile.Name,
		"--label", targetLabel + "=" + profile.Target.Address(),
		"--network", "bridge",
		"--device", run.backend.device(),
		"--security-opt", "no-new-privileges:true",
		"--restart", "no",
		// PID 1 を tini にする。sh を PID 1 にすると SIGTERM が既定で無視され、
		// docker stop は猶予を待ち切ってから SIGKILL で終わらせる。agent が
		// 合図を受けて、装置へ切断を伝えられない。
		"--init",
		// 設定と秘密が触れるのはこのtmpfsだけである。コンテナを止めれば消える。
		"--tmpfs", "/run/sshc-vpn:rw,nosuid,nodev,size=8m,mode=700",
		"--volume", socketDirectory + ":/run/sshc-vpn-socket",
		"--log-opt", "max-size=1m", "--log-opt", "max-file=1",
	}
	arguments = append(arguments, run.backend.capabilities()...)
	return append(arguments, image)
}

// sendDocument は、設定と秘密を標準入力でコンテナへ渡す。
//
// 書き途中の文書をagentが読まないよう、別の名前へ書いてから置き換える。
func (manager *Manager) sendDocument(ctx context.Context, name, document string) error {
	const script = "umask 077; cat > /run/sshc-vpn/profile.pending" +
		" && mv /run/sshc-vpn/profile.pending /run/sshc-vpn/profile.json"
	if _, err := manager.docker.outputWithInput(ctx, document, "exec", "-i", name, "sh", "-c", script); err != nil {
		return fmt.Errorf("%w: %w", ErrSessionFailed, err)
	}
	return nil
}

// waitForRelay は、中継のソケットが現れるまで待つ。
//
// ソケットが現れることが、トンネル・経路・パケットフィルタまで用意できた合図で
// ある。コンテナが先に終わったら、agent が書いた理由の語を返す。生のログは
// 返さない。IP アドレスやパスを含み、利用者へそのまま見せる形ではないからである。
func (manager *Manager) waitForRelay(ctx context.Context, name string, profile Profile) error {
	path := manager.socketPath(profile.Name)
	deadline := time.Now().Add(relayDeadline(profile))
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		running, err := manager.containerRunning(ctx, name)
		if err != nil {
			return err
		}
		if !running {
			return manager.sessionFailure(profile.Name, FailureUnknown)
		}
		if time.Now().After(deadline) {
			return manager.sessionFailure(profile.Name, FailureTimeout)
		}
		if err := sleepContext(ctx, readyPollInterval); err != nil {
			return err
		}
	}
}

// maxFailureBytes は、失敗の理由を読む上限である。
const maxFailureBytes = 1 << 10

// sessionFailure は、agent が書いた理由の語を読み、読めなければ fallback を使う。
func (manager *Manager) sessionFailure(profileName string, fallback FailureReason) error {
	failure := &SessionFailure{Profile: profileName, Reason: fallback}
	contents, err := os.ReadFile(filepath.Join(manager.socketDirectory(profileName), failureFileName))
	if err != nil || len(contents) > maxFailureBytes {
		return failure
	}
	var written struct {
		Reason FailureReason `json:"reason"`
	}
	if json.Unmarshal(contents, &written) == nil && knownFailureReasons[written.Reason] {
		failure.Reason = written.Reason
	}
	return failure
}

// tunnelPhase は、トンネルを待っているあいだ、何を待っているかを表す。
//
// 承認を待つ経路では、待っている相手は装置ではなく人である。「トンネルを
// 待っています」とだけ出ていると、利用者は電話を見に行かない。
func tunnelPhase(profile Profile) StartPhase {
	if profile.WaitsForApproval() {
		return PhaseApproval
	}
	return PhaseTunnel
}

// agentDeadline は、コンテナが相手を待つのをやめる時刻である。engine が待つ
// 長さから余裕を引いて決める。二つが離れると、失敗の理由が残らなくなる。
func agentDeadline(profile Profile, now time.Time) time.Time {
	return now.Add(relayDeadline(profile) - connectAttemptMargin)
}

// relayDeadline は、この経路が立つのを待つ長さである。
//
// 人が電話で承認する経路は、機械だけで進む経路より長くかかる。同じ長さで打ち
// 切ると、承認する前に畳んでしまう。
func relayDeadline(profile Profile) time.Duration {
	if profile.WaitsForApproval() {
		return approvalReadyTimeout
	}
	return readyTimeout
}

// containerRunning は、コンテナが動いているかを返す。コンテナが無ければ false
// を返し、docker そのものの失敗は失敗として返す。
func (manager *Manager) containerRunning(ctx context.Context, name string) (bool, error) {
	output, err := manager.docker.output(ctx, "container", "inspect", "--format", "{{.State.Running}}", name)
	if isMissingContainer(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) == "true", nil
}

// containerLogs は、利用者へ見せられる形で直近のログを返す。
func (manager *Manager) containerLogs(ctx context.Context, name string, secrets Secrets) string {
	output, err := manager.docker.combined(ctx, "logs", "--tail", "40", name)
	if err != nil {
		return ""
	}
	return redact(strings.TrimSpace(output), secrets)
}

// stopContainer は、コンテナを止めて消す。止められなくても消しに行く。残った
// コンテナの方が害が大きい。
//
// 呼び出し側の ctx が取り消されていても片付けきる。起動を途中でやめたときに
// 呼ばれるので、ctx をそのまま使うと docker を一度も呼べずに終わる。
func (manager *Manager) stopContainer(ctx context.Context, name string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if _, err := manager.docker.output(cleanup, "stop", "--time", strconv.Itoa(int(stopTimeout/time.Second)), name); err != nil {
		_, _ = manager.docker.output(cleanup, "rm", "--force", name)
		return
	}
	_, _ = manager.docker.output(cleanup, "rm", name)
}

// requireOurContainer は、その名前のコンテナが本当にこのengineのものかを確かめる。
func (manager *Manager) requireOurContainer(ctx context.Context, name, profileName string) (bool, error) {
	format := "{{index .Config.Labels \"" + ownerLabel + "\"}} {{index .Config.Labels \"" + profileLabel +
		"\"}} {{index .Config.Labels \"" + workspaceLabel + "\"}}"
	output, err := manager.docker.output(ctx, "container", "inspect", "--format", format, name)
	if isMissingContainer(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) != 3 || fields[0] != strconv.Itoa(manager.owner) || fields[1] != profileName ||
		fields[2] != manager.workspace {
		return false, fmt.Errorf("%w: %s", ErrSessionForeign, name)
	}
	return true, nil
}

// isMissingContainer は、docker の失敗が「そのコンテナは無い」だったかを返す。
//
// それ以外の失敗（daemon が応えない、など）を「無い」と読むと、無いはずの名前で
// コンテナを作りに行き、名前の衝突という分かりにくい失敗になる。
func isMissingContainer(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "No such container") || strings.Contains(message, "No such object")
}
