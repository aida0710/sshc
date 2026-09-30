package vpn

import (
	"slices"
	"strings"
)

// openvpn backend。利用者の設定ファイル（.ovpn）で、OpenVPN のトンネルを張る。
//
// 設定ファイルは鍵を含むことが多いので、中身は Vault に置く。一覧の表示と、接続先が
// VPN サーバーそのものでないかの確かめには、設定ファイルの remote から取り出した
// サーバーを、シークレットでない設定として持つ。
//
// 設定ファイルの中身は InspectOpenVPNConfig で確かめる。コンテナでは、サーバーが配る
// 経路と DNS を入れない指定と、sshc が決める指定（interface など）を、設定ファイルの
// あとにコマンドラインで足す（container/backend-openvpn.sh）。

// OpenVPNSettings は、openvpn backend のシークレットでない設定である。
type OpenVPNSettings struct {
	// Servers は、設定ファイルの remote の行のサーバーである（InspectOpenVPNConfig の
	// Servers と同じ並び）。設定ファイルと合わないものは保存しない。
	Servers []string
	// Username は、auth-user-pass で送るユーザー名である。空なら送らない。パスワードは
	// Vault にある。
	Username string
}

// OpenVPNSecrets は、openvpn backend のシークレットである。
type OpenVPNSecrets struct {
	// Config は、設定ファイルの中身である。
	Config string
	// Password は、auth-user-pass で送るパスワードである。Username があるときだけ使う。
	Password string
}

const (
	// MaxOpenVPNConfigLength は、設定ファイルの長さの上限（バイト数）である。API の
	// VPNSecrets.openvpnConfig と同じ値にする。証明書と鍵をインラインで含んでも、
	// ふつうは 20 KiB に収まる。
	MaxOpenVPNConfigLength = 64 << 10
	// maxOpenVPNServers は、プロファイルに持つサーバーの数の上限である。remote の数の
	// 上限と同じにする。API の OpenVPNProfile.servers と同じ値である。
	maxOpenVPNServers = maxOpenVPNRemotes
)

// openVPNCredentialForbidden は、ユーザー名とパスワードに使えない字である。OpenVPN は
// 1行目をユーザー名、2行目をパスワードとして読む。
const openVPNCredentialForbidden = "\r\n\x00"

type openVPNBackend struct{}

func (openVPNBackend) device() string { return "/dev/net/tun" }

// capabilities は、WireGuard と同じ権限（NET_ADMIN、NET_RAW、DAC_OVERRIDE）だけを渡す。
// OpenVPN を別のユーザーで動かす指定（user、group、chroot）は断るので、SETUID、SETGID、
// CHOWN、SYS_CHROOT は要らない。
func (openVPNBackend) capabilities() []string { return wireGuardBackend{}.capabilities() }

func (openVPNBackend) validateSettings(profile Profile) error {
	settings := profile.OpenVPN
	if settings == nil {
		return fieldError(ErrSettings, "openvpn", ReasonRequired)
	}
	if len(settings.Servers) == 0 {
		return fieldError(ErrSettings, "openvpn.servers", ReasonRequired)
	}
	if len(settings.Servers) > maxOpenVPNServers {
		return &FieldError{Kind: ErrSettings, Field: "openvpn.servers", Reason: ReasonTooMany, Limit: maxOpenVPNServers}
	}
	for _, server := range settings.Servers {
		if err := validateServerName("openvpn.servers", server); err != nil {
			return err
		}
	}
	if settings.Username == "" {
		return nil
	}
	return validateUsername("openvpn.username", settings.Username)
}

// validateSecrets は、設定ファイルを確かめ、プロファイルの設定と合っているかを見る。
//
// 設定ファイルは Vault にしか無いので、サーバーの並びが合うことはここでしか確かめ
// られない。合わないまま保存すると、一覧に出るサーバーと、実際に繋ぐサーバーが食い違う。
func (openVPNBackend) validateSecrets(profile Profile, secrets Secrets) error {
	var stored OpenVPNSecrets
	if secrets.OpenVPN != nil {
		stored = *secrets.OpenVPN
	}
	if err := requireSecret(openVPNConfigField, stored.Config, MaxOpenVPNConfigLength); err != nil {
		return err
	}
	summary, err := InspectOpenVPNConfig([]byte(stored.Config))
	if err != nil {
		return err
	}
	settings := profile.OpenVPN
	if settings == nil {
		return fieldError(ErrSettings, "openvpn", ReasonRequired)
	}
	if !slices.Equal(summary.Servers, settings.Servers) {
		return fieldError(ErrSettings, "openvpn.servers", ReasonConfigMismatch)
	}
	if settings.Username == "" {
		if summary.AsksCredentials {
			return fieldError(ErrSettings, "openvpn.username", ReasonRequiredByConfig)
		}
		return nil
	}
	field := "secrets." + SecretKeyOpenVPNPassword
	if err := requireSecret(field, stored.Password, maxSecretLength); err != nil {
		return err
	}
	if strings.ContainsAny(stored.Password, openVPNCredentialForbidden) {
		return fieldError(ErrSecrets, field, ReasonFormat)
	}
	return nil
}

