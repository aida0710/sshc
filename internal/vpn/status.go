package vpn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	// OpenConnections は、この経路を通っている接続（と、経路を待っている接続）の
	// 数である。経路を止めると、これらの接続も切れる。
	OpenConnections int
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
// docker から読んだ状態は routeReadingLifetime のあいだ使い回す。画面は開いている
// あいだ一覧を読み直し、CLI も経路の段階を読みに来るので、読み取りのたびに docker を
// 呼ぶと、その数だけ応答が遅れる。Docker が使えなければ、その理由を返す。
func (manager *Manager) Statuses(ctx context.Context) (map[string]Status, error) {
	reading, err := manager.readRoutes(ctx)
	if err != nil {
		return nil, err
	}
	if reading.err != nil {
		return nil, reading.err
	}
	return manager.composeStatuses(reading), nil
}

// KnownStatuses は、docker を待たずに分かる経路の状態を返す。known が false なら、
// 経路の状態をまだ確かめていない。そのときは docker から読み始め、sshcエンジンが
// 自分で起動・停止した経路の状態だけを返す。
//
// 画面を開いたときに、プロファイルの一覧を docker を待たずに見せるために使う。
func (manager *Manager) KnownStatuses() (statuses map[string]Status, known bool, err error) {
	reading, known := manager.knownRoutes()
	if !known {
		// since が 0 の読み取りは、sshcエンジンが一度でも起動・停止した経路を
		// すべて、docker より新しいものとして扱う。
		return manager.composeStatuses(&routeReading{}), false, nil
	}
	if reading.err != nil {
		return nil, true, reading.err
	}
	return manager.composeStatuses(reading), true, nil
}

// composeStatuses は、docker から読んだ状態に、sshcエンジンがその後に起動・停止した
// 経路と、用意の途中の経路を重ねる。
func (manager *Manager) composeStatuses(reading *routeReading) map[string]Status {
	statuses := map[string]Status{}
	for name, running := range reading.containers {
		statuses[name] = manager.containerStatus(name, running)
	}
	for _, name := range manager.names() {
		state := manager.state(name)
		if state.changedSince(reading.since) {
			if state.isRunning() {
				statuses[name] = manager.containerStatus(name, true)
			} else {
				delete(statuses, name)
			}
		}
		if _, present := statuses[name]; present {
			continue
		}
		if phase := state.currentPhase(); phase != "" {
			statuses[name] = Status{Name: name, Phase: phase, OpenConnections: state.openConnections()}
		}
	}
	return statuses
}

// containerStatus は、コンテナがあるプロファイルひとつの状態を組み立てる。
func (manager *Manager) containerStatus(profileName string, running bool) Status {
	state := manager.state(profileName)
	status := Status{
		Name: profileName, Running: running, Phase: state.currentPhase(), OpenConnections: state.openConnections(),
	}
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
	return lastBytes(redactLogs(joinLogSections(record, containerLogs), secrets), maxLogBytes), nil
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
