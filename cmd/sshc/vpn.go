package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"sshc/internal/vpn"
)

// VPN 経路は engine が持つ。この CLI は入力を集めて engine へ渡し、返ってきた
// 状態を出すだけである。コンテナも Vault も自分では触らない。

const (
	// maxVPNKeyBytes は、受け取る鍵の長さの上限である。base64 の 32 バイト鍵は
	// 44 文字であり、それを超えるものは形が違う。
	maxVPNKeyBytes = 64
	// defaultTunnelAddress は、トンネル側で名乗るアドレスの初期値である。
	defaultTunnelAddress = "10.0.0.2/32"
	// defaultOpenConnectProtocol は、openconnect の方式の初期値である。
	// internal/vpn の既定と同じ語を使う。
	defaultOpenConnectProtocol = "anyconnect"
)

var errVPNSetupInput = errors.New("vpn profile input is invalid")

// vpnOverview は、engine が返す一覧のうち、この CLI が読む部分である。
type vpnOverview struct {
	Available bool         `json:"available"`
	Detail    string       `json:"detail"`
	Profiles  []vpnSession `json:"profiles"`
}

type vpnSession struct {
	Profile vpnStoredProfile `json:"profile"`
	Running bool             `json:"running"`
	// RelaySocket は、中継のソケットの場所である。開いていなければ空になる。
	RelaySocket string     `json:"relaySocket"`
	Connections []string   `json:"connections"`
	Tunnel      *vpnTunnel `json:"tunnel,omitempty"`
	// Phase は、いま経路を用意している段階である。用意していなければ空。
	Phase string `json:"phase,omitempty"`
}

type vpnTunnel struct {
	Interface string `json:"interface,omitempty"`
	Address   string `json:"address,omitempty"`
	Since     string `json:"since,omitempty"`
	Backend   string `json:"backend,omitempty"`
	// TargetAddress は、VPNの中で引けた接続先のアドレスである。
	TargetAddress string `json:"targetAddress,omitempty"`
}

type vpnLogs struct {
	Lines string `json:"lines"`
}

// vpnStoredProfile は、engine が返すプロファイルである。
//
// 応答は未知の項目を許さずに読む。backend ごとの節も含めて、engine が返す形を
// そのまま持つ。表示に使うのは名前と方式と接続先だけだが、持たない項目があると
// 応答そのものを読めない。
type vpnStoredProfile struct {
	Name        string                 `json:"name"`
	Backend     string                 `json:"backend"`
	Target      string                 `json:"target"`
	DNS         []string               `json:"dns,omitempty"`
	WireGuard   *vpnRequestWireGuard   `json:"wireguard,omitempty"`
	L2TP        *vpnRequestL2TP        `json:"l2tp,omitempty"`
	OpenConnect *vpnRequestOpenConnect `json:"openconnect,omitempty"`
}

