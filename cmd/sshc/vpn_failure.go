package main

import (
	"errors"
	"fmt"
	"io"
)

// sshc vpn の失敗を書き、終了コードを返す。--json では共通の失敗の形を使い、人向けには
// VPN の言い方で書く。案内する先は sshc sync ではなく sshc vpn である。

// finishVPNFailure は、VPN の操作の失敗を書き、終了コードを返す。engine と vault に
// まつわる失敗は、ほかのコマンドと同じ案内（writeEngineFailureAdvice）で書く。
func finishVPNFailure(called vpnInvocation, err error, environment commandEnvironment) int {
	return finishCommandFailure(commandFailureReport{
		cause: err, failure: classifyCommandFailure(err), asJSON: called.JSON,
		stdout: environment.stdout, stderr: environment.stderr,
		writeHuman: func(stderr io.Writer, _ commandFailure) {
			fmt.Fprintln(stderr, humanVPNFailure(called, err))
		},
	})
}

// humanVPNFailure は、VPN の操作の失敗を人向けの1文にする。CLI が見つけた入力の誤りと
// engine の拒否は、その理由を書く。それ以外は、失敗の種類ごとの言い方で書く。
func humanVPNFailure(called vpnInvocation, err error) string {
	var input *vpnInputError
	if errors.As(err, &input) {
		return "sshc: " + input.sentence
	}
	var problem engineProblem
	if errors.As(err, &problem) && !problem.OutcomeUnknown {
		if sentence, known := describeVPNRefusal(problem, called); known {
			return "sshc: " + sentence
		}
	}
	failure := classifyCommandFailure(err)
	switch failure.Kind {
	case "canceled":
		return "sshc: the VPN operation was canceled"
	case "outcome_unknown":
		// 経路の起動や削除を頼んだあとで応答が途切れると、engine の側で終わったかは
		// 分からない。やり直す前に、いまの状態を見てもらう。
		return "sshc: the connection to the engine ended before the VPN operation finished, " +
			"so its outcome is unknown; run sshc vpn to see the current state"
	case "interactive_terminal_required":
		return "sshc: sshc vpn add and sshc vpn edit ask for the settings; run them in an interactive terminal"
	case "invalid_engine_response", "response_too_large", "http_error":
		return "sshc: " + engineInvalidResponseAdvice
	case "transport_error", "engine_unavailable":
		return "sshc: the engine is unavailable; check that it is running, then try again"
	}
	return fmt.Sprintf("sshc: the VPN operation failed (%s); run sshc vpn to see the current state", failure.Kind)
}
