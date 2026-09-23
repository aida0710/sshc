package main

import (
	"context"
	"errors"
	"fmt"

	"sshc/internal/vpn"
)

// engine が VPN の操作を断った理由を、利用者向けの日本語の文に直す。
//
// engine は決まった語（コード、項目の JSON パス、理由）だけを返す。文言はここと
// 画面の i18n カタログに持ち、Go の engine には持たない。

// vpnRefusalSentences は、理由の語を持たない拒否のコードと、その言い方である。
var vpnRefusalSentences = map[string]string{
	"vpn_profile_unknown":       "そのVPNプロファイルはありません。sshc vpn list で名前を確認してください。",
	"vpn_target_mismatch":       "この接続先は、そのVPNプロファイルの接続先と一致しません。",
	"vpn_docker_missing":        "Dockerを使えません。Dockerが動いているか、このユーザーが操作できるかを確認してください。",
	"vpn_tunnel_device_missing": "この方式に必要なデバイスがこの機械にありません。",
	"vpn_image_build_failed":    "VPNコンテナのイメージを作れませんでした。ネットワークとDockerを確認してください。",
	"vpn_container_foreign":     "同じ名前の別用途のコンテナがあるため操作しません。",
	"connection_unknown":        "その接続にはsshcの設定を持たせられません。",
	"vault_locked":              "Vaultが施錠されています。sshc vault unlock で解錠してからやり直してください。",
	"vault_missing":             "Vaultがまだありません。sshc vault create で作成してからやり直してください。",
}

// vpnFieldReasons は、項目を受け取れなかった理由の語と、その言い方である。
// %d を含むものには上限が入る。
var vpnFieldReasons = map[vpn.Reason]string{
	vpn.ReasonRequired:     "値がありません。",
	vpn.ReasonFormat:       "書き方が違います。",
	vpn.ReasonTooLong:      "長すぎます（%d文字まで）。",
	vpn.ReasonTooMany:      "多すぎます（%d件まで）。",
	vpn.ReasonOutOfRange:   "ポート番号が1から65535の範囲にありません。",
	vpn.ReasonNotIPv4:      "IPv4アドレスではありません。",
	vpn.ReasonUnroutable:   "ループバックなど、経路を作れないアドレスです。",
	vpn.ReasonNameNeedsDNS: "名前で書く場合は、VPNの中のDNSサーバーも指定してください。",
	vpn.ReasonUnsupported:  "この版が知らない値です。",
	vpn.ReasonUnexpected:   "選んだ方式とは別の方式の設定です。",
}

// vpnFailureReasons は、経路を用意できなかった理由の語と、その言い方である。
var vpnFailureReasons = map[vpn.FailureReason]string{
	vpn.FailureUnknown:           "理由を読み取れませんでした。",
	vpn.FailureTimeout:           "待つ上限までに経路ができませんでした。",
	vpn.FailureServerUnresolved:  "VPN装置の名前を引けませんでした。サーバーの書き方を確認してください。",
	vpn.FailureIPsecNegotiation:  "IPsecが成立しませんでした。事前共有鍵と暗号方式を確認してください。",
	vpn.FailurePPPAuthentication: "PPPの認証が通りませんでした。利用者名とパスワードを確認してください。",
	vpn.FailureOpenConnect:       "openconnectがVPN装置へ繋げませんでした。",
	vpn.FailureHandshakeTimeout:  "WireGuardの相手と握手できませんでした。鍵とサーバーを確認してください。",
	vpn.FailureTargetUnresolved:  "VPNの中で接続先の名前を引けませんでした。DNSサーバーと接続先を確認してください。",
	vpn.FailureTunnelLost:        "用意できたあとでトンネルが切れました。",
}

// describeVPNRefusal は、engine の拒否を1文に直す。知らない拒否なら false を返す。
func describeVPNRefusal(problem engineProblem, called vpnInvocation) (string, bool) {
	switch problem.Code {
	case "vpn_profile_exists":
		if called.Action == vpnRename {
			return fmt.Sprintf("%s という名前のVPNプロファイルはすでにあります。別の名前を指定してください。",
				safeTerminalCell(called.Rename)), true
		}
		name := safeTerminalCell(called.Name)
		return fmt.Sprintf("%s という名前のVPNプロファイルはすでにあります。作り直すときは sshc vpn remove %s で"+
			"削除してから追加し、名前を変えるときは sshc vpn rename を使ってください。", name, name), true
	case "vpn_profile_invalid", "vpn_secrets_missing":
		return describeVPNField(problem), true
	case "vpn_session_failed":
		sentence, known := vpnFailureReasons[vpn.FailureReason(problem.Reason)]
		if !known {
			sentence = vpnFailureReasons[vpn.FailureUnknown]
		}
		return "VPNの経路が成立しませんでした。" + sentence, true
	}
	sentence, known := vpnRefusalSentences[problem.Code]
	return sentence, known
}

// describeVPNField は、項目の誤りを「項目: 理由」の1文にする。古い engine が
// 項目を返さなかった場合は、何を確かめればよいかだけを言う。
func describeVPNField(problem engineProblem) string {
	sentence, known := vpnFieldReasons[vpn.Reason(problem.Reason)]
	if problem.Field == "" || !known {
		if problem.Code == "vpn_secrets_missing" {
			return "このプロファイルの秘密が足りません。"
		}
		return "この設定では経路を作れません。接続先とサーバーの書き方を確認してください。"
	}
	if problem.Limit > 0 {
		sentence = fmt.Sprintf(sentence, problem.Limit)
	}
	return safeTerminalCell(problem.Field) + ": " + sentence
}

// finishVPNFailure は、VPN の操作の失敗を書き、終了コードを返す。
//
// JSON では共通の失敗の形を使う。人向けには、VPN の拒否を日本語の文で書き、
// それ以外の失敗（engine が無いなど）は共通の言い方に任せる。
func finishVPNFailure(called vpnInvocation, err error, environment commandEnvironment) int {
	var problem engineProblem
	if called.JSON || !errors.As(err, &problem) || problem.OutcomeUnknown {
		return finishSyncFailure(called.JSON, err, environment.stdout, environment.stderr)
	}
	sentence, known := describeVPNRefusal(problem, called)
	if !known {
		return finishSyncFailure(false, err, environment.stdout, environment.stderr)
	}
	fmt.Fprintln(environment.stderr, "sshc: "+sentence)
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return 1
}
