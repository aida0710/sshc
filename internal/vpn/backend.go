package vpn

import "time"

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
)

// backend は、トンネルの張り方ひとつぶんの違いである。
//
// 共通の流れ（コンテナ、中継、経路、fail closed）はここに入れない。backend を
// 足すときに触るのは、その backend の file と、下の backends の表だけにする。
type backend interface {
	// device は、コンテナへ渡すトンネルのデバイスである。
	device() string
	// capabilities は、docker run へ渡す権限の指定である。
	capabilities() []string
	// validateSettings は、この backend の節を確かめる。節が無いことも断る。
	validateSettings(profile Profile) error
	validateSecrets(profile Profile, secrets Secrets) error
	// ownSecrets は、secrets のうち、この backend の節だけを残した写しである。
	ownSecrets(secrets Secrets) Secrets
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
