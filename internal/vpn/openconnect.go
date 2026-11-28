package vpn

import (
	"fmt"
	"strings"
	"time"

	"sshc/internal/totp"
)

// openconnect backend。
//
// openconnect は接続のたびに外部スクリプトを呼び、そこで interface と経路を
// 作らせる。既定の vpnc-script はVPNサーバーが配った既定経路とDNSをそのまま入れる。
// それでは、このコンテナが「接続先ひとつだけを通す」約束を守れない。だから
// 自分の script（container/vpnc-script）をイメージに焼き、それを呼ばせる。

// OpenConnectSettings は、openconnect backendの秘密でない設定である。
type OpenConnectSettings struct {
	// Server は、VPNサーバーのホスト名またはアドレスである。ホスト名はコンテナの中で名前解決する。
	Server string
	// Username は、VPNのユーザー名である。パスワードはVaultにある。
	Username string
	// Protocol は、VPNサーバーのプロトコルである。空なら anyconnect。
	Protocol string
	// ServerCertificate は、サーバーの証明書を固定するフィンガープリントである
	// （openConnectFingerprintPrefixes の `sha256:...` または `pin-sha256:...`）。
	// 公的な認証局の証明書を使うサーバーでは空でよい。自己署名証明書のサーバーでは、
	// これが無いと openconnect は接続しない。
	ServerCertificate string
	// SecondFactor は、パスワードのあとへの備えである。空なら何もしない。
	SecondFactor string
	// ApprovalWord は、SecondFactor が approve のときに二要素認証の入力として送る語である。
	// 空なら何も送らない。VPNサーバーが二要素認証の入力を求める場合だけ書く（Duo なら
	// push、VPNサーバーによっては phone や sms）。
	ApprovalWord string
}

// パスワードのあとにVPNサーバーがすることへの備えである。
const (
	// SecondFactorApprove は、ユーザーがスマートフォンで承認するのを待つ。Duo Mobile などが
	// 通知を出し、承認するまでVPNサーバーは応答を返さない。
	//
	// 二要素認証の入力を求めるVPNサーバーと、何も求めずに通知だけ出すVPNサーバーがある。前者には
	// ApprovalWord を送り、後者には何も送らない。どちらも、待つ長さが普通の
	// 接続より長いことは同じである。
	SecondFactorApprove = "approve"
	// SecondFactorTOTP は、Vault に置いたTOTPのシークレットから作ったコードを送る。
	SecondFactorTOTP = "totp"
)

// defaultOpenConnectProtocol は、方式を書かなかったときに使う方式である。
// Cisco AnyConnect と、それと互換のVPNサーバー（ocserv など）が使う。
const defaultOpenConnectProtocol = "anyconnect"

// openConnectProtocols は、openconnect が話せる方式のうち、このプロファイルで
// 指定できるものである。openconnect の --protocol にそのまま渡る。
var openConnectProtocols = map[string]bool{
	"anyconnect": true, "nc": true, "pulse": true, "gp": true,
	"f5": true, "fortinet": true, "array": true,
}

// openConnectFingerprintPrefixes は、openconnect が --servercert で受け取る
// 2つの書き方である。証明書そのものの SHA-256（16進）と、公開鍵の PIN（base64）。
var openConnectFingerprintPrefixes = []string{"sha256:", "pin-sha256:"}

// totpMinimumRemaining は、渡すコードに残しておく有効な時間である。窓の終わり
// 間際に作ると、VPNサーバーへ届く前に次のコードへ変わる。
const totpMinimumRemaining = 5 * time.Second

type openConnectBackend struct{}

func (openConnectBackend) device() string { return "/dev/net/tun" }

func (openConnectBackend) capabilities() []string { return commonCapabilities }

func (openConnectBackend) validateSettings(profile Profile) error {
	settings := profile.OpenConnect
	if settings == nil {
		return fieldError(ErrSettings, "openconnect", ReasonRequired)
	}
	if err := validateServerName("openconnect.server", settings.Server); err != nil {
		return err
	}
	if err := validateUsername("openconnect.username", settings.Username); err != nil {
		return err
	}
	if settings.Protocol != "" && !openConnectProtocols[settings.Protocol] {
		return fieldError(ErrSettings, "openconnect.protocol", ReasonUnsupported)
	}
	if err := validateFingerprint(settings.ServerCertificate); err != nil {
		return err
	}
	switch settings.SecondFactor {
	case "", SecondFactorApprove, SecondFactorTOTP:
	default:
		return fieldError(ErrSettings, "openconnect.secondFactor", ReasonUnsupported)
	}
	if err := validateLength("openconnect.approvalWord", settings.ApprovalWord, maxApprovalWordLength); err != nil {
		return err
	}
	// 答えは1行として送る。改行が混じると、サーバーが受け取る問答がずれる。
	if strings.ContainsAny(settings.ApprovalWord, " \t\r\n") {
		return fieldError(ErrSettings, "openconnect.approvalWord", ReasonFormat)
	}
	return nil
}

func validateFingerprint(fingerprint string) error {
	if fingerprint == "" {
		return nil
	}
	if err := validateLength("openconnect.serverCertificate", fingerprint, maxFingerprintLength); err != nil {
		return err
	}
	known := false
	for _, prefix := range openConnectFingerprintPrefixes {
		known = known || strings.HasPrefix(fingerprint, prefix)
	}
	if !known || strings.ContainsAny(fingerprint, " \t\r\n\"\\") {
		return fieldError(ErrSettings, "openconnect.serverCertificate", ReasonFormat)
	}
	return nil
}

