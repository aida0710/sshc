package main

import (
	"context"
	"errors"
	"io"

	"sshc/internal/handoff"
)

// commandFailureReport は、engine を使うコマンドの失敗 1 つを報告する材料である。
type commandFailureReport struct {
	// cause は元の失敗である。取り消し（context.Canceled）なら終了コードを 130 にする。
	cause   error
	failure commandFailure
	asJSON  bool
	stdout  io.Writer
	stderr  io.Writer
	// writeHuman は、engine と vault の共通の案内が無い種別について、そのコマンド固有の
	// 文を stderr へ書く。--json のときは呼ばない。
	writeHuman func(stderr io.Writer, failure commandFailure)
}

// finishCommandFailure は、失敗の報告の共通の手順である。終了コードを決め、--json なら
// 失敗の封筒を標準出力へ、そうでなければ共通の案内か、コマンド固有の文を標準エラー
// 出力へ書く。
func finishCommandFailure(report commandFailureReport) int {
	exit := exitFailure
	if errors.Is(report.cause, context.Canceled) {
		exit = exitInterrupted
	}
	if report.asJSON {
		_ = writeCommandFailure(report.stdout, report.failure)
		return exit
	}
	if !writeEngineFailureAdvice(report.stderr, report.failure) {
		report.writeHuman(report.stderr, report.failure)
	}
	return exit
}

// classifyCommandFailure は、engine を使うコマンドの失敗を、--json の封筒と人向けの文で
// 使う種別に分ける。コマンド固有の失敗は各コマンドが先に分け、残りをここに任せる。
func classifyCommandFailure(err error) commandFailure {
	var problem engineProblem
	if errors.As(err, &problem) && problem.OutcomeUnknown {
		return commandFailure{Kind: "outcome_unknown", Retryable: false}
	}
	switch {
	case errors.Is(err, context.Canceled):
		return commandFailure{Kind: "canceled", Retryable: true}
	case errors.As(err, new(engineNotRunning)):
		return commandFailure{Kind: "engine_not_running", Retryable: true}
	case errors.Is(err, handoff.ErrSchemaVersion), errors.Is(err, handoff.ErrProtocolVersion):
		return commandFailure{Kind: "engine_incompatible", Retryable: false}
	case errors.Is(err, errEngineIdentityMismatch):
		return commandFailure{Kind: "engine_mismatch", Retryable: false}
	case errors.Is(err, errEngineVaultMissing):
		return commandFailure{Kind: "vault_missing", Retryable: false}
	case errors.Is(err, errEngineVaultLocked):
		return commandFailure{Kind: "vault_locked", Retryable: false}
	case errors.Is(err, errInteractivePromptRequired):
		return commandFailure{Kind: "interactive_terminal_required", Retryable: false}
	case errors.Is(err, errEngineInvalidResponse), errors.Is(err, errEngineResponseTooLarge):
		return commandFailure{Kind: "invalid_engine_response", Retryable: false}
	}
	if errors.As(err, &problem) {
		return commandFailure{Kind: problem.Code, Retryable: problem.Retryable}
	}
	return commandFailure{Kind: "engine_unavailable", Retryable: true}
}