func runVPN(ctx context.Context, called vpnInvocation, environment commandEnvironment) int {
	stateDir, client, stdin, stdout, stderr, terminal :=
		environment.stateDir, environment.client, environment.stdin, environment.stdout, environment.stderr, environment.terminal
	if err := ctx.Err(); err != nil {
		return finishSyncFailure(called.JSON, err, stdout, stderr)
	}
	if called.Action == vpnProxy {
		return runVPNProxy(ctx, called, environment)
	}
	var prompt *os.File
	if called.Action == vpnAdd {
		var err error
		prompt, err = requireSyncSetupTerminal(stdin, stderr, terminal)
		if err != nil {
			return finishSyncFailure(false, errSyncSetupTTY, stdout, stderr)
		}
	}
	engine, err := openEngineAPI(ctx, stateDir, client)
	if err != nil {
		return finishSyncFailure(called.JSON, err, stdout, stderr)
	}
	defer func() { _ = engine.Close() }()

	// どの操作も、engine は変更後の一覧をそのまま返す。呼び出し側が状態を
	// 取り直す必要はない。
	var overview vpnOverview
	switch called.Action {
	case vpnList:
		if err := engine.getJSON(ctx, "/api/v1/vpn", &overview); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	case vpnAdd:
		if err := addVPNProfile(ctx, engine, called.Name, stdin, prompt, terminal, &overview); err != nil {
			return finishSyncFailure(false, err, stdout, stderr)
		}
	case vpnRemove:
		confirmed, exit := confirmAction(ctx, called.Yes,
			fmt.Sprintf("VPNプロファイル %q と、その秘密と、接続への紐付けを削除しますか？ [y/N] ",
				safeTerminalCell(called.Name)),
			systemActionConfirmer, stderr)
		if exit != 0 {
			return exit
		}
		if !confirmed {
			fmt.Fprintln(stdout, "何も変更していません。")
			return 0
		}
		if err := engine.sendJSON(ctx, http.MethodDelete, vpnProfilePath(called.Name), nil, &overview); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	case vpnUp:
		// 経路が立つまで待つあいだ、何も出ないと止まって見える。初回はイメージの
		// 用意だけで分単位になる。
		if !called.JSON {
			fmt.Fprintf(stderr, "%s のVPN経路を用意しています。初回はイメージの作成に数分かかることがあります…\n",
				safeTerminalCell(called.Name))
		}
		if err := engine.sendJSON(ctx, http.MethodPost, vpnProfilePath(called.Name)+"/session", struct{}{}, &overview); err != nil {
			code := finishSyncFailure(called.JSON, err, stdout, stderr)
			if !called.JSON {
				fmt.Fprintf(stderr, "コンテナの出力は sshc vpn logs %s で読めます。\n", safeTerminalCell(called.Name))
			}
			return code
		}
	case vpnDown:
		if err := engine.sendJSON(ctx, http.MethodDelete, vpnProfilePath(called.Name)+"/session", nil, &overview); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	case vpnRename:
		body := map[string]string{"name": called.Rename}
		if err := engine.sendJSON(ctx, http.MethodPost,
			vpnProfilePath(called.Name)+"/rename", body, &overview); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	case vpnLogsAction:
		var logs vpnLogs
		if err := engine.getJSON(ctx, vpnProfilePath(called.Name)+"/logs", &logs); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
		if called.JSON {
			if err := writeCommandEnvelope(stdout, commandEnvelope{
				SchemaVersion: 1, Success: true, Result: logs,
			}); err != nil {
				return 1
			}
			return 0
		}
		for _, line := range strings.Split(strings.TrimRight(logs.Lines, "\n"), "\n") {
			fmt.Fprintln(stdout, safeTerminalCell(line))
		}
		return 0
	case vpnBind, vpnUnbind:
		body := map[string]string{"alias": called.Alias, "profile": called.Name}
		if err := engine.sendJSON(ctx, http.MethodPut, "/api/v1/vpn/bindings", body, &overview); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	}
	if called.JSON {
		if err := writeCommandEnvelope(stdout, commandEnvelope{
			SchemaVersion: 1, Success: true, Result: overview,
		}); err != nil {
			return 1
		}
		return 0
	}
	writeVPNOverview(stdout, overview)
	return 0
}

func vpnProfilePath(name string) string {
	return "/api/v1/vpn/profiles/" + url.PathEscape(name)
}

