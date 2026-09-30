// Package vpnrefusal は、VPN の操作や接続を断った理由を、決まった語と、利用者
// 向けの1文に直す。
//
// HTTP API の problem、engine の中継の答え、CLI の表示、Terminal の接続ログが同じ
// 語を使う。文は2つの言語で持つ。sshcエンジンの Terminal の接続ログは日本語の文
// （Sentence）を、CLI は英語の文（EnglishSentence）を使う。画面は語を i18n カタログで
// 訳し、この文は使わない。英語の文は、画面の英語（web/src/i18n/messages/en.ts の
// vpn.*）と同じ文面にする。
package vpnrefusal

import (
	"errors"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/vpn"
)

// Refusal は、断った理由の語である。
type Refusal struct {
	// Code は、断った種類である（`vpn_route_failed` など）。
	Code string
	// Field は、項目の誤りのときの項目の JSON パスである。
	Field string
	// Reason は、項目の誤りの理由、または経路や接続先へ届かなかった理由の語である。
	Reason string
	// Limit は、長さや件数の上限である。無ければ 0。
	Limit int
	// Line と Directive は、設定ファイル（OpenVPN の .ovpn）の中の誤りのときの、1から
	// 数えた行番号と、断った指示の名前である。無ければ 0 と空。
	Line      int
	Directive string
}

// 断った種類の語である。
const (
	CodeProfileUnknown      = "vpn_profile_unknown"
	CodeProfileExists       = "vpn_profile_exists"
	CodeConnectionUnknown   = "connection_unknown"
	CodeProfileInvalid      = "vpn_profile_invalid"
	CodeSecretsMissing      = "vpn_secrets_missing"
	CodeVaultLocked         = "vault_locked"
	CodeVaultMissing        = "vault_missing"
	CodeDockerMissing       = "vpn_docker_missing"
	CodeDockerNotRunning    = "vpn_docker_not_running"
	CodeTunnelDeviceMissing = "vpn_tunnel_device_missing"
	CodeImageBuildFailed    = "vpn_image_build_failed"
	CodeContainerForeign    = "vpn_container_foreign"
	CodeSocketPathTooLong   = "vpn_socket_path_too_long"
	// CodeChangedConcurrently は、読んでから書くまでのあいだに、ほかの操作が同じ
	// 設定を変えたことを表す。何も書いていない。
	CodeChangedConcurrently = "vpn_changed_concurrently"
	// CodeRouteDisconnected は、利用者が経路を切断したため、経路を起動しなかった（起動の
	// 途中なら中止した）ことを表す。
	CodeRouteDisconnected = "vpn_route_disconnected"
	// CodeRouteStopped は、起動の途中で経路が停止されたため、起動を中止したことを表す。
	// sshcエンジンの終了と、プロファイルの削除・名前の変更が経路を停止する。
	CodeRouteStopped       = "vpn_route_stopped"
	CodeDestinationInvalid = vpn.CodeDestinationInvalid
	CodeTargetFailed       = vpn.CodeTargetFailed
	CodeRouteFailed        = vpn.CodeRouteFailed
)

// kinds は、失敗の種類と語の対応である。上から順に照らし、最初に当たったものを使う。
var kinds = []struct {
	kind error
	code string
}{
	{application.ErrUnknownVPNProfile, CodeProfileUnknown},
	{application.ErrVPNProfileExists, CodeProfileExists},
	{application.ErrUnknownConnection, CodeConnectionUnknown},
	{application.ErrMetadataVPN, CodeProfileInvalid},
	{vpn.ErrProfileName, CodeProfileInvalid},
	{vpn.ErrBackend, CodeProfileInvalid},
	{vpn.ErrSettings, CodeProfileInvalid},
	{vpn.ErrSecrets, CodeSecretsMissing},
	{secret.ErrUnknownCredential, CodeSecretsMissing},
	{secret.ErrLocked, CodeVaultLocked},
	{secret.ErrNoVault, CodeVaultMissing},
	{vpn.ErrRouteDisconnected, CodeRouteDisconnected},
	{vpn.ErrRouteStopped, CodeRouteStopped},
	{vpn.ErrDockerMissing, CodeDockerMissing},
	{vpn.ErrDockerNotRunning, CodeDockerNotRunning},
	{vpn.ErrTunnelDevice, CodeTunnelDeviceMissing},
	{vpn.ErrImageBuild, CodeImageBuildFailed},
	{vpn.ErrContainerForeign, CodeContainerForeign},
	{vpn.ErrSocketPath, CodeSocketPathTooLong},
	{vpn.ErrDestination, CodeDestinationInvalid},
	{vpn.ErrTargetFailed, CodeTargetFailed},
	{vpn.ErrRouteFailed, CodeRouteFailed},
}

