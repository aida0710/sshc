package vpn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// 経路のいまの様子を読む。どれも経路を変えない。

// Status は、プロファイルひとつの状態である。
type Status struct {
	Name string
	// Running は、コンテナが動いていることを表す。
	Running bool
	// RelaySocket は、engine が差し出している中継のソケットの場所である。
	// 経路が使えるときだけ値がある。
	RelaySocket string
	// Tunnel は、コンテナの中のトンネルの様子である。経路が無ければゼロ値。
	Tunnel TunnelStatus
	// Phase は、いま経路を用意している段階である。用意していなければ空。
	Phase StartPhase
}

// TunnelStatus は、コンテナの agent が書き出したトンネルの様子である。
//
// 秘密は含まない。含めてよいのは、画面へ出して困らないものだけである。
type TunnelStatus struct {
	// Interface は、トンネルの network interface の名前である。
	Interface string `json:"interface"`
	// Address は、トンネル側でこの端末が名乗っているアドレスである。L2TP では
	// 相手から受け取った値になる。
	Address string `json:"address"`
	// Since は、経路が用意できた時刻である。
	Since string `json:"since"`
	// Backend は、そのとき使った方式である。
	Backend string `json:"backend"`
}

// Status は、このプロファイルの状態を返す。
func (manager *Manager) Status(ctx context.Context, profileName string) (Status, error) {
	if err := validateProfileName(profileName); err != nil {
		return Status{}, err
	}
	statuses, err := manager.Statuses(ctx)
	if err != nil {
		return Status{}, err
	}
	if status, present := statuses[profileName]; present {
		return status, nil
	}
	return Status{Name: profileName, Phase: manager.state(profileName).currentPhase()}, nil
}

// Statuses は、この engine のコンテナを持つ経路と、用意の途中の経路の状態を、
// プロファイル名ごとに返す。
//
// docker は1回だけ呼ぶ。画面は用意の途中に一覧を読み直すので、経路ごとに
// docker を呼ぶと、経路の数だけ応答が遅れる。
func (manager *Manager) Statuses(ctx context.Context) (map[string]Status, error) {
	if _, err := manager.command(ctx); err != nil {
		return nil, err
	}
	format := "{{.Label \"" + profileLabel + "\"}}\t{{.State}}"
	output, err := manager.docker.output(ctx, "ps", "--all", "--format", format,
		"--filter", "label="+ownerLabel+"="+strconv.Itoa(manager.owner),
		"--filter", "label="+workspaceLabel+"="+manager.workspace)
	if err != nil {
		return nil, err
	}
	statuses := map[string]Status{}
	for _, name := range manager.names() {
		if phase := manager.state(name).currentPhase(); phase != "" {
			statuses[name] = Status{Name: name, Phase: phase}
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 || validateProfileName(fields[0]) != nil {
			continue
		}
		statuses[fields[0]] = manager.containerStatus(fields[0], fields[1] == "running")
	}
	return statuses, nil
}

// containerStatus は、コンテナがあるプロファイルひとつの状態を組み立てる。
func (manager *Manager) containerStatus(profileName string, running bool) Status {
	state := manager.state(profileName)
	status := Status{Name: profileName, Running: running, Phase: state.currentPhase()}
	// コンテナが終わっても、ホスト側にはソケットのファイルが残る。動いている
	// コンテナと engine の中継が揃っているときだけ、使える経路として見せる。
	if running {
		status.RelaySocket = state.relaySocket()
	}
	if status.RelaySocket != "" {
		status.Tunnel = manager.tunnelStatus(profileName)
	}
	return status
}

// tunnelStatus は、agent が書き出した様子を読む。読めなければゼロ値を返す。
// 状態が読めないことは失敗ではない。経路があることは中継のソケットが示している。
func (manager *Manager) tunnelStatus(profileName string) TunnelStatus {
	contents, err := os.ReadFile(filepath.Join(manager.routeDirectory(profileName), statusFileName))
	if err != nil || len(contents) > maxStatusBytes {
		return TunnelStatus{}
	}
	var tunnel TunnelStatus
	if err := json.Unmarshal(contents, &tunnel); err != nil {
		return TunnelStatus{}
	}
	return tunnel
}

// Logs は、この経路について sshcエンジンが行ったことの記録と、コンテナの直近の
// ログを、秘密を伏せて返す。コンテナが無ければ、最後に用意できなかったときの
// ログを返す。
//
// 繋がらないときに利用者が最初に見る場所である。docker を直接叩かせない。
// Docker が使えないときも記録は返す。イメージの作成や docker の検出で失敗した
// 理由は、記録にしか残っていない。
func (manager *Manager) Logs(ctx context.Context, profileName string, secrets Secrets) (string, error) {
	if err := validateProfileName(profileName); err != nil {
		return "", err
	}
	record := manager.state(profileName).record.text()
	containerLogs, err := manager.currentContainerLogs(ctx, profileName, secrets)
	if err != nil {
		if record == "" {
			return "", err
		}
		containerLogs = "（読めませんでした：" + err.Error() + "）"
	}
	return lastBytes(redact(joinLogSections(record, containerLogs), secrets), maxLogBytes), nil
}

// currentContainerLogs は、コンテナのログを返す。コンテナが無ければ、最後に
// 用意できなかったときのログを返す。
func (manager *Manager) currentContainerLogs(ctx context.Context, profileName string, secrets Secrets) (string, error) {
	if _, err := manager.command(ctx); err != nil {
		return "", err
	}
	name := manager.containerName(profileName)
	ours, err := manager.requireOurContainer(ctx, name, profileName)
	if err != nil {
		return "", err
	}
	if !ours {
		// 用意できなかったコンテナは片付けてある。そのとき残したログを返す。
		return manager.state(profileName).lastFailureLogs(), nil
	}
	return manager.containerLogs(ctx, name, secrets), nil
}

// joinLogSections は、sshcエンジンの記録とコンテナのログを見出し付きで並べる。
func joinLogSections(record, containerLogs string) string {
	if record == "" {
		record = "（まだありません）"
	}
	if strings.TrimSpace(containerLogs) == "" {
		containerLogs = "（ありません）"
	}
	return "== sshcエンジンの記録 ==\n" + record + "\n\n== コンテナのログ ==\n" + containerLogs
}
