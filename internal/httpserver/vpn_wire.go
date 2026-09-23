package httpserver

import (
	"sshc/internal/application"
	"sshc/internal/vpn"
)

// VPN の API の本文の形である。CLI も同じ型で読み書きする。形を二か所に持つと、
// 片方だけに項目が増えたときに、未知の項目を許さない読み手が壊れる。

// VPNOverview は、保存済みのプロファイルと、それぞれのいまの状態である。
type VPNOverview struct {
	Available bool               `json:"available"`
	Detail    string             `json:"detail,omitempty"`
	Profiles  []VPNProfileStatus `json:"profiles"`
}

// VPNProfileStatus は、プロファイルひとつと、その経路のいまの状態である。
type VPNProfileStatus struct {
	Profile application.VPNProfile `json:"profile"`
	Running bool                   `json:"running"`
	// RelaySocket は、中継のソケットの場所である。開いていなければ空になる。
	RelaySocket string   `json:"relaySocket"`
	Connections []string `json:"connections"`
	// Tunnel は、コンテナの中のトンネルの様子である。経路が無ければ省略する。
	Tunnel *VPNTunnel `json:"tunnel,omitempty"`
	// Phase は、いま経路を用意している段階である。用意していなければ空。
	Phase vpn.StartPhase `json:"phase,omitempty"`
}

// VPNTunnel は、コンテナの中のトンネルの様子である。
type VPNTunnel struct {
	Interface string `json:"interface,omitempty"`
	Address   string `json:"address,omitempty"`
	Since     string `json:"since,omitempty"`
	Backend   string `json:"backend,omitempty"`
	// TargetAddress は、VPNの中で引けた接続先のアドレスである。
	TargetAddress string `json:"targetAddress,omitempty"`
}

// VPNLogs は、コンテナの直近のログを、秘密を伏せたものである。
type VPNLogs struct {
	Lines string `json:"lines"`
}

// VPNProfileRequest は、プロファイルの保存要求である。秘密は省略できる。
type VPNProfileRequest struct {
	Profile application.VPNProfile `json:"profile"`
	Secrets *vpn.SecretsDocument   `json:"secrets,omitempty"`
}

// VPNRenameRequest は、プロファイルの改名要求である。
type VPNRenameRequest struct {
	Name string `json:"name"`
}

// VPNBindingRequest は、接続が通るプロファイルを決める要求である。
type VPNBindingRequest struct {
	Alias   string `json:"alias"`
	Profile string `json:"profile"`
}