// addVPNProfile は、プロファイルひとつ分の入力を集めて engine へ渡す。
//
// 秘密は引数にも環境変数にも置かない。端末から no-echo で読み、送ったあとに
// その場で消す。
func addVPNProfile(
	ctx context.Context, engine *engineAPI, name string,
	stdin, prompt *os.File, terminal passwordTerminal, overview *vpnOverview,
) error {
	backend, err := promptVisibleSetup(ctx, stdin, prompt,
		"Backend [wireguard] (wireguard/l2tp_ipsec): ", "wireguard")
	if err != nil {
		return err
	}
	target, err := promptVisibleSetup(ctx, stdin, prompt, "Target through the VPN (host:port): ", "")
	if err != nil {
		return err
	}
	// 接続先を名前で書くなら、その名前をVPNの中で引くDNSが要る。アドレスで
	// 書くなら空のままでよい。
	resolvers, err := promptVisibleSetup(ctx, stdin, prompt,
		"DNS servers inside the VPN (comma separated, blank for none): ", "")
	if err != nil {
		return err
	}
	profile := vpnRequestProfile{Name: name, Backend: backend, Target: target, DNS: splitVPNResolvers(resolvers)}
	var secrets []vpnSecretField
	switch backend {
	case "wireguard":
		profile.WireGuard, secrets, err = readWireGuardProfile(ctx, stdin, prompt, terminal)
	case "l2tp_ipsec":
		profile.L2TP, secrets, err = readL2TPProfile(ctx, stdin, prompt, terminal)
	case "openconnect":
		profile.OpenConnect, secrets, err = readOpenConnectProfile(ctx, stdin, prompt, terminal)
	default:
		err = fmt.Errorf("%w: backend は wireguard か l2tp_ipsec か openconnect です", errVPNSetupInput)
	}
	defer func() {
		for _, field := range secrets {
			zeroBytes(field.value)
		}
	}()
	if err != nil {
		return err
	}
	if target == "" {
		return errVPNSetupInput
	}
	payload, err := buildVPNProfilePayload(profile, secrets)
	if err != nil {
		return err
	}
	// payload は秘密を含む。sendSecretJSON が送り終えた本文を消す。
	return engine.sendSecretJSON(ctx, http.MethodPut, vpnProfilePath(name), payload, overview)
}

// readWireGuardProfile は、wireguard の設定と秘密鍵を読む。
func readWireGuardProfile(
	ctx context.Context, stdin, prompt *os.File, terminal passwordTerminal,
) (*vpnRequestWireGuard, []vpnSecretField, error) {
	server, err := promptVisibleSetup(ctx, stdin, prompt, "VPN server (host:port): ", "")
	if err != nil {
		return nil, nil, err
	}
	peerKey, err := promptVisibleSetup(ctx, stdin, prompt, "Peer public key: ", "")
	if err != nil {
		return nil, nil, err
	}
	address, err := promptVisibleSetup(ctx, stdin, prompt,
		setupVisibleLabel("Tunnel address", defaultTunnelAddress), defaultTunnelAddress)
	if err != nil {
		return nil, nil, err
	}
	privateKey, err := promptMaskedPassword(ctx, stdin, prompt, terminal, "Private key: ")
	if err != nil {
		zeroBytes(privateKey)
		return nil, nil, err
	}
	secrets := []vpnSecretField{{name: "wireguardPrivateKey", value: privateKey}}
	if server == "" || peerKey == "" || address == "" {
		return nil, secrets, errVPNSetupInput
	}
	if len(privateKey) > maxVPNKeyBytes || !base64KeyBytes(privateKey) {
		return nil, secrets, fmt.Errorf("%w: 秘密鍵の形が違います", errVPNSetupInput)
	}
	return &vpnRequestWireGuard{Server: server, PeerPublicKey: peerKey, Address: address}, secrets, nil
}

