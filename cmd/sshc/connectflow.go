package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"sshc/internal/handoff"
)

// engineProbe は認証済み engine の状態と接続情報を取得する。
// handoff の存在だけでは engine の稼働を保証できないため、HTTP 応答を検証する。
type engineProbe interface {
	Status(context.Context) (statusAnswer, error)
	Connection(context.Context, string) (connectAnswer, error)
}

// httpProbe は生成時に取得した handoff の engine だけに要求を送る。
type httpProbe struct {
	found  handoff.Handoff
	client *http.Client
}

// Status は `sshc status` と同じ状態の要求を送る。engine に断られたときは、`sshc ssh` の
// 断りの文（refusedRequestError）にする。`sshc status` 自身の文は変えない。
func (probe httpProbe) Status(ctx context.Context) (statusAnswer, error) {
	answer, err := requestStatus(ctx, probe.found, probe.client)
	return answer, explainRefusedRequest(err)
}

func (probe httpProbe) Connection(ctx context.Context, alias string) (connectAnswer, error) {
	return requestConnection(ctx, probe.found, alias, probe.client)
}

// reachUnlockedEngine は稼働中で Vault のロックを解除した engine を返す。
// engine は起動せず、届かない場合とロック中の場合は、理由ごとの復旧手順を返す。
func reachUnlockedEngine(
	ctx context.Context, stateDir string, client *http.Client,
	newProbe func(handoff.Handoff) engineProbe,
) (engineProbe, error) {
	found, status, err := liveEngineStatus(ctx, stateDir, client, newProbe)
	if err != nil {
		err = explainEngineUnreachable(ctx, err)
		if isEngineNotRunning(err) {
			return nil, fmt.Errorf("%w, or use ssh to connect without it", err)
		}
		return nil, err
	}

	probe := newProbe(found)
	if status.Vault && status.Unlocked {
		return probe, nil
	}
	// Vault 未作成とロック中を区別する。
	if !status.Vault {
		return nil, errors.New(vaultMissingAdvice)
	}
	return nil, errors.New(vaultLockedAdvice)
}

// liveEngineStatus は handoff を読み、対象 engine の状態を取得する。
func liveEngineStatus(
	ctx context.Context, stateDir string, client *http.Client,
	newProbe func(handoff.Handoff) engineProbe,
) (handoff.Handoff, statusAnswer, error) {
	found, err := verifiedHandoff(ctx, stateDir, client)
	if err != nil {
		return handoff.Handoff{}, statusAnswer{}, err
	}
	status, err := newProbe(found).Status(ctx)
	if err != nil {
		return handoff.Handoff{}, statusAnswer{}, err
	}
	// 所有者と protocol version は稼働中の engine の応答を使用する。
	found.Owner = status.Owner
	found.ProtocolVersion = status.ProtocolVersion
	return found, status, nil
}