func (openConnectBackend) validateSecrets(profile Profile, secrets Secrets) error {
	var stored OpenConnectSecrets
	if secrets.OpenConnect != nil {
		stored = *secrets.OpenConnect
	}
	if err := requireSecret("secrets."+SecretKeyOpenConnectPassword, stored.Password, maxSecretLength); err != nil {
		return err
	}
	if profile.OpenConnect == nil || profile.OpenConnect.SecondFactor != SecondFactorTOTP {
		return nil
	}
	field := "secrets." + SecretKeyOpenConnectTOTPSecret
	if err := requireSecret(field, stored.TOTPSecret, maxTOTPSecretLength); err != nil {
		return err
	}
	if _, err := totp.Parse(stored.TOTPSecret); err != nil {
		return fieldError(ErrSecrets, field, ReasonFormat)
	}
	return nil
}

func (openConnectBackend) writeAgentSection(request agentSectionRequest, document *agentDocument) error {
	settings := *request.profile.OpenConnect
	secrets := *request.secrets.OpenConnect
	second, err := secondFactorAnswer(settings, secrets, request.now)
	if err != nil {
		return err
	}
	document.OpenConnect = &openConnectDocument{
		Server:            settings.Server,
		Username:          settings.Username,
		Protocol:          openConnectProtocol(settings),
		ServerCertificate: settings.ServerCertificate,
		Password:          secrets.Password,
		SecondFactor:      second,
		WaitsForApproval:  settings.SecondFactor == SecondFactorApprove,
	}
	return nil
}

func (openConnectBackend) secretValues(secrets Secrets) []string {
	if secrets.OpenConnect == nil {
		return nil
	}
	return []string{secrets.OpenConnect.Password, secrets.OpenConnect.TOTPSecret}
}

func (openConnectBackend) ownSecrets(_ Profile, secrets Secrets) Secrets {
	return Secrets{OpenConnect: secrets.OpenConnect}
}

func (openConnectBackend) waitsForApproval(profile Profile) bool {
	return profile.OpenConnect != nil && profile.OpenConnect.SecondFactor == SecondFactorApprove
}

// openConnectDocument は、agent が openconnect を呼ぶのに要るものである。
// パスワードは agent が標準入力で openconnect へ渡し、引数には置かない。
type openConnectDocument struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	Protocol string `json:"protocol"`
	// ServerCertificate は、サーバーの証明書を固定するフィンガープリントである。空なら公的な
	// 認証局として検証させる。
	ServerCertificate string `json:"serverCertificate"`
	Password          string `json:"password"`
	// SecondFactor は、VPNサーバーが二要素認証の入力を求めたときに送る1行である。空なら
	// 何も送らない。agent は中身を解さず、そのまま openconnect へ渡す。
	SecondFactor string `json:"secondFactor,omitempty"`
	// WaitsForApproval は、この接続が人の承認を待つかである。agent はこれを
	// ログへ書く。何も求めてこないVPNサーバーでは、待っているあいだ出力が止まる。
	// 理由が書いていないと、止まったのか待っているのかが分からない。
	WaitsForApproval bool `json:"waitsForApproval,omitempty"`
}

// openConnectProtocol は、profile が指定した方式、または既定を返す。
func openConnectProtocol(settings OpenConnectSettings) string {
	if settings.Protocol == "" {
		return defaultOpenConnectProtocol
	}
	return settings.Protocol
}

// secondFactorAnswer は、VPNサーバーが求める二要素認証の入力を作る。
//
// openconnect は、パスワードを標準入力から読んだあと、次の質問にも標準入力の
// 次の行を使う。フォームの名前も項目の名前もVPNサーバーごとに違うので、そこへは触れ
// ない。答えが空なら、二要素認証の入力を求めないVPNサーバーだということである。
func secondFactorAnswer(settings OpenConnectSettings, secrets OpenConnectSecrets, now time.Time) (string, error) {
	switch settings.SecondFactor {
	case "":
		return "", nil
	case SecondFactorApprove:
		// 何も求めずに通知だけ出すVPNサーバーには、送る語が無い。空の行を送ると、
		// 二要素認証の入力を求めるVPNサーバーに「空の答え」を渡すことになる。
		return settings.ApprovalWord, nil
	case SecondFactorTOTP:
		config, err := totp.Parse(secrets.TOTPSecret)
		if err != nil {
			return "", fieldError(ErrSecrets, "secrets."+SecretKeyOpenConnectTOTPSecret, ReasonFormat)
		}
		code, err := config.Code(now)
		if err != nil {
			return "", fmt.Errorf("%w: the second factor code could not be made", ErrSecrets)
		}
		return code, nil
	}
	return "", fieldError(ErrSettings, "openconnect.secondFactor", ReasonUnsupported)
}

// secondFactorWait は、コードを作る前に待つ長さである。
//
// TOTP のコードは窓ごとに変わる。窓の終わり間際に作ったコードは、VPNサーバーへ届く
// までに古くなりうる。そのときは次の窓が始まるまで待つ。
func secondFactorWait(profile Profile, secrets Secrets, now time.Time) time.Duration {
	if profile.OpenConnect == nil || profile.OpenConnect.SecondFactor != SecondFactorTOTP ||
		secrets.OpenConnect == nil {
		return 0
	}
	config, err := totp.Parse(secrets.OpenConnect.TOTPSecret)
	if err != nil || config.Period <= 0 {
		return 0
	}
	period := time.Duration(config.Period) * time.Second
	elapsed := time.Duration(now.UnixNano()) % period
	remaining := period - elapsed
	if remaining >= totpMinimumRemaining {
		return 0
	}
	return remaining
}