// Known は、code がこの package の語かを返す。engine が返した problem のうち、
// VPN の拒否だけをここの文で見せるのに使う。
func Known(code string) bool {
	if code == CodeChangedConcurrently {
		return true
	}
	for _, known := range kinds {
		if known.code == code {
			return true
		}
	}
	return false
}

// Of は、err を断った理由の語へ直す。VPN の失敗でなければ false を返す。
func Of(err error) (Refusal, bool) {
	if application.IsExternalChange(err) {
		return Refusal{Code: CodeChangedConcurrently}, true
	}
	for _, known := range kinds {
		if !errors.Is(err, known.kind) {
			continue
		}
		refusal := Refusal{Code: known.code}
		var field *vpn.FieldError
		if errors.As(err, &field) {
			refusal.Field, refusal.Reason, refusal.Limit = field.Field, string(field.Reason), field.Limit
		}
		var line *vpn.ConfigLineError
		if errors.As(err, &line) {
			refusal.Line, refusal.Directive = line.Line, line.Directive
		}
		var destination *vpn.DestinationError
		if errors.As(err, &destination) {
			refusal.Reason = string(destination.Reason)
		}
		var target *vpn.TargetFailure
		if errors.As(err, &target) {
			refusal.Reason = string(target.Reason)
		}
		var route *vpn.RouteFailure
		if errors.As(err, &route) {
			refusal.Reason = string(route.Reason)
		}
		return refusal, true
	}
	return Refusal{}, false
}

// HasLogs は、その拒否の原因が sshc vpn logs（画面の「ログ」）に残っているかを返す。
// 案内の文に「ログを確認してください」を添えるかどうかに使う。
func HasLogs(code string) bool {
	switch code {
	case CodeRouteFailed, CodeImageBuildFailed, CodeTargetFailed, CodeDockerNotRunning:
		return true
	}
	return false
}

// RequiresAction は、利用者が何かを直さない限り、繰り返しても同じ理由で断られるかを
// 返す。Terminal はこれが真なら再接続を繰り返さない。
func (refusal Refusal) RequiresAction() bool {
	switch refusal.Code {
	case CodeTargetFailed:
		// 接続先が応えない、VPN が切れた、は時間が経てば直りうる。
		switch vpn.FailureReason(refusal.Reason) {
		case vpn.FailureTargetUnreachable, vpn.FailureTunnelLost, vpn.FailureTimeout:
			return false
		}
		return true
	case CodeRouteFailed:
		switch vpn.FailureReason(refusal.Reason) {
		case vpn.FailureTunnelLost, vpn.FailureTimeout, vpn.FailureUnknown:
			return false
		}
		return true
	case CodeChangedConcurrently:
		return false
	}
	return true
}

// sentences は、理由の語を持たない拒否の言い方である。
var sentences = map[string]string{
	CodeProfileUnknown: "指定したVPNプロファイルが見つかりません。sshc vpn で名前を確認してください。",
	CodeProfileExists:  "同じ名前のVPNプロファイルがすでにあります。",
	CodeConnectionUnknown: "この接続にはVPNプロファイルを設定できません。Includeしたファイルやワイルドカードの" +
		"Hostだけで定義された接続は、sshcで編集できません。",
	CodeVaultLocked:  "Vaultがロックされています。sshc vault unlock でロックを解除してからやり直してください。",
	CodeVaultMissing: "Vaultがまだありません。sshc vault create で作成してからやり直してください。",
	CodeDockerMissing: "Dockerが見つかりません。VPN経路にはDockerが必要です。" +
		"Docker Desktopなどをインストールしてください。",
	CodeDockerNotRunning: "Dockerが起動していません。Docker Desktopなどを起動してからやり直してください。" +
		"起動している場合は、現在のユーザーにDockerを操作する権限があるかを確認してください。",
	CodeTunnelDeviceMissing: "この方式に必要なトンネル用のデバイス（/dev/net/tun、/dev/ppp）をDockerで使用できません。",
	CodeImageBuildFailed:    "VPNのコンテナイメージの作成に失敗しました。ネットワークとDockerを確認してください。",
	CodeContainerForeign:    "sshc以外が作成した同じ名前のコンテナがあるため、操作を中止しました。",
	// ソケットの場所は、プロファイル名から作る固定長の識別子で決まり、名前の長さに
	// 左右されない（vpn.routeDirectory）。長さを変えられるのは ~/.ssh の実際の場所だけである。
	CodeSocketPathTooLong: "sshcのデータの保存場所（~/.ssh/sshc）のパスが長すぎるため、VPN経路の中継を作成できません。" +
		"~/.sshをパスの短いフォルダへ移し、元の場所にシンボリックリンクを置いてください。",
	CodeChangedConcurrently: "ほかの操作と同時に変更されたため、保存しませんでした。もう一度やり直してください。",
	CodeRouteDisconnected:   "VPN経路が切断されたため、起動を中止しました。",
	CodeRouteStopped:        "VPN経路が停止されたため、起動を中止しました。",
}

