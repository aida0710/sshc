package main

import (
	"errors"
	"fmt"

	"sshc/internal/vpnrefusal"
)

// engine が VPN の操作を断った理由を、利用者向けの英語の文に直す。
//
// engine は決まった語（コード、項目の JSON パス、理由）だけを返す。文は
// internal/vpnrefusal にあり、画面の英語と同じ文面を使う。ここは、CLI の引数を
// 使って言い換えるものだけを持つ。

// describeVPNRefusal は、engine の拒否を1文に直す。VPN の拒否でなければ false を返す。
func describeVPNRefusal(problem engineProblem, called vpnInvocation) (string, bool) {
	if problem.Code == vpnrefusal.CodeProfileExists {
		if called.Action == vpnRename {
			return fmt.Sprintf("A VPN profile named \"%s\" already exists. Choose another name.",
				safeTerminalCell(called.Rename)), true
		}
		return fmt.Sprintf("A VPN profile named \"%s\" already exists. To change its settings, use %s; "+
			"to change its name, use sshc vpn rename.",
			safeTerminalCell(called.Name), safeTerminalCell(vpnrefusal.ProfileCommand("edit", called.Name))), true
	}
	if !vpnrefusal.Known(problem.Code) {
		return "", false
	}
	return vpnrefusal.EnglishSentence(vpnrefusal.Refusal{
		Code: problem.Code, Field: safeTerminalCell(problem.Field), Reason: problem.Reason, Limit: problem.Limit,
		Line: problem.Line, Directive: safeTerminalCell(problem.Directive),
	}), true
}

// vpnRouteError は、`sshc ssh <alias>` が VPN 経路を用意できなかったことを、
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
	if vpnrefusal.HasLogs(problem.Code) {
		sentence += " " + vpnLogsHint(profile)
	}
	return &vpnRouteError{sentence: sentence, err: err}
}

// vpnLogsHint は、経路を用意できなかった理由をログで読む方法を案内する文である。
func vpnLogsHint(profile string) string {
	return fmt.Sprintf("See the logs with %s for details.", safeTerminalCell(vpnrefusal.ProfileCommand("logs", profile)))
}