func (openVPNBackend) writeAgentSection(request agentSectionRequest, document *agentDocument) error {
	settings := *request.profile.OpenVPN
	secrets := *request.secrets.OpenVPN
	section := &openVPNDocument{Config: secrets.Config, Servers: settings.Servers}
	// ユーザー名が無ければ、パスワードは使わない。保存済みのパスワードが Vault に
	// 残っていても、コンテナへは渡さない。
	if settings.Username != "" {
		section.Username, section.Password = settings.Username, secrets.Password
	}
	document.OpenVPN = section
	return nil
}

// secretValues は、パスワードと設定ファイルと、インラインのブロックの中の行を返す。
// 鍵は、設定ファイル全体としてではなく、1行ずつログに現れうる。
func (openVPNBackend) secretValues(secrets Secrets) []string {
	if secrets.OpenVPN == nil {
		return nil
	}
	values := []string{secrets.OpenVPN.Password, secrets.OpenVPN.Config}
	return append(values, openVPNInlineLines(secrets.OpenVPN.Config)...)
}

// minimumRedactedLineLength は、伏せる鍵の行の短さの下限である。鍵の行は 64 字（PEM）か
// 32 字（OpenVPN の静的鍵）である。PEM の本文の最後の行だけは短いが、鍵の断片にすぎない。
// 短い語まで伏せると、ログのふつうの語が消える。
const minimumRedactedLineLength = 16

// openVPNCredentialBlocks は、ユーザー名とパスワードを書くインラインのブロックである。
// 1行目がユーザー名、2行目がパスワードである（OpenVPN の get_user_pass）。
var openVPNCredentialBlocks = map[string]bool{"auth-user-pass": true, "http-proxy-user-pass": true}

// openVPNCredentialPasswordLine は、認証情報のブロックの中で、パスワードを書く行の番号
// （1から数える）である。
const openVPNCredentialPasswordLine = 2

// openVPNInlineLines は、インラインのブロックの中の、伏せる行を返す。
//
// 鍵の行は長さで選ぶ。「-----BEGIN …」のような区切りの行と、<connection> の中の指示は
// 伏せない。認証情報のブロックのパスワードは、Vault のパスワードと同じく長さに関係なく
// 伏せる。
func openVPNInlineLines(config string) []string {
	var lines []string
	inside, credentials, lineInBlock := false, false, 0
	for _, line := range strings.Split(config, "\n") {
		trimmed := strings.TrimSpace(line)
		if tag, isInline := inlineTag([]string{trimmed}); isInline {
			inside = !strings.HasPrefix(tag, "/") && tag != "connection"
			credentials = inside && openVPNCredentialBlocks[tag]
			lineInBlock = 0
			continue
		}
		if !inside {
			continue
		}
		lineInBlock++
		switch {
		case credentials && lineInBlock == openVPNCredentialPasswordLine && trimmed != "":
			lines = append(lines, trimmed)
		case len(trimmed) >= minimumRedactedLineLength && !strings.HasPrefix(trimmed, "-----"):
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func (openVPNBackend) ownSecrets(_ Profile, secrets Secrets) Secrets {
	return Secrets{OpenVPN: secrets.OpenVPN}
}

func (openVPNBackend) waitsForApproval(Profile) bool { return false }

// openVPNDocument は、agent が OpenVPN を起動するのに要るものである。
//
// 設定ファイルとパスワードは、agent が tmpfs の上の root だけが読めるファイルへ書き、
// そのパスを OpenVPN へ渡す。引数にも環境変数にも置かない。
type openVPNDocument struct {
	Config string `json:"config"`
	// Servers は、agent が先に名前解決を確かめるサーバーである。どれも引けなければ、
	// OpenVPN を起動せずに理由を返す。
	Servers  []string `json:"servers"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
}