// fieldReasons は、項目を受け取れなかった理由の言い方である。%d を含むものには上限が入る。
var fieldReasons = map[vpn.Reason]string{
	vpn.ReasonRequired:    "入力してください。",
	vpn.ReasonFormat:      "形式が正しくありません。",
	vpn.ReasonTooLong:     "長すぎます（%d文字まで）。",
	vpn.ReasonTooMany:     "多すぎます（%d件まで）。",
	vpn.ReasonOutOfRange:  "ポート番号は1〜65535で指定してください。",
	vpn.ReasonNotIPv4:     "IPv4アドレスを指定してください。",
	vpn.ReasonUnroutable:  "ループバックアドレスなど、使用できないアドレスです。",
	vpn.ReasonUnsupported: "使用できない値です。",
	vpn.ReasonUnexpected:  "選択した方式では使用しない設定項目です。",
}

// destinationReasons は、接続先を VPN 経由で使えない理由の言い方である。
var destinationReasons = map[vpn.Reason]string{
	vpn.ReasonFormat:       "接続先のHostNameの形式が正しくありません。",
	vpn.ReasonOutOfRange:   "接続先のPortは1〜65535で指定してください。",
	vpn.ReasonNotIPv4:      "VPN経由の接続先には、IPv4アドレスかホスト名を指定してください。IPv6アドレスには対応していません。",
	vpn.ReasonUnroutable:   "ループバックアドレスなど、VPN経由では使用できないアドレスです。",
	vpn.ReasonNameNeedsDNS: "接続先をホスト名で指定する場合は、VPNプロファイルにVPN内のDNSサーバーを指定してください。",
}

// routeReasons は、経路を用意できなかった理由の言い方である。
var routeReasons = map[vpn.FailureReason]string{
	vpn.FailureUnknown:           "原因を特定できませんでした。",
	vpn.FailureTimeout:           "接続がタイムアウトしました。",
	vpn.FailureServerUnresolved:  "VPNサーバーの名前解決に失敗しました。サーバーの指定を確認してください。",
	vpn.FailureIPsecNegotiation:  "IPsecのネゴシエーションに失敗しました。事前共有鍵と暗号スイートを確認してください。",
	vpn.FailurePPPAuthentication: "PPPの認証に失敗しました。ユーザー名とパスワードを確認してください。",
	vpn.FailureOpenConnect:       "ユーザー名、パスワード、二要素認証、証明書を確認してください。",
	vpn.FailureHandshakeTimeout:  "ハンドシェイクに失敗しました。鍵とサーバーの指定を確認してください。",
	vpn.FailureTunnelLost:        "接続が確立した直後にVPNが切断されました。",
}

// targetReasons は、経路はあるが接続先へ繋げなかった理由の言い方である。
var targetReasons = map[vpn.FailureReason]string{
	vpn.FailureTargetUnresolved:  "VPN内のDNSサーバーで接続先の名前解決に失敗しました。DNSサーバーとHostNameを確認してください。",
	vpn.FailureTargetNeedsDNS:    destinationReasons[vpn.ReasonNameNeedsDNS],
	vpn.FailureTargetIsServer:    "接続先がVPNサーバーと同じアドレスです。VPNサーバーそのものにはVPN経由で接続できません。",
	vpn.FailureTargetUnreachable: "接続先が応答しないか、接続を拒否しました。HostNameとPortを確認してください。",
	vpn.FailureTunnelLost:        "VPNが切断されました。もう一度接続してください。",
	vpn.FailureTimeout:           "接続がタイムアウトしました。",
}

// japanese は、sshcエンジンの Terminal の接続ログとコンテナのログが使う日本語の言い方である。
var japanese = phrasebook{
	codes: sentences, fields: fieldReasons, destinations: destinationReasons,
	routes: routeReasons, targets: targetReasons, directives: directiveReasons,
	frames: phraseFrames{
		routeFailed:     "VPNの接続に失敗しました。%s",
		targetFailed:    "VPN経由で接続先に接続できませんでした。%s",
		operationFailed: "VPNの操作に失敗しました。",
		secretsMissing:  "このVPNプロファイルのシークレット（秘密鍵やパスワード）が保存されていません。",
		profileInvalid:  "VPNプロファイルに使用できない値があります。VPNサーバーとDNSサーバーの指定を確認してください。",
		lineDirective:   "%d行目の%s",
		line:            "%d行目：%s",
		peerMissing:     "%d行目の[Peer]に「%s」がありません。",
		fileMissing:     "設定ファイルに「%s」がありません。",
	},
}

// Sentence は、理由を sshcエンジンの接続ログに書く日本語の1文にする。
func Sentence(refusal Refusal) string { return japanese.sentence(refusal) }

// EnglishSentence は、理由を CLI に書く英語の1文にする。
func EnglishSentence(refusal Refusal) string { return english.sentence(refusal) }
