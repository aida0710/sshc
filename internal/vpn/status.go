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
	// Target は、このセッションが繋ぐ先である。
	Target string
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
	// TargetAddress は、VPNの中で引けた接続先のアドレスである。接続先を
	// アドレスで書いた場合は、その値がそのまま入る。
	TargetAddress string `json:"targetAddress"`
}

// Status は、このプロファイルの状態を返す。
func (manager *Manager) Status(ctx context.Context, profileName string) (Status, error) {
	if err := validateProfileName(profileName); err != nil {
		return Status{}, err
	}
	if _, err := manager.command(ctx); err != nil {
		return Status{}, err
	}
	state := manager.state(profileName)
	name := manager.containerName(profileName)
	status := Status{Name: profileName, Phase: state.currentPhase()}
	ours, err := manager.requireOurContainer(ctx, name, profileName)
	if err != nil {
		return Status{}, err
	}
	if !ours {
		return status, nil
	}
	running, err := manager.containerRunning(ctx, name)
	if err != nil {
		return Status{}, err
	}
	status.Running = running
	format := "{{index .Config.Labels \"" + targetLabel + "\"}}"
	if output, err := manager.docker.output(ctx, "container", "inspect", "--format", format, name); err == nil {
		status.Target = strings.TrimSpace(output)
	}
	// コンテナが終わっても、ホスト側にはソケットのファイルが残る。動いている
	// コンテナと engine の中継が揃っているときだけ、使える経路として見せる。
	if running {
		status.RelaySocket = state.relaySocket()
	}
	if status.RelaySocket != "" {
		status.Tunnel = manager.tunnelStatus(profileName)
	}
	return status, nil
}

// tunnelStatus は、agent が書き出した様子を読む。読めなければゼロ値を返す。
// 状態が読めないことは失敗ではない。経路があることは中継のソケットが示している。
func (manager *Manager) tunnelStatus(profileName string) TunnelStatus {
	contents, err := os.ReadFile(filepath.Join(manager.socketDirectory(profileName), statusFileName))
	if err != nil || len(contents) > maxStatusBytes {
		return TunnelStatus{}
	}
	var tunnel TunnelStatus
	if err := json.Unmarshal(contents, &tunnel); err != nil {
		return TunnelStatus{}
	}
	return tunnel
}

// Logs は、そのコンテナの直近のログを、秘密を伏せて返す。コンテナが無ければ、
// 最後に用意できなかったときのログを返す。
//
// 繋がらないときに利用者が最初に見る場所である。docker を直接叩かせない。
func (manager *Manager) Logs(ctx context.Context, profileName string, secrets Secrets) (string, error) {
	if err := validateProfileName(profileName); err != nil {
		return "", err
	}
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
