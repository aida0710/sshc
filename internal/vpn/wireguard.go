package vpn

import (
	"slices"
	"strconv"
	"strings"
)

// wireguard backend。利用者の設定ファイル（wg-quick の形）で、userspace の wireguard-go の
// トンネルを張る。
//
// 設定ファイルは鍵（PrivateKey と PresharedKey）を含むので、利用者が書いたとおりの本文を
// そのまま Vault に置く（OpenVPN の .ovpn と同じ扱い）。一覧の表示には、設定ファイルの
// Endpoint から取り出したサーバーを、シークレットでない設定として持つ。
//
// コンテナへは、wg setconf が読む形に直して渡す（wgSetconfText）。wg-quick だけが読む
// Address と MTU は agent が ip で設定し、DNS はプロファイルの DNS として使う。経路は
// 作らない。接続先への経路は、ほかの backend と同じく connect が1つずつ作る。

// WireGuardSettings は、wireguard backend のシークレットでない設定である。
type WireGuardSettings struct {
	// Servers は、設定ファイルの Endpoint のサーバーである（WireGuardConfig.Servers と同じ
	// 並び）。設定ファイルと合わないものは保存しない。
	Servers []string
}

// WireGuardSecrets は、wireguard backend のシークレットである。
type WireGuardSecrets struct {
	// Config は、設定ファイルの中身（鍵を含む、利用者が書いたとおりの本文）である。
	Config string
}

// wireGuardKeyLength は、32 バイトの鍵を base64 で書いた長さである。
const wireGuardKeyLength = 44

// wireGuardServersField は、サーバーの項目の JSON パスである。
const wireGuardServersField = "wireguard.servers"

type wireGuardBackend struct{}

func (wireGuardBackend) device() string { return "/dev/net/tun" }

// capabilities は、実際のコンテナで確かめた権限だけを渡す。NET_ADMIN はトンネル
// と経路のため、NET_RAW は iptables のため、DAC_OVERRIDE は利用者のものである
// ソケット用ディレクトリへ書くため、CHOWN は中継のソケットを利用者のものにする
// ためである。既定で付いてくる残り（MKNOD・SYS_CHROOT・SETUID など）は要らない。
func (wireGuardBackend) capabilities() []string {
	return []string{
		"--cap-drop", "ALL",
		"--cap-add", "NET_ADMIN",
		"--cap-add", "NET_RAW",
		"--cap-add", "DAC_OVERRIDE",
		"--cap-add", "CHOWN",
	}
}

func (wireGuardBackend) validateSettings(profile Profile) error {
	settings := profile.WireGuard
	if settings == nil {
		return fieldError(ErrSettings, "wireguard", ReasonRequired)
	}
	if len(settings.Servers) == 0 {
		return fieldError(ErrSettings, wireGuardServersField, ReasonRequired)
	}
	if len(settings.Servers) > maxWireGuardPeers {
		return &FieldError{Kind: ErrSettings, Field: wireGuardServersField, Reason: ReasonTooMany, Limit: maxWireGuardPeers}
	}
	for _, server := range settings.Servers {
		if err := validateServerName(wireGuardServersField, server); err != nil {
			return err
		}
	}
	return nil
}

// validateSecrets は、設定ファイルを確かめ、プロファイルの設定と合っているかを見る。
//
// 設定ファイルは Vault にしか無いので、サーバーと DNS が合うことはここでしか確かめられない。
// サーバーが合わないまま保存すると、一覧に出るサーバーと実際に繋ぐサーバーが食い違う。DNS が
// 合わないと、接続先の名前を引けるかの確かめ（Profile.Destination）と、実際に名前解決に使う
// DNS が別になる。
func (wireGuardBackend) validateSecrets(profile Profile, secrets Secrets) error {
	config, err := wireGuardConfigOf(secrets)
	if err != nil {
		return err
	}
	defer config.Forget()
	settings := profile.WireGuard
	if settings == nil {
		return fieldError(ErrSettings, "wireguard", ReasonRequired)
	}
	if !slices.Equal(config.Servers(), settings.Servers) {
		return fieldError(ErrSettings, wireGuardServersField, ReasonConfigMismatch)
	}
	if !slices.Equal(config.DNS, profile.normalized().DNS) {
		return fieldError(ErrSettings, "dns", ReasonConfigMismatch)
	}
	return nil
}

// wireGuardConfigOf は、シークレットの設定ファイルを読む。
func wireGuardConfigOf(secrets Secrets) (WireGuardConfig, error) {
	var stored WireGuardSecrets
	if secrets.WireGuard != nil {
		stored = *secrets.WireGuard
	}
	if err := requireSecret(wireGuardConfigField, stored.Config, MaxWireGuardConfigLength); err != nil {
		return WireGuardConfig{}, err
	}
	return ParseWireGuardConfig([]byte(stored.Config))
}

