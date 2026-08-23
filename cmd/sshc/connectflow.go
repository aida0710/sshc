package main

import (
	"context"
	"errors"
	"io"
	"net/http"

	"sshc/internal/handoff"
)

// engineProbe は認証済み engine の状態と接続情報を取得する。
// handoff の存在だけでは engine の稼働を保証できないため、HTTP 応答を検証する。
type engineProbe interface {
	Status(context.Context) (statusAnswer, error)
	Connection(context.Context, string) (connectAnswer, error)
}

// errInterrupted は Ctrl-C による中断を表し、終了コード 130 に変換される。
var errInterrupted = errors.New("interrupted")

// errEngineChanged は待機中に engine の識別情報が変わったことを表す。
var errEngineChanged = errors.New("the running sshc changed while waiting; run the command again")

// httpProbe は生成時に取得した handoff の engine だけに要求を送る。
type httpProbe struct {
	found  handoff.Handoff
	client *http.Client
}

func (probe httpProbe) Status(ctx context.Context) (statusAnswer, error) {
	return requestStatus(ctx, probe.found, probe.client)
}

func (probe httpProbe) Connection(ctx context.Context, alias string) (connectAnswer, error) {
	return requestConnection(ctx, probe.found, alias, probe.client)
}

// reachUnlockedEngine は稼働中で解錠済みの engine を返す。
// engine は起動せず、停止中または施錠中の場合は復旧手順を返す。
func reachUnlockedEngine(
	ctx context.Context, stateDir string, client *http.Client,
	newProbe func(handoff.Handoff) engineProbe, stderr io.Writer,
) (engineProbe, error) {
	found, status, err := liveEngineStatus(ctx, stateDir, client, newProbe)
	if err != nil {
		return nil, errors.New("sshc is not running; run sshc engine in another terminal, or use ssh to connect without it")
	}

	probe := newProbe(found)
	if status.Vault && status.Unlocked {
		return probe, nil
	}
	// Vault 未作成と解錠済みを区別する。
	if !status.Vault {
		return nil, errors.New("this installation has no vault; run sshc vault create")
	}
	return nil, errors.New("the sshc vault is locked; run sshc vault unlock")
}

// liveEngineStatus は handoff を読み、対象 engine の状態を取得する。
func liveEngineStatus(
	ctx context.Context, stateDir string, client *http.Client,
	newProbe func(handoff.Handoff) engineProbe,
) (handoff.Handoff, statusAnswer, error) {
	found, err := readHandoff(stateDir)
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
