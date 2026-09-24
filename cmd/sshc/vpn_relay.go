package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"sshc/internal/connectionlog"
	"sshc/internal/httpserver"
	"sshc/internal/vpn"
)

// VPN 経路の中継へ繋ぐ。`sshc <接続先>` と、ホストの ssh が ProxyCommand として
// 使う `sshc vpn proxy` が使う。

// errVPNRelayMissing は、engine が経路を差し出さなかったことを表す。
var errVPNRelayMissing = errors.New("the engine did not open a relay for that VPN profile")

// vpnRouteThroughEngine は、engine に経路を起こさせ、その中継から接続先へ繋ぐ。
//
// CLI は秘密を持たない。コンテナも Vault も engine が持ち、こちらは利用者だけが
// 開けるソケットへ繋ぐだけである。
func vpnRouteThroughEngine(stateDir string, client *http.Client) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, profile, address string) (net.Conn, error) {
		connection, err := dialVPNRelay(ctx, stateDir, client, vpnRelayRequest{profile: profile, address: address})
		if err != nil {
			copyEngineRecord(ctx, engineRecordRequest{stateDir: stateDir, client: client, profile: profile},
				connectionlog.Detailed)
			return nil, describedVPNRouteError(profile, err)
		}
		copyEngineRecord(ctx, engineRecordRequest{stateDir: stateDir, client: client, profile: profile},
			connectionlog.Full)
		return connection, nil
	}
}

// maxCopiedRecordLines は、sshcエンジンの記録から CLI の接続ログへ写す行数の上限である。
// 1回の接続の試みが収まる長さにする。
const maxCopiedRecordLines = 80

// engineRecordRequest は、sshcエンジンの記録を取り寄せる先である。
type engineRecordRequest struct {
	stateDir string
	client   *http.Client
	profile  string
}

// copyEngineRecord は、VPN 経路について sshcエンジンが行ったことの記録を、CLI の
// 接続ログへ写す。
//
// CLI の接続では、経路の準備（docker、イメージ、コンテナ）は sshcエンジンの中で
// 行われ、CLI の接続ログには何も出ない。失敗したときは debug2 から、成功した
// ときは debug3 から写す。記録を読めなくても、接続の成否は変えない。
func copyEngineRecord(ctx context.Context, request engineRecordRequest, level connectionlog.Level) {
	if !connectionlog.Enabled(ctx, level) {
		return
	}
	engine, err := openEngineAPI(ctx, request.stateDir, request.client)
	if err != nil {
		connectionlog.Say(ctx, level, "sshcエンジンの記録を読めませんでした：%v", err)
		return
	}
	defer func() { _ = engine.Close() }()
	var logs httpserver.VPNLogs
	if err := engine.getJSON(ctx, vpnProfilePath(request.profile)+"/logs", &logs); err != nil {
		connectionlog.Say(ctx, level, "sshcエンジンの記録を読めませんでした：%v", err)
		return
	}
	lines := engineRecordLines(logs.Lines)
	if len(lines) > maxCopiedRecordLines {
		lines = lines[len(lines)-maxCopiedRecordLines:]
	}
	connectionlog.Say(ctx, level, "sshcエンジンの記録（VPNプロファイル %s、最後の%d行まで）：",
		safeTerminalCell(request.profile), maxCopiedRecordLines)
	for _, line := range lines {
		connectionlog.Say(ctx, level, "  %s", safeTerminalCell(line))
	}
}

