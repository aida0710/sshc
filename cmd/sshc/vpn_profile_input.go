package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"sshc/internal/application"
	"sshc/internal/httpserver"
	"sshc/internal/vpn"
)

// VPN プロファイルの作成（sshc vpn add）と編集（sshc vpn edit）で、ターミナルから
// 入力を読み、engine へ渡す。
//
// 秘密は引数にも環境変数にも置かない。ターミナルから no-echo で読み、送ったあとに
// その場で消す。

const (
	// maxVPNKeyBytes は、受け取る鍵の長さの上限である。base64 の 32 バイト鍵は
	// 44 文字であり、それを超えるものは形が違う。
	maxVPNKeyBytes = 64
	// defaultTunnelAddress は、トンネル側で名乗るアドレスの初期値である。
	defaultTunnelAddress = "10.0.0.2/32"
	// defaultOpenConnectProtocol は、openconnect のプロトコルの初期値である。
	// internal/vpn の既定と同じ語を使う。
	defaultOpenConnectProtocol = "anyconnect"
	// clearWord は、編集で、任意の項目の保存済みの値を消すときに入力する語である。
	// 空欄は「保存済みの値のまま」を表すので、消すには別の語が要る。
	clearWord = "-"
	// secondFactorNone は、二要素認証を使わないことを表す入力の語である。
	secondFactorNone = "none"
)

var errVPNSetupInput = errors.New("vpn profile input is invalid")

// vpnInputError は、engine へ送る前に CLI が見つけた誤りである。engine の拒否とは
// 別に、そのまま人向けの文として出す。
type vpnInputError struct {
	sentence string
	// cause は、errors.Is で見分けるための分類である。
	cause error
}

func (failure *vpnInputError) Error() string { return failure.sentence }

func (failure *vpnInputError) Unwrap() error { return failure.cause }

// 入力の誤りの文である。
var (
	errVPNInputMissing = &vpnInputError{sentence: "必須の項目が入力されていません。", cause: errVPNSetupInput}
	errVPNInputBackend = &vpnInputError{sentence: "方式には wireguard、l2tp_ipsec、openconnect のいずれかを指定してください。", cause: errVPNSetupInput}
	errVPNInputKey     = &vpnInputError{sentence: "秘密鍵の形式が正しくありません。", cause: errVPNSetupInput}
	errVPNInputUnknown = &vpnInputError{sentence: "指定したVPNプロファイルが見つかりません。sshc vpn で名前を確認してください。", cause: errVPNSetupInput}
)

// vpnProfilePrompter は、プロファイルひとつ分の入力をターミナルから読む。
//
// 編集では、表示する項目の初期値を保存済みの値にし、空欄のシークレットは保存済みの
// 値のまま残す。
type vpnProfilePrompter struct {
	ctx      context.Context
	stdin    *os.File
	prompt   *os.File
	terminal passwordTerminal
	// editing は、保存済みのプロファイルを編集していることを表す。
	editing bool
}

// required は、必須の項目を読む。current が空でなければ初期値にする。
func (p vpnProfilePrompter) required(label, current string) (string, error) {
	if current == "" {
		return promptVisibleSetup(p.ctx, p.stdin, p.prompt, label+": ", "")
	}
	return promptVisibleSetup(p.ctx, p.stdin, p.prompt, setupVisibleLabel(label, current), current)
}

// optional は、任意の項目を読む。編集では、空欄は保存済みの値のまま、clearWord は
// 値を消す。作成では、空欄は値なしである。
func (p vpnProfilePrompter) optional(label, current string) (string, error) {
	if !p.editing || current == "" {
		return promptVisibleSetup(p.ctx, p.stdin, p.prompt, label+" (blank for none): ", "")
	}
	value, err := promptVisibleSetup(p.ctx, p.stdin, p.prompt,
		setupVisibleLabel(label+" ("+clearWord+" to clear)", current), current)
	if err != nil || value != clearWord {
		return value, err
	}
	return "", nil
}

// secret は、シークレットをひとつ no-echo で読む。keeps が真なら、空欄は保存済みの
// 値を残すことを案内する。
func (p vpnProfilePrompter) secret(label string, keeps bool) ([]byte, error) {
	if keeps {
		label += " (blank keeps the saved value)"
	}
	value, err := promptMaskedPassword(p.ctx, p.stdin, p.prompt, p.terminal, label+": ")
	if err != nil {
		zeroBytes(value)
		return nil, err
	}
	return value, nil
}

// vpnProfileInput は、読み取ったプロファイルと、送るシークレットである。
type vpnProfileInput struct {
	profile application.VPNProfile
	// secrets は、送るシークレットである。編集で空欄にした項目は含まない。
	secrets []vpnSecretField
}

