package vpn

import (
	"net/netip"
	"strings"
)

// wireguard backend。userspace の wireguard-go でトンネルを張る。

// WireGuardSettings は、wireguard backendの秘密でない設定である。
type WireGuardSettings struct {
	// Server は、トンネルの相手である。ここへはコンテナの通常回線で届く。
	Server Endpoint
	// PeerPublicKey は、相手の公開鍵である。秘密ではない。
	PeerPublicKey string
	// Address は、トンネル側でこの端末が名乗るアドレスである（CIDR表記）。
	Address string
}

// wireGuardKeyLength は、32 バイトの鍵を base64 で書いた長さである。
const wireGuardKeyLength = 44

// wireGuardKeepaliveSeconds は、NAT の内側からでも経路を保つための間隔である。
// 相手が先に話しかけてくる構成でも、こちらの経路が落ちたままにならない。
const wireGuardKeepaliveSeconds = "25"

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
	if settings.Server.Host == "" {
		return fieldError(ErrSettings, "wireguard.server", ReasonRequired)
	}
	if strings.ContainsAny(settings.Server.Host, " \t\r\n") {
		return fieldError(ErrSettings, "wireguard.server", ReasonFormat)
	}
	if err := validatePort(ErrSettings, "wireguard.server", settings.Server.Port); err != nil {
		return err
	}
	if err := validateWireGuardKey(ErrSettings, "wireguard.peerPublicKey", settings.PeerPublicKey); err != nil {
		return err
	}
	if settings.Address == "" {
		return fieldError(ErrSettings, "wireguard.address", ReasonRequired)
	}
	prefix, err := netip.ParsePrefix(settings.Address)
	if err != nil {
		return fieldError(ErrSettings, "wireguard.address", ReasonFormat)
	}
	if !prefix.Addr().Is4() {
		return fieldError(ErrSettings, "wireguard.address", ReasonNotIPv4)
	}
	return nil
}

func (wireGuardBackend) validateSecrets(_ Profile, secrets Secrets) error {
	if secrets.WireGuard == nil {
		return fieldError(ErrSecrets, "secrets."+SecretKeyWireGuardPrivateKey, ReasonRequired)
	}
	return validateWireGuardKey(ErrSecrets, "secrets."+SecretKeyWireGuardPrivateKey, secrets.WireGuard.PrivateKey)
}

// validateWireGuardKey は、鍵がbase64の32バイトであることだけを確かめる。
//
// 設定ファイルへ書く値なので、改行や引用符が混じったまま渡さない。
func validateWireGuardKey(kind error, field, key string) error {
	if key == "" {
		return fieldError(kind, field, ReasonRequired)
	}
	if len(key) != wireGuardKeyLength || !strings.HasSuffix(key, "=") {
		return fieldError(kind, field, ReasonFormat)
	}
	for _, character := range key[:len(key)-1] {
		if !isASCIIAlphanumeric(character) && character != '+' && character != '/' {
			return fieldError(kind, field, ReasonFormat)
		}
	}
	return nil
}

func (wireGuardBackend) writeAgentSection(request agentSectionRequest, document *agentDocument) error {
	document.WireGuard = &wireGuardDocument{
		Configuration: wireGuardConfiguration(request.profile, *request.secrets.WireGuard),
		Address:       request.profile.WireGuard.Address,
	}
	return nil
}

func (wireGuardBackend) secretValues(secrets Secrets) []string {
	if secrets.WireGuard == nil {
		return nil
	}
	return []string{secrets.WireGuard.PrivateKey}
}

func (wireGuardBackend) waitsForApproval(Profile) bool { return false }

// wireGuardDocument は、agent が wireguard のトンネルを張るのに要るものである。
type wireGuardDocument struct {
	// Configuration は wg setconf がそのまま読む本文である。
	Configuration string `json:"configuration"`
	// Address は、トンネル側でこの端末が名乗るアドレスである。wg setconf は
	// これを扱わないので、ip address add へ別に渡す。
	Address string `json:"address"`
}

// wireGuardConfiguration は、wg setconf が読む本文を作る。
//
// AllowedIPs は接続先とDNSサーバーだけにする。トンネルが運ぶのはその通信に
// 限られ、VPNの向こうのネットワーク全体を引き込まない。
func wireGuardConfiguration(profile Profile, secrets WireGuardSecrets) string {
	settings := *profile.WireGuard
	lines := []string{
		"[Interface]",
		"PrivateKey = " + secrets.PrivateKey,
		"",
		"[Peer]",
		"PublicKey = " + settings.PeerPublicKey,
		"Endpoint = " + settings.Server.Address(),
		"AllowedIPs = " + strings.Join(allowedAddresses(profile), ", "),
		"PersistentKeepalive = " + wireGuardKeepaliveSeconds,
		"",
	}
	return strings.Join(lines, "\n")
}

// allowedAddresses は、トンネルが運んでよい相手である。
//
// 接続先を名前で書いた場合、そのアドレスはこの時点では分からない。まずDNS
// サーバーまでを通し、agent がVPNの中で名前を引いてから接続先を足す。
func allowedAddresses(profile Profile) []string {
	allowed := make([]string, 0, len(profile.DNS)+1)
	if _, err := netip.ParseAddr(profile.Target.Host); err == nil {
		allowed = append(allowed, profile.Target.Host+"/32")
	}
	for _, resolver := range profile.DNS {
		allowed = append(allowed, resolver+"/32")
	}
	return allowed
}