// engineRecordLines は、sshc vpn logs の出力から、sshcエンジンの記録の行だけを
// 取り出す（internal/vpn の joinLogSections の形）。
func engineRecordLines(logs string) []string {
	const recordHeading, containerHeading = "== sshcエンジンの記録 ==", "== コンテナのログ =="
	_, record, found := strings.Cut(logs, recordHeading)
	if !found {
		return nil
	}
	record, _, _ = strings.Cut(record, containerHeading)
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(record), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// vpnRelayRequest は、engine の中継へ頼む経路と接続先である。
type vpnRelayRequest struct {
	profile string
	// address は、接続先（`host:port`）である。
	address string
}

// dialVPNRelay は、名前の付いた経路を起こし、その中継から接続先へ繋ぐ。
//
// 中継へは1行目に接続先を送り、engine が繋げたと答えてから、バイト列を運ぶ。
// 繋げなかったときは、engine が答えた理由を engineProblem として返す。
func dialVPNRelay(ctx context.Context, stateDir string, client *http.Client, request vpnRelayRequest) (net.Conn, error) {
	relaySocket, err := startVPNRoute(ctx, stateDir, client, request.profile)
	if err != nil {
		return nil, err
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", relaySocket)
	if err != nil {
		return nil, err
	}
	reply, err := askVPNRelay(ctx, connection, request.address)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	if reply.Code != "" {
		_ = connection.Close()
		return nil, engineProblem{Code: reply.Code, Reason: reply.Reason}
	}
	return connection, nil
}

// startVPNRoute は、engine に経路を起こさせ、その中継のソケットの場所を返す。
func startVPNRoute(ctx context.Context, stateDir string, client *http.Client, profile string) (string, error) {
	engine, err := openEngineAPI(ctx, stateDir, client)
	if err != nil {
		return "", err
	}
	defer func() { _ = engine.Close() }()
	// 経路を起こし終えるまで、初回はイメージの作成で数分かかる。待つあいだ、
	// 時間のかかる段階に入ったことを知らせる。
	watching, stopWatching := context.WithCancel(ctx)
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		announcePhases(watching, phaseWatch{engine: engine, profile: profile, interval: phasePollInterval})
	}()
	var overview httpserver.VPNOverview
	err = engine.sendJSON(ctx, http.MethodPost, vpnProfilePath(profile)+"/session", nil, &overview)
	stopWatching()
	<-watched
	if err != nil {
		return "", err
	}
	for _, session := range overview.Profiles {
		if session.Profile.Name == profile && session.RelaySocket != "" {
			return session.RelaySocket, nil
		}
	}
	return "", errVPNRelayMissing
}

// askVPNRelay は、engine の中継へ接続先を伝え、その答えを読む。ctx が終われば
// 待つのをやめる。
func askVPNRelay(ctx context.Context, connection net.Conn, address string) (vpn.RelayReply, error) {
	stop := context.AfterFunc(ctx, func() { _ = connection.SetDeadline(time.Now()) })
	defer stop()
	if err := vpn.WriteRelayRequest(connection, address); err != nil {
		return vpn.RelayReply{}, err
	}
	reply, err := vpn.ReadRelayReply(connection)
	if cause := ctx.Err(); cause != nil {
		return vpn.RelayReply{}, cause
	}
	return reply, err
}

// runVPNProxy は、標準入出力をその経路の中継へ繋ぐ。
//
// ホストの ssh・scp・git が ProxyCommand として使うための口である。SSH の
// 握手も鍵もそれらの側にあり、こちらが運ぶのはバイト列だけである。
func runVPNProxy(ctx context.Context, called vpnInvocation, environment commandEnvironment) int {
	// 標準出力はデータの通り道である。案内も診断もここへは書かない。
	relay, err := dialVPNRelay(ctx, environment.stateDir, environment.client,
		vpnRelayRequest{profile: called.Name, address: called.Target})
	if err != nil {
		if errors.Is(err, errVPNRelayMissing) {
			fmt.Fprintf(environment.stderr, "sshc: %v\n", err)
			return 1
		}
		return finishVPNFailure(called, err, environment)
	}
	defer func() { _ = relay.Close() }()

	fromRelay := make(chan error, 1)
	go func() {
		_, err := io.Copy(environment.stdout, relay)
		fromRelay <- err
	}()
	if _, err := io.Copy(relay, environment.stdin); err != nil {
		fmt.Fprintf(environment.stderr, "sshc: VPN接続でのデータ送信に失敗しました: %v\n", err)
		return 1
	}
	// 送る側が終わったことを相手へ伝える。伝えないと、相手は入力の終わりを
	// 待ち続ける。
	if half, ok := relay.(interface{ CloseWrite() error }); ok {
		_ = half.CloseWrite()
	}
	if err := <-fromRelay; err != nil {
		fmt.Fprintf(environment.stderr, "sshc: VPN接続でのデータ受信に失敗しました: %v\n", err)
		return 1
	}
	return 0
}