// readL2TPProfile は、L2TP/IPsec の設定と二つの秘密を読む。
func readL2TPProfile(
	ctx context.Context, stdin, prompt *os.File, terminal passwordTerminal,
) (*vpnRequestL2TP, []vpnSecretField, error) {
	server, err := promptVisibleSetup(ctx, stdin, prompt, "VPN server (host): ", "")
	if err != nil {
		return nil, nil, err
	}
	username, err := promptVisibleSetup(ctx, stdin, prompt, "VPN username: ", "")
	if err != nil {
		return nil, nil, err
	}
	// 暗号方式は、古い装置と合わないときだけ書く。空なら strongSwan の既定に任せる。
	ike, err := promptVisibleSetup(ctx, stdin, prompt, "IKE proposals (blank for the default): ", "")
	if err != nil {
		return nil, nil, err
	}
	esp, err := promptVisibleSetup(ctx, stdin, prompt, "ESP proposals (blank for the default): ", "")
	if err != nil {
		return nil, nil, err
	}
	password, err := promptMaskedPassword(ctx, stdin, prompt, terminal, "VPN password: ")
	if err != nil {
		zeroBytes(password)
		return nil, nil, err
	}
	psk, err := promptMaskedPassword(ctx, stdin, prompt, terminal, "IPsec pre-shared key: ")
	if err != nil {
		zeroBytes(password)
		zeroBytes(psk)
		return nil, nil, err
	}
	secrets := []vpnSecretField{
		{name: "l2tpPassword", value: password},
		{name: "ipsecPsk", value: psk},
	}
	if server == "" || username == "" || len(password) == 0 || len(psk) == 0 {
		return nil, secrets, errVPNSetupInput
	}
	return &vpnRequestL2TP{Server: server, Username: username, IKE: ike, ESP: esp}, secrets, nil
}

// readOpenConnectProfile は、openconnect の設定とパスワードを読む。
func readOpenConnectProfile(
	ctx context.Context, stdin, prompt *os.File, terminal passwordTerminal,
) (*vpnRequestOpenConnect, []vpnSecretField, error) {
	server, err := promptVisibleSetup(ctx, stdin, prompt, "VPN server (host): ", "")
	if err != nil {
		return nil, nil, err
	}
	username, err := promptVisibleSetup(ctx, stdin, prompt, "VPN username: ", "")
	if err != nil {
		return nil, nil, err
	}
	protocol, err := promptVisibleSetup(ctx, stdin, prompt,
		setupVisibleLabel("Protocol", defaultOpenConnectProtocol), defaultOpenConnectProtocol)
	if err != nil {
		return nil, nil, err
	}
	// 自己署名の装置では指紋が要る。公的な認証局の証明書なら空でよい。
	certificate, err := promptVisibleSetup(ctx, stdin, prompt,
		"Server certificate fingerprint (sha256:... or pin-sha256:..., blank to verify normally): ", "")
	if err != nil {
		return nil, nil, err
	}
	// パスワードのあとに装置がすること。approve は電話の承認を待ち、totp は
	// Vault に置いた種からコードを作る。
	secondFactor, err := promptVisibleSetup(ctx, stdin, prompt,
		"Second factor (none/approve/totp) [none]: ", "none")
	if err != nil {
		return nil, nil, err
	}
	settings := &vpnRequestOpenConnect{
		Server: server, Username: username, Protocol: protocol, ServerCertificate: certificate,
	}
	if secondFactor != "none" {
		settings.SecondFactor = secondFactor
	}
	if settings.SecondFactor == vpn.SecondFactorApprove {
		// 二段目を聞いてくる装置にだけ語を送る。パスワードだけで通知を出す
		// 装置には何も送らない。
		word, err := promptVisibleSetup(ctx, stdin, prompt,
			"Word to send if the device asks a second question (blank to send nothing, Duo often takes push): ", "")
		if err != nil {
			return nil, nil, err
		}
		settings.ApprovalWord = word
	}
	password, err := promptMaskedPassword(ctx, stdin, prompt, terminal, "VPN password: ")
	if err != nil {
		zeroBytes(password)
		return nil, nil, err
	}
	secrets := []vpnSecretField{{name: "openconnectPassword", value: password}}
	if settings.SecondFactor == vpn.SecondFactorTOTP {
		seed, err := promptMaskedPassword(ctx, stdin, prompt, terminal, "Second factor TOTP secret: ")
		if err != nil {
			zeroBytes(password)
			zeroBytes(seed)
			return nil, nil, err
		}
		secrets = append(secrets, vpnSecretField{name: "openconnectTotpSecret", value: seed})
		if len(seed) == 0 {
			return nil, secrets, errVPNSetupInput
		}
	}
	if server == "" || username == "" || len(password) == 0 {
		return nil, secrets, errVPNSetupInput
	}
	return settings, secrets, nil
}

