package main

import (
	"context"
	"errors"
	"fmt"

	"sshc/internal/vpnrefusal"
)

// engine が VPN の操作を断った理由を、利用者向けの日本語の文に直す。
//
// engine は決まった語（コード、項目の JSON パス、理由）だけを返す。文は
// internal/vpnrefusal にあり、engine の Terminal の接続ログと同じ言い方をする。
// ここは、CLI の引数を使って言い換えるものだけを持つ。

// describeVPNRefusal は、engine の拒否を1文に直す。VPN の拒否でなければ false を返す。
func describeVPNRefusal(problem engineProblem, called vpnInvocation) (string, bool) {
	if problem.Code == vpnrefusal.CodeProfileExists {
		if called.Action == vpnRename {
			return fmt.Sprintf("%s という名前のVPNプロファイルはすでにあります。別の名前を指定してください。",
				safeTerminalCell(called.Rename)), true
		}
		name := safeTerminalCell(called.Name)
		return fmt.Sprintf("%s という名前のVPNプロファイルはすでにあります。設定を変更する場合は sshc vpn edit %s を、"+
			"名前を変更する場合は sshc vpn rename を使用してください。", name, name), true
	}
	if !vpnrefusal.Known(problem.Code) {
		return "", false
	}
	return vpnrefusal.Sentence(vpnrefusal.Refusal{
		Code: problem.Code, Field: safeTerminalCell(problem.Field), Reason: problem.Reason, Limit: problem.Limit,
	}), true
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
	if problem.Code == vpnrefusal.CodeSessionFailed {
		sentence += fmt.Sprintf("詳しくは sshc vpn logs %s でログを確認してください。", safeTerminalCell(profile))
	}
	return &vpnRouteError{sentence: sentence, err: err}
}
