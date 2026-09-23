package vpn

import (
	"context"
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
	ownerLabel   = "io.sshc.vpn.owner"
	profileLabel = "io.sshc.vpn.profile"
	targetLabel  = "io.sshc.vpn.target"

	// relaySocketName は、コンテナが差し出す中継の名前である。ホスト側では
	// プロファイルごとのディレクトリの下に現れる。
	relaySocketName = "relay.sock"
	// statusFileName は、agent がトンネルの様子を書き出す先である。
	statusFileName = "status.json"
	// maxStatusBytes は、その様子を読む上限である。壊れたファイルで engine の
	// memory を埋めない。
	maxStatusBytes = 4 << 10

	// readyPollInterval は、中継のソケットが現れたかを見に行く間隔である。
	readyPollInterval = 200 * time.Millisecond
	// readyTimeout は、トンネルが成立するまで待つ上限である。IKEやDNSの
	// 遅い相手でも、この時間を超えるなら利用者へ理由を見せた方がよい。
	readyTimeout = 45 * time.Second
	// stopTimeout は、コンテナへ止まる時間を与える長さである。
	stopTimeout = 10 * time.Second
)

// containerName は、この利用者のこのプロファイルのコンテナ名である。
//
// uidを含める。同じ機械の別の利用者のコンテナを、名前だけで掴まないためである。
func containerName(profileName string, owner int) string {
	return "sshc-vpn-" + profileName + "-" + strconv.Itoa(owner)
}

// socketDirectory は、このプロファイルの中継ソケットを置くホスト側の場所である。
func (manager *Manager) socketDirectory(profileName string) string {
	return filepath.Join(manager.directory, profileName)
}

func (manager *Manager) socketPath(profileName string) string {
	return filepath.Join(manager.socketDirectory(profileName), relaySocketName)
}

// start は、このプロファイルのコンテナを起動し、中継が開くまで待つ。
func (manager *Manager) start(ctx context.Context, profile Profile, secrets Secrets) error {
	document, err := newAgentDocument(profile, secrets, manager.owner)
	if err != nil {
		return err
	}
	if err := requireTunnelDevice(profile.Backend); err != nil {
		return err
	}
	device, err := tunnelDevice(profile.Backend)
	if err != nil {
		return err
	}
	image, err := manager.ensureImage(ctx)
	if err != nil {
		return err
	}
	directory := manager.socketDirectory(profile.Name)
	if err := prepareSocketDirectory(directory); err != nil {
		return err
	}
	name := containerName(profile.Name, manager.owner)
	arguments := runArguments(containerRun{
		name: name, image: image, profile: profile, owner: manager.owner,
		socketDirectory: directory, device: device,
	})
	if _, err := manager.docker.output(ctx, arguments...); err != nil {
		return fmt.Errorf("%w: %w", ErrSessionFailed, err)
	}
	if err := manager.sendDocument(ctx, name, document); err != nil {
		_ = manager.stopContainer(ctx, name)
		return err
	}
	if err := manager.waitForRelay(ctx, name, profile, secrets); err != nil {
		_ = manager.stopContainer(ctx, name)
		return err
	}
	return nil
}

// tunnelDevice は、backendが要るデバイスである。
func tunnelDevice(backend BackendName) (string, error) {
	switch backend {
	case WireGuard:
		return "/dev/net/tun", nil
	case L2TPIPsec:
		return "/dev/ppp", nil
	}
	return "", fmt.Errorf("%w: %s", ErrBackend, backend)
}

// requireTunnelDevice は、backendが要るデバイスがこの機械にあるかを見る。
//
// 無いまま起動すると、コンテナの中の分かりにくい失敗になる。ここで断る方が、
// 利用者は何を用意すればよいかを知れる。
func requireTunnelDevice(backend BackendName) error {
	device, err := tunnelDevice(backend)
	if err != nil {
		return err
	}
	if _, err := os.Stat(device); err != nil {
		return fmt.Errorf("%w: %s がありません", ErrTunnelDevice, device)
	}
	return nil
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
	// 様子を今のものとして見せてしまう。
	if err := removeIfPresent(filepath.Join(directory, statusFileName)); err != nil {
		return err
	}
	return removeIfPresent(filepath.Join(directory, relaySocketName))
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
	socketDirectory string
	// device は、この backend が要るトンネルのデバイスである。
	device string
}

