package vpn

import (
	"encoding/json"
	"fmt"
	"strings"
)

// agentDocument は、コンテナのagentへ標準入力で渡す設定である。
//
// 秘密を含むので、コマンド引数・環境変数・イメージ・bind mountには置かない。
// agentは読み終えたらこの文書を消す。
type agentDocument struct {
	Backend     string             `json:"backend"`
	Target      endpointDocument   `json:"target"`
	SocketOwner int                `json:"socketOwner"`
	WireGuard   *wireGuardDocument `json:"wireguard,omitempty"`
}

type endpointDocument struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type wireGuardDocument struct {
	// Configuration は wg setconf がそのまま読む本文である。
	Configuration string `json:"configuration"`
	// Address は、トンネル側でこの端末が名乗るアドレスである。wg setconf は
	// これを扱わないので、ip address add へ別に渡す。
	Address string `json:"address"`
}

// newAgentDocument は、プロファイルと秘密から、コンテナへ渡す設定を作る。
func newAgentDocument(profile Profile, secrets Secrets, socketOwner int) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	if err := profile.ValidateSecrets(secrets); err != nil {
		return "", err
	}
	document := agentDocument{
		Backend:     string(profile.Backend),
		Target:      endpointDocument{Host: profile.Target.Host, Port: profile.Target.Port},
		SocketOwner: socketOwner,
		WireGuard: &wireGuardDocument{
			Configuration: wireGuardConfiguration(*profile.WireGuard, profile.Target, secrets),
			Address:       profile.WireGuard.Address,
		},
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// wireGuardConfiguration は、wg setconf が読む本文を作る。
//
// AllowedIPs は接続先ひとつだけにする。トンネルが運ぶのはその接続先への通信に
// 限られ、VPNの向こうのネットワーク全体を引き込まない。
func wireGuardConfiguration(settings WireGuardSettings, target Endpoint, secrets Secrets) string {
	lines := []string{
		"[Interface]",
		"PrivateKey = " + secrets.WireGuardPrivateKey,
		"",
		"[Peer]",
		"PublicKey = " + settings.PeerPublicKey,
		"Endpoint = " + settings.Server.Address(),
		fmt.Sprintf("AllowedIPs = %s/32", target.Host),
		// NATの内側からでも経路を保つ。相手が先に話しかけてくる構成でも、
		// こちらの経路が落ちたままにならない。
		"PersistentKeepalive = 25",
		"",
	}
	return strings.Join(lines, "\n")
}

// redact は、表示する文字列から秘密を伏せる。docker logs をそのまま見せない。
func redact(text string, secrets Secrets) string {
	if secrets.WireGuardPrivateKey == "" {
		return text
	}
	return strings.ReplaceAll(text, secrets.WireGuardPrivateKey, "[REDACTED]")
}