// forget は、読み取ったシークレットを消す。
func (input vpnProfileInput) forget() {
	for _, field := range input.secrets {
		zeroBytes(field.value)
	}
}

// readVPNProfile は、プロファイルひとつ分の入力を読む。current が nil なら作成、
// そうでなければ編集で、current の値を初期値にする。
func readVPNProfile(p vpnProfilePrompter, name string, current *application.VPNProfile) (vpnProfileInput, error) {
	previous := application.VPNProfile{Backend: vpn.WireGuard}
	if current != nil {
		previous = *current
	}
	backend, err := p.required("Backend (wireguard/l2tp_ipsec/openconnect)", string(previous.Backend))
	if err != nil {
		return vpnProfileInput{}, err
	}
	// 接続先をホスト名で書く接続に使うなら、その名前をVPNの中で名前解決する
	// DNSサーバーが要る。アドレスで書く接続にしか使わないなら空のままでよい。
	resolvers, err := p.optional("DNS servers inside the VPN (comma separated)", strings.Join(previous.DNS, ","))
	if err != nil {
		return vpnProfileInput{}, err
	}
	input := vpnProfileInput{profile: application.VPNProfile{
		Name: name, Backend: vpn.BackendName(backend), DNS: splitVPNResolvers(resolvers),
	}}
	// 方式を変えたら、前の方式のシークレットは使えない。新しい方式のものを入力させる。
	keeps := p.editing && previous.Backend == input.profile.Backend
	switch input.profile.Backend {
	case vpn.WireGuard:
		input.profile.WireGuard, input.secrets, err = readWireGuardProfile(p, previous.WireGuard, keeps)
	case vpn.L2TPIPsec:
		input.profile.L2TP, input.secrets, err = readL2TPProfile(p, previous.L2TP, keeps)
	case vpn.OpenConnect:
		input.profile.OpenConnect, input.secrets, err = readOpenConnectProfile(p, previous.OpenConnect, keeps)
	default:
		err = errVPNInputBackend
	}
	if err != nil {
		input.forget()
		return vpnProfileInput{}, err
	}
	return input, nil
}

// requireSecret は、シークレットが空でないかを確かめる。keeps が真なら、空欄は
// 保存済みの値を残すので通す。
func requireSecret(value []byte, keeps bool) error {
	if len(value) == 0 && !keeps {
		return errVPNInputMissing
	}
	return nil
}

// sentSecrets は、送るシークレットだけを並べる。空欄のものは送らない。
func sentSecrets(fields ...vpnSecretField) []vpnSecretField {
	sent := make([]vpnSecretField, 0, len(fields))
	for _, field := range fields {
		if len(field.value) != 0 {
			sent = append(sent, field)
		}
	}
	return sent
}

// readWireGuardProfile は、wireguard の設定と秘密鍵を読む。
func readWireGuardProfile(
	p vpnProfilePrompter, current *application.WireGuardProfile, keeps bool,
) (*application.WireGuardProfile, []vpnSecretField, error) {
	previous := application.WireGuardProfile{Address: defaultTunnelAddress}
	if current != nil {
		previous = *current
	}
	server, err := p.required("VPN server (host:port)", previous.Server)
	if err != nil {
		return nil, nil, err
	}
	peerKey, err := p.required("Peer public key", previous.PeerPublicKey)
	if err != nil {
		return nil, nil, err
	}
	address, err := p.required("Tunnel address", previous.Address)
	if err != nil {
		return nil, nil, err
	}
	privateKey, err := p.secret("Private key", keeps)
	if err != nil {
		return nil, nil, err
	}
	secrets := sentSecrets(vpnSecretField{name: vpn.SecretKeyWireGuardPrivateKey, value: privateKey})
	if server == "" || peerKey == "" || address == "" {
		return nil, secrets, errVPNInputMissing
	}
	if err := requireSecret(privateKey, keeps); err != nil {
		return nil, secrets, err
	}
	if len(privateKey) != 0 && (len(privateKey) > maxVPNKeyBytes || !base64KeyBytes(privateKey)) {
		return nil, secrets, errVPNInputKey
	}
	return &application.WireGuardProfile{Server: server, PeerPublicKey: peerKey, Address: address}, secrets, nil
}