// runArguments は、コンテナを起動する引数である。
//
// --privileged と --network host は使わない。渡す権限は CAP_NET_ADMIN、渡す
// デバイスはトンネルのものだけである。秘密は引数に載せない。
func runArguments(run containerRun) []string {
	name, image, profile, owner, socketDirectory := run.name, run.image, run.profile, run.owner, run.socketDirectory
	return []string{
		"run", "--detach", "--name", name,
		"--label", ownerLabel + "=" + strconv.Itoa(owner),
		"--label", profileLabel + "=" + profile.Name,
		"--label", targetLabel + "=" + profile.Target.Address(),
		"--network", "bridge",
		"--cap-add", "NET_ADMIN",
		"--device", run.device,
		"--security-opt", "no-new-privileges:true",
		"--restart", "no",
		// 設定と秘密が触れるのはこのtmpfsだけである。コンテナを止めれば消える。
		"--tmpfs", "/run/sshc-vpn:rw,nosuid,nodev,size=8m,mode=700",
		"--volume", socketDirectory + ":/run/sshc-vpn-socket",
		"--log-opt", "max-size=1m", "--log-opt", "max-file=1",
		image,
	}
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
// ある。コンテナが先に終わったら、その理由を秘密を伏せて返す。
func (manager *Manager) waitForRelay(ctx context.Context, name string, profile Profile, secrets Secrets) error {
	path := manager.socketPath(profile.Name)
	deadline := time.Now().Add(readyTimeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		running, err := manager.containerRunning(ctx, name)
		if err != nil {
			return err
		}
		if !running {
			return fmt.Errorf("%w: %s", ErrSessionFailed, manager.containerLogs(ctx, name, secrets))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %s の中で経路が成立しませんでした。%s",
				ErrSessionFailed, profile.Name, manager.containerLogs(ctx, name, secrets))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyPollInterval):
		}
	}
}

func (manager *Manager) containerRunning(ctx context.Context, name string) (bool, error) {
	output, err := manager.docker.output(ctx, "container", "inspect", "--format", "{{.State.Running}}", name)
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(output) == "true", nil
}

// containerLogs は、利用者へ見せられる形で直近のログを返す。
func (manager *Manager) containerLogs(ctx context.Context, name string, secrets Secrets) string {
	output, err := manager.docker.output(ctx, "logs", "--tail", "40", name)
	if err != nil {
		return ""
	}
	return redact(strings.TrimSpace(output), secrets)
}

func (manager *Manager) stopContainer(ctx context.Context, name string) error {
	if _, err := manager.docker.output(ctx, "stop", "--time", strconv.Itoa(int(stopTimeout/time.Second)), name); err != nil {
		// 止められなくても消しに行く。残ったコンテナの方が害が大きい。
		_, _ = manager.docker.output(ctx, "rm", "--force", name)
		return nil
	}
	_, _ = manager.docker.output(ctx, "rm", name)
	return nil
}

// requireOurContainer は、その名前のコンテナが本当にこのengineのものかを確かめる。
func (manager *Manager) requireOurContainer(ctx context.Context, name, profileName string) (bool, error) {
	format := "{{index .Config.Labels \"" + ownerLabel + "\"}} {{index .Config.Labels \"" + profileLabel + "\"}}"
	output, err := manager.docker.output(ctx, "container", "inspect", "--format", format, name)
	if err != nil {
		return false, nil
	}
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) != 2 || fields[0] != strconv.Itoa(manager.owner) || fields[1] != profileName {
		return false, fmt.Errorf("%w: %s", ErrSessionForeign, name)
	}
	return true, nil
}