// vpnRequestProfile は、保存要求のうち秘密でない部分である。API の形と揃える。
type vpnRequestProfile struct {
	Name        string                 `json:"name"`
	Backend     string                 `json:"backend"`
	Target      string                 `json:"target"`
	DNS         []string               `json:"dns,omitempty"`
	WireGuard   *vpnRequestWireGuard   `json:"wireguard,omitempty"`
	L2TP        *vpnRequestL2TP        `json:"l2tp,omitempty"`
	OpenConnect *vpnRequestOpenConnect `json:"openconnect,omitempty"`
}

type vpnRequestOpenConnect struct {
	Server            string `json:"server"`
	Username          string `json:"username"`
	Protocol          string `json:"protocol,omitempty"`
	ServerCertificate string `json:"serverCertificate,omitempty"`
	SecondFactor      string `json:"secondFactor,omitempty"`
	ApprovalWord      string `json:"approvalWord,omitempty"`
}

type vpnRequestWireGuard struct {
	Server        string `json:"server"`
	PeerPublicKey string `json:"peerPublicKey"`
	Address       string `json:"address"`
}

type vpnRequestL2TP struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	IKE      string `json:"ike,omitempty"`
	ESP      string `json:"esp,omitempty"`
}

// vpnSecretField は、本文へ書く秘密ひとつである。値は []byte のまま運び、送った
// あとに消せるようにする。Go の文字列にすると、消せる場所がなくなる。
type vpnSecretField struct {
	name  string
	value []byte
}

// buildVPNProfilePayload は、保存要求の本文を組み立てる。
func buildVPNProfilePayload(profile vpnRequestProfile, secrets []vpnSecretField) ([]byte, error) {
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

// writeVPNOverview は、一覧と状態を人向けに書く。
func writeVPNOverview(out io.Writer, overview vpnOverview) {
	if !overview.Available {
		fmt.Fprintf(out, "この機械ではVPN経路を作れません: %s\n\n", safeTerminalCell(overview.Detail))
	}
	if len(overview.Profiles) == 0 {
		fmt.Fprintln(out, "VPNプロファイルはありません。sshc vpn add <名前> で作成します。")
		return
	}
	rows := make([][2]string, 0, len(overview.Profiles)*2)
	for _, session := range overview.Profiles {
		state := "stopped"
		switch {
		case session.RelaySocket != "":
			state = "up"
		case session.Phase != "":
			state = "starting: " + vpnPhaseWord(session.Phase)
		case session.Running:
			state = "starting"
		}
		connections := "-"
		if len(session.Connections) > 0 {
			connections = strings.Join(session.Connections, ", ")
		}
		rows = append(rows,
			[2]string{session.Profile.Name, fmt.Sprintf("%s  %s  %s",
				session.Profile.Backend, session.Profile.Target, state)},
			[2]string{"", "connections: " + connections},
		)
		if session.Tunnel != nil && session.Tunnel.Interface != "" {
			rows = append(rows, [2]string{"", fmt.Sprintf("tunnel: %s %s since %s",
				session.Tunnel.Interface, session.Tunnel.Address, session.Tunnel.Since)})
			if session.Tunnel.TargetAddress != "" && session.Tunnel.TargetAddress != session.Profile.Target {
				rows = append(rows, [2]string{"", "target address: " + session.Tunnel.TargetAddress})
			}
		}
		if len(session.Profile.DNS) > 0 {
			rows = append(rows, [2]string{"", "dns: " + strings.Join(session.Profile.DNS, ", ")})
		}
	}
	writeSyncRows(out, rows)
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

// vpnPhaseWord は、経路を用意している段階を人向けの一語に直す。
func vpnPhaseWord(phase string) string {
	switch phase {
	case "image":
		return "building the image"
	case "container":
		return "starting the container"
	case "tunnel":
		return "waiting for the tunnel"
	case "approval":
		return "waiting for approval on the phone"
	}
	return phase
}

// errVPNRelayMissing は、engine が経路を差し出さなかったことを表す。
var errVPNRelayMissing = errors.New("the engine did not open a relay for that VPN profile")

// vpnRouteThroughEngine は、engine に経路を起こさせ、その中継のソケットへ繋ぐ。
//
// CLI は秘密を持たない。コンテナも Vault も engine が持ち、こちらは利用者だけが
// 開けるソケットへ繋ぐだけである。
func vpnRouteThroughEngine(stateDir string, client *http.Client) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, profile, address string) (net.Conn, error) {
		return dialVPNRelay(ctx, stateDir, client, profile, address)
	}
}