// readL2TPProfile は、L2TP/IPsec の設定と二つのシークレットを読む。
func readL2TPProfile(
	p vpnProfilePrompter, current *application.L2TPProfile, keeps bool,
) (*application.L2TPProfile, []vpnSecretField, error) {
	var previous application.L2TPProfile
	if current != nil {
		previous = *current
	}
	server, err := p.required("VPN server (host)", previous.Server)
	if err != nil {
		return nil, nil, err
	}
	username, err := p.required("VPN username", previous.Username)
	if err != nil {
		return nil, nil, err
	}
	// 暗号スイートは、古いVPN機器と合わないときだけ書く。空なら strongSwan の既定に任せる。
	ike, err := p.optional("IKE proposals", previous.IKE)
	if err != nil {
		return nil, nil, err
	}
	esp, err := p.optional("ESP proposals", previous.ESP)
	if err != nil {
		return nil, nil, err
	}
	password, err := p.secret("VPN password", keeps)
	if err != nil {
		return nil, nil, err
	}
	psk, err := p.secret("IPsec pre-shared key", keeps)
	if err != nil {
		zeroBytes(password)
		return nil, nil, err
	}
	secrets := sentSecrets(
		vpnSecretField{name: vpn.SecretKeyL2TPPassword, value: password},
		vpnSecretField{name: vpn.SecretKeyIPsecPSK, value: psk},
	)
	if server == "" || username == "" {
		return nil, secrets, errVPNInputMissing
	}
	if requireSecret(password, keeps) != nil || requireSecret(psk, keeps) != nil {
		return nil, secrets, errVPNInputMissing
	}
	return &application.L2TPProfile{Server: server, Username: username, IKE: ike, ESP: esp}, secrets, nil
}

// readOpenConnectProfile は、openconnect の設定とパスワードを読む。
func readOpenConnectProfile(
	p vpnProfilePrompter, current *application.OpenConnectProfile, keeps bool,
) (*application.OpenConnectProfile, []vpnSecretField, error) {
	previous := application.OpenConnectProfile{Protocol: defaultOpenConnectProtocol}
	if current != nil {
		previous = *current
	}
	server, err := p.required("VPN server (host)", previous.Server)
	if err != nil {
		return nil, nil, err
	}
	username, err := p.required("VPN username", previous.Username)
	if err != nil {
		return nil, nil, err
	}
	protocol, err := p.required("Protocol", previous.Protocol)
	if err != nil {
		return nil, nil, err
	}
	// 自己署名の証明書のサーバーではフィンガープリントが要る。公的な認証局の
	// 証明書なら空でよい。
	certificate, err := p.optional("Server certificate fingerprint (sha256:... or pin-sha256:...)",
		previous.ServerCertificate)
	if err != nil {
		return nil, nil, err
	}
	// パスワードのあとにサーバーが求めること。approve はスマートフォンでの承認を
	// 待ち、totp は Vault に置いたシークレットからコードを作る。
	secondFactor := previous.SecondFactor
	if secondFactor == "" {
		secondFactor = secondFactorNone
	}
	if secondFactor, err = p.required("Second factor (none/approve/totp)", secondFactor); err != nil {
		return nil, nil, err
	}
	settings := &application.OpenConnectProfile{
		Server: server, Username: username, Protocol: protocol, ServerCertificate: certificate,
	}
	if secondFactor != secondFactorNone {
		settings.SecondFactor = secondFactor
	}
	if settings.SecondFactor == vpn.SecondFactorApprove {
		// 二つ目の質問を求めるサーバーにだけ語を送る。パスワードだけで通知を出す
		// サーバーには何も送らない。
		word, err := p.optional("Word to send if the server asks a second question (Duo often takes push)",
			previous.ApprovalWord)
		if err != nil {
			return nil, nil, err
		}
		settings.ApprovalWord = word
	}
	password, err := p.secret("VPN password", keeps)
	if err != nil {
		return nil, nil, err
	}
	fields := []vpnSecretField{{name: vpn.SecretKeyOpenConnectPassword, value: password}}
	// TOTP を使い始めるときは、保存済みのシークレットが無いので入力させる。
	keepsSeed := keeps && previous.SecondFactor == vpn.SecondFactorTOTP
	if settings.SecondFactor == vpn.SecondFactorTOTP {
		seed, err := p.secret("Second factor TOTP secret", keepsSeed)
		if err != nil {
			zeroBytes(password)
			return nil, nil, err
		}
		fields = append(fields, vpnSecretField{name: vpn.SecretKeyOpenConnectTOTPSecret, value: seed})
		if err := requireSecret(seed, keepsSeed); err != nil {
			return nil, sentSecrets(fields...), err
		}
	}
	secrets := sentSecrets(fields...)
	if server == "" || username == "" {
		return nil, secrets, errVPNInputMissing
	}
	if err := requireSecret(password, keeps); err != nil {
		return nil, secrets, err
	}
	return settings, secrets, nil
}

