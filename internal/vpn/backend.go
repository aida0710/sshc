package vpn

import (
	"slices"
	"time"
)

// BackendName は、トンネルの張り方である。
type BackendName string

const (
	// WireGuard は userspace の wireguard-go でトンネルを張る。
	WireGuard BackendName = "wireguard"
	// L2TPIPsec は strongSwan と xl2tpd と pppd でトンネルを張る。大学や
	// 会社の装置に多い方式である。
	L2TPIPsec BackendName = "l2tp_ipsec"
	// OpenConnect は openconnect でトンネルを張る。Cisco AnyConnect と
	// その仲間（ocserv、GlobalProtect、Pulse など）へ繋ぐ。
	OpenConnect BackendName = "openconnect"
	// OpenVPN は、利用者の設定ファイル（.ovpn）で OpenVPN のトンネルを張る。
	OpenVPN BackendName = "openvpn"
	// IKEv2 は strongSwan（swanctl）で IKEv2/IPsec のトンネルを張る。Windows、macOS、
	// スマートフォンの標準の VPN が話す方式である。
	IKEv2 BackendName = "ikev2"
)

// backend は、トンネルの張り方ひとつぶんの違いである。
//
// 共通の流れ（コンテナ、中継、経路、fail closed）はここに入れない。
//
// 方式を足すときは、その方式の file（<方式>.go、container/backend-<方式>.sh）と下の
// backends の表のほかに、次の並びにも足す。【検査】と書いた並びは、足し忘れると
// テストが落ちる。書いていない並びは、その方式のテストで確かめる。
//
// internal/vpn:
//   - profile.go: Profile の節、foreignSection【検査】
//   - secrets.go: Secrets の節、SecretsDocument、Secrets と Document【検査】、SecretKey の定数
//   - agentdocument.go: agentDocument の節
//   - failure.go: 方式に固有の失敗の理由の語と knownFailureReasons
//   - container/agent.sh の backend の枝と container/Dockerfile の COPY【検査】、Dockerfile のパッケージ
//
// ほかの層:
//   - internal/application/vpnprofile.go: VPNProfile の節、Profile の写し。
//     保存の形が増えるので、metadata.json の schema を上げる（metadata.go）
//   - internal/vpnprofile/secrets.go: overlaySecrets【検査】
//   - internal/vpnrefusal: 理由の語の言い方と testdata/failure-reasons.json【検査】
//   - cmd/sshc/vpn_profile_input.go: 方式の質問と誤りの文、readVPNProfile の switch【検査】。
//     方式の入力は vpn_<方式>_input.go、説明は cmd/sshc/internal/clispec/spec.go
//   - api/openapi.yaml: VPNBackend の enum【検査】と方式の節
//   - web/src/vpn: vpnBackends.ts（型で検査）、vpnFailureReasons.ts【検査】、vpnProfileDraft.ts、
//     vpnProfileRules.ts、vpnSecretRules.ts、vpnFieldErrors.ts、VPNProfileCard.tsx、
//     VPNProfileForm.tsx と方式の入力欄。文は web/src/i18n
type backend interface {
	// device は、コンテナへ渡すトンネルのデバイスである。空なら何も渡さない。
	device() string
	// capabilities は、docker run へ渡す権限の指定である。
	capabilities() []string
	// validateSettings は、この backend の節を確かめる。節が無いことも断る。
	validateSettings(profile Profile) error
	validateSecrets(profile Profile, secrets Secrets) error
	// ownSecrets は、secrets のうち、この backend の節だけを残した写しである。プロファイルの
	// 設定が使わないシークレット（消した [Peer] の鍵など）も残さない。
	ownSecrets(profile Profile, secrets Secrets) Secrets
	// writeAgentSection は、agent へ渡す文書のうち、この backend の節を書く。
	writeAgentSection(request agentSectionRequest, document *agentDocument) error
	// secretValues は、ログから伏せる値である。
	secretValues(secrets Secrets) []string
	// waitsForApproval は、人の承認を待つ経路かを返す。
	waitsForApproval(profile Profile) bool
}

// agentSectionRequest は、backend が agent へ渡す節を作るのに使うものである。
type agentSectionRequest struct {
	profile Profile
	secrets Secrets
	// now は、二段目のコードを作る時刻である。
	now time.Time
}

var backends = map[BackendName]backend{
	WireGuard:   wireGuardBackend{},
	L2TPIPsec:   l2tpBackend{},
	OpenConnect: openConnectBackend{},
	OpenVPN:     openVPNBackend{},
	IKEv2:       ikev2Backend{},
}

// Backends は、backends の表にある方式の名前を、名前の順に返す。表の外に方式ごとの
// 並びを持つ層のテストが、足し忘れを見つけるのに使う。
func Backends() []BackendName {
	names := make([]BackendName, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// backendFor は、名前から backend を引く。
func backendFor(name BackendName) (backend, error) {
	chosen, known := backends[name]
	if !known {
		return nil, fieldError(ErrBackend, "backend", ReasonUnsupported)
	}
	return chosen, nil
}

// commonCapabilities は、既定の権限のまま NET_ADMIN だけを足す指定である。
//
// 要る権限を実際の装置で確かめられていない backend が使う。確かめずに削ると、
// 繋がらなくなった理由が権限にあることを利用者が知る手段が無い。
var commonCapabilities = []string{"--cap-add", "NET_ADMIN"}