// dialVPNRelay は、名前の付いた経路を起こし、その中継のソケットへ繋ぐ。
//
// wantedTarget が空でなければ、その相手へ行く経路であることを確かめてから繋ぐ。
func dialVPNRelay(
	ctx context.Context, stateDir string, client *http.Client, profile, wantedTarget string,
) (net.Conn, error) {
	engine, err := openEngineAPI(ctx, stateDir, client)
	if err != nil {
		return nil, err
	}
	defer func() { _ = engine.Close() }()
	var overview vpnOverview
	if err := engine.sendJSON(ctx, http.MethodPost,
		vpnProfilePath(profile)+"/session", struct{}{}, &overview); err != nil {
		return nil, err
	}
	for _, session := range overview.Profiles {
		if session.Profile.Name != profile {
			continue
		}
		if wantedTarget != "" && session.Profile.Target != wantedTarget {
			return nil, fmt.Errorf("%w: %s は %s へ繋ぐ経路である",
				vpn.ErrTargetMismatch, profile, session.Profile.Target)
		}
		if session.RelaySocket == "" {
			return nil, errVPNRelayMissing
		}
		return (&net.Dialer{}).DialContext(ctx, "unix", session.RelaySocket)
	}
	return nil, errVPNRelayMissing
}

// runVPNProxy は、標準入出力をその経路の中継へ繋ぐ。
//
// ホストの ssh・scp・git が ProxyCommand として使うための口である。SSH の
// 握手も鍵もそれらの側にあり、こちらが運ぶのはバイト列だけである。
func runVPNProxy(ctx context.Context, called vpnInvocation, environment commandEnvironment) int {
	// 標準出力はデータの通り道である。案内も診断もここへは書かない。
	relay, err := dialVPNRelay(ctx, environment.stateDir, environment.client, called.Name, called.Target)
	if err != nil {
		// この2つは engine の拒否ではなく、こちらで分かる食い違いである。
		// 共通の言い換えに通すと、何が食い違ったのかが消える。
		if errors.Is(err, vpn.ErrTargetMismatch) || errors.Is(err, errVPNRelayMissing) {
			fmt.Fprintf(environment.stderr, "sshc: %v\n", err)
			return 1
		}
		return finishSyncFailure(false, err, environment.stderr, environment.stderr)
	}
	defer func() { _ = relay.Close() }()

	fromRelay := make(chan error, 1)
	go func() {
		_, err := io.Copy(environment.stdout, relay)
		fromRelay <- err
	}()
	if _, err := io.Copy(relay, environment.stdin); err != nil {
		fmt.Fprintf(environment.stderr, "sshc: VPN経路への書き込みが止まりました: %v\n", err)
		return 1
	}
	// 送る側が終わったことを相手へ伝える。伝えないと、相手は入力の終わりを
	// 待ち続ける。
	if half, ok := relay.(interface{ CloseWrite() error }); ok {
		_ = half.CloseWrite()
	}
	if err := <-fromRelay; err != nil {
		fmt.Fprintf(environment.stderr, "sshc: VPN経路からの読み取りが止まりました: %v\n", err)
		return 1
	}
	return 0
}