// vpnSecretField は、本文へ書く秘密ひとつである。値は []byte のまま運び、送った
// あとに消せるようにする。Go の文字列にすると、消せる場所がなくなる。
type vpnSecretField struct {
	name  string
	value []byte
}

// buildVPNProfilePayload は、保存要求の本文を組み立てる。
func buildVPNProfilePayload(profile application.VPNProfile, secrets []vpnSecretField) ([]byte, error) {
	encoded, err := json.Marshal(profile)
	if err != nil {
		return nil, err
	}
	payload := append([]byte(`{"profile":`), encoded...)
	payload = append(payload, []byte(`,"secrets":{`)...)
	for index, field := range secrets {
		if index != 0 {
			payload = append(payload, ',')
		}
		payload = append(payload, '"')
		payload = append(payload, field.name...)
		payload = append(payload, '"', ':')
		if payload, err = appendVaultJSONString(payload, field.value); err != nil {
			return nil, err
		}
	}
	return append(payload, []byte(`}}`)...), nil
}

// base64KeyBytes は、鍵が base64 の字だけでできているかを報告する。
func base64KeyBytes(key []byte) bool {
	if len(key) == 0 {
		return false
	}
	for _, character := range key {
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		digit := character >= '0' && character <= '9'
		if !letter && !digit && character != '+' && character != '/' && character != '=' {
			return false
		}
	}
	return true
}

// splitVPNResolvers は、読み取った DNS の並びを一件ずつに分ける。空なら無し。
func splitVPNResolvers(value string) []string {
	resolvers := make([]string, 0, 3)
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			resolvers = append(resolvers, trimmed)
		}
	}
	if len(resolvers) == 0 {
		return nil
	}
	return resolvers
}

// createVPNProfile は、新しいプロファイルを作るよう engine へ頼む。同じ名前が
// あれば engine は断る。add で既存のプロファイルを黙って上書きしない。
//
// payload は秘密を含む。sendSecretJSON が送り終えた本文を消す。
func createVPNProfile(ctx context.Context, engine *engineAPI, payload []byte, overview *httpserver.VPNOverview) error {
	return engine.sendSecretJSON(ctx, http.MethodPost, vpnProfilesPath, payload, overview)
}

// updateVPNProfile は、保存済みのプロファイルを更新するよう engine へ頼む。本文に
// 含めなかったシークレットは、engine が保存済みの値のまま残す。
//
// payload は秘密を含む。sendSecretJSON が送り終えた本文を消す。
func updateVPNProfile(
	ctx context.Context, engine *engineAPI, name string, payload []byte, overview *httpserver.VPNOverview,
) error {
	return engine.sendSecretJSON(ctx, http.MethodPut, vpnProfilePath(name), payload, overview)
}

// addVPNProfile は、プロファイルひとつ分の入力を集め、作成を engine へ頼む。
func addVPNProfile(ctx context.Context, engine *engineAPI, p vpnProfilePrompter, name string,
	overview *httpserver.VPNOverview,
) error {
	input, err := readVPNProfile(p, name, nil)
	if err != nil {
		return err
	}
	defer input.forget()
	payload, err := buildVPNProfilePayload(input.profile, input.secrets)
	if err != nil {
		return err
	}
	return createVPNProfile(ctx, engine, payload, overview)
}

// editVPNProfile は、保存済みの値を初期値にして入力を集め、更新を engine へ頼む。
func editVPNProfile(ctx context.Context, engine *engineAPI, p vpnProfilePrompter, name string,
	overview *httpserver.VPNOverview,
) error {
	var listed httpserver.VPNOverview
	if err := engine.getJSON(ctx, "/api/v1/vpn", &listed); err != nil {
		return err
	}
	current, err := savedVPNProfile(listed, name)
	if err != nil {
		return err
	}
	p.editing = true
	input, err := readVPNProfile(p, name, &current)
	if err != nil {
		return err
	}
	defer input.forget()
	payload, err := buildVPNProfilePayload(input.profile, input.secrets)
	if err != nil {
		return err
	}
	return updateVPNProfile(ctx, engine, name, payload, overview)
}

// savedVPNProfile は、engine の一覧から name のプロファイルを探す。
func savedVPNProfile(overview httpserver.VPNOverview, name string) (application.VPNProfile, error) {
	for _, entry := range overview.Profiles {
		if entry.Profile.Name == name {
			return entry.Profile, nil
		}
	}
	return application.VPNProfile{}, errVPNInputUnknown
}
