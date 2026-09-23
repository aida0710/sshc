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
	"vpn_profile_unknown":       "指定したVPNプロファイルが見つかりません。sshc vpn list で名前を確認してください。",
	"vpn_target_mismatch":       "この接続先は、そのVPNプロファイルの接続先と一致しません。",
	"vpn_docker_missing":        "Dockerを使用できません。Dockerが起動しているか、このユーザーに操作する権限があるかを確認してください。",
	"vpn_tunnel_device_missing": "この方式に必要なデバイスがこのマシンにありません。",
	"vpn_image_build_failed":    "VPNコンテナのイメージを作成できませんでした。ネットワークとDockerを確認してください。",
	"vpn_container_foreign":     "同じ名前の別のコンテナがあるため、操作を中止しました。",
	"connection_unknown":        "この接続にはsshcの設定を保存できません。",
	"vault_locked":              "Vaultがロックされています。sshc vault unlock でロックを解除してからやり直してください。",
	"vault_missing":             "Vaultがまだありません。sshc vault create で作成してからやり直してください。",
}

// vpnFieldReasons は、項目を受け取れなかった理由の語と、その言い方である。
// %d を含むものには上限が入る。
var vpnFieldReasons = map[vpn.Reason]string{
	vpn.ReasonRequired:     "入力してください。",
	vpn.ReasonFormat:       "形式が正しくありません。",
	vpn.ReasonTooLong:      "長すぎます（%d文字まで）。",
	vpn.ReasonTooMany:      "多すぎます（%d件まで）。",
	vpn.ReasonOutOfRange:   "ポート番号は1〜65535で指定してください。",
	vpn.ReasonNotIPv4:      "IPv4アドレスを指定してください。",
	vpn.ReasonUnroutable:   "ループバックアドレスなど、使用できないアドレスです。",
	vpn.ReasonNameNeedsDNS: "ホスト名で指定する場合は、VPN内のDNSサーバーも指定してください。",
	vpn.ReasonUnsupported:  "このバージョンでは使用できない値です。",
	vpn.ReasonUnexpected:   "選択した方式とは別の方式の設定です。",
}

// vpnFailureReasons は、経路を用意できなかった理由の語と、その言い方である。
var vpnFailureReasons = map[vpn.FailureReason]string{
	vpn.FailureUnknown:           "原因を特定できませんでした。",
	vpn.FailureTimeout:           "接続がタイムアウトしました。",
	vpn.FailureServerUnresolved:  "VPNサーバーの名前解決に失敗しました。サーバーの指定を確認してください。",
	vpn.FailureIPsecNegotiation:  "IPsecのネゴシエーションに失敗しました。事前共有鍵と暗号スイートを確認してください。",
	vpn.FailurePPPAuthentication: "PPPの認証に失敗しました。ユーザー名とパスワードを確認してください。",
	vpn.FailureOpenConnect:       "ユーザー名、パスワード、二要素認証、証明書を確認してください。",
	vpn.FailureHandshakeTimeout:  "ハンドシェイクに失敗しました。鍵とサーバーの指定を確認してください。",
	vpn.FailureTargetUnresolved:  "VPN内で接続先の名前解決に失敗しました。DNSサーバーと接続先を確認してください。",
	vpn.FailureTunnelLost:        "接続が確立した直後にVPNが切断されました。",
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
		return "VPNの接続に失敗しました。" + sentence, true
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
			return "このプロファイルの秘密が保存されていません。"
		}
		return "この設定ではVPN経路を作成できません。接続先とサーバーの指定を確認してください。"
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
	var input *vpnInputError
	if !called.JSON && errors.As(err, &input) {
		fmt.Fprintln(environment.stderr, "sshc: "+input.sentence)
		return 1
	}
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

// vpnRouteError は、`sshc <接続先>` が VPN 経路を用意できなかったことを、
// `sshc vpn up` と同じ言い方で表す。元の失敗は Unwrap で辿れる。
type vpnRouteError struct {
	sentence string
	err      error
}

func (failure *vpnRouteError) Error() string { return failure.sentence }

func (failure *vpnRouteError) Unwrap() error { return failure.err }

// describedVPNRouteError は、engine の拒否を人向けの文にし、経路を用意できなかった
// ときはログの読み方を添える。知らない失敗はそのまま返す。
func describedVPNRouteError(profile string, err error) error {
	var problem engineProblem
	if !errors.As(err, &problem) || problem.OutcomeUnknown {
		return err
	}
	sentence, known := describeVPNRefusal(problem, vpnInvocation{Action: vpnUp, Name: profile})
	if !known {
		return err
	}
	if problem.Code == "vpn_session_failed" {
		sentence += fmt.Sprintf("詳しくは sshc vpn logs %s でログを確認してください。", safeTerminalCell(profile))
	}
	return &vpnRouteError{sentence: sentence, err: err}
}