func (wireGuardBackend) writeAgentSection(request agentSectionRequest, document *agentDocument) error {
	config, err := wireGuardConfigOf(request.secrets)
	if err != nil {
		return err
	}
	defer config.Forget()
	addresses := make([]string, 0, len(config.Addresses))
	for _, address := range config.Addresses {
		addresses = append(addresses, address.String())
	}
	document.WireGuard = &wireGuardDocument{
		Configuration: wgSetconfText(config),
		Addresses:     addresses,
		MTU:           config.MTU,
	}
	return nil
}

// secretValues は、設定ファイルと、その中の鍵を返す。鍵は、設定ファイル全体としてではなく、
// 1行ずつログに現れうる。
func (wireGuardBackend) secretValues(secrets Secrets) []string {
	if secrets.WireGuard == nil {
		return nil
	}
	values := []string{secrets.WireGuard.Config}
	config, err := ParseWireGuardConfig([]byte(secrets.WireGuard.Config))
	if err != nil {
		return values
	}
	defer config.Forget()
	values = append(values, string(config.PrivateKey.Value))
	for _, peer := range config.Peers {
		if peer.PresharedKey.present() {
			values = append(values, string(peer.PresharedKey.Value))
		}
	}
	return values
}

func (wireGuardBackend) waitsForApproval(Profile) bool { return false }

func (wireGuardBackend) ownSecrets(_ Profile, secrets Secrets) Secrets {
	return Secrets{WireGuard: secrets.WireGuard}
}

// wireGuardDocument は、agent が wireguard のトンネルを張るのに要るものである。
type wireGuardDocument struct {
	// Configuration は wg setconf がそのまま読む本文である。
	Configuration string `json:"configuration"`
	// Addresses は、トンネル側でこのマシンが名乗る IPv4 アドレス（CIDR）である。wg setconf は
	// これを扱わないので、ip address add へ別に渡す。
	Addresses []string `json:"addresses"`
	// MTU は、トンネルの MTU である。0 なら wireguard-go の既定のままにする。
	MTU int `json:"mtu,omitempty"`
}

// wgSetconfText は、wg setconf が読む本文を作る。
//
// 利用者の本文をそのまま渡さず、読み取った値から組み立て直す。wg-quick だけが読む項目
// （Address、DNS、MTU、Table、SaveConfig）と注釈は、wg setconf が読めないので書かない。
func wgSetconfText(config WireGuardConfig) string {
	lines := []string{"[Interface]", "PrivateKey = " + string(config.PrivateKey.Value)}
	if config.ListenPort != "" {
		lines = append(lines, "ListenPort = "+config.ListenPort)
	}
	if config.FwMark != "" {
		lines = append(lines, "FwMark = "+config.FwMark)
	}
	for _, peer := range config.Peers {
		lines = append(lines, "", "[Peer]", "PublicKey = "+peer.PublicKey)
		if peer.PresharedKey.present() {
			lines = append(lines, "PresharedKey = "+string(peer.PresharedKey.Value))
		}
		if peer.Endpoint != "" {
			lines = append(lines, "Endpoint = "+peer.Endpoint)
		}
		if len(peer.AllowedIPs) > 0 {
			allowed := make([]string, 0, len(peer.AllowedIPs))
			for _, prefix := range peer.AllowedIPs {
				allowed = append(allowed, prefix.String())
			}
			lines = append(lines, "AllowedIPs = "+strings.Join(allowed, ", "))
		}
		if keepalive := wireGuardKeepalive(peer); keepalive > 0 {
			lines = append(lines, "PersistentKeepalive = "+strconv.Itoa(keepalive))
		}
	}
	return strings.Join(append(lines, ""), "\n")
}

// wireGuardKeepalive は、[Peer] に使う PersistentKeepalive の秒数である。
//
// sshc は、ハンドシェイクが済んだことでトンネルが張れたと判断し、ハンドシェイクが続いて
// いることでトンネルが生きていると判断する。Endpoint のある [Peer] で keepalive が無いと、
// 送るものが無いあいだハンドシェイクが起きないので、sshc の既定の間隔を使う。
func wireGuardKeepalive(peer WireGuardPeer) int {
	if peer.PersistentKeepalive == 0 && peer.Endpoint != "" {
		return defaultWireGuardKeepaliveSeconds
	}
	return peer.PersistentKeepalive
}
