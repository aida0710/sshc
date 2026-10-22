package remoteengine

import (
	"context"
	"errors"
	"time"
)

// EngineAction は、実行先のエンジンへの、ターミナルを止める操作である。
type EngineAction string

const (
	ActionStop    EngineAction = "stop"
	ActionRestart EngineAction = "restart"
	ActionUpdate  EngineAction = "update"
)

// actionArguments は、操作ごとに接続先で走らせる sshc の引数である。確認は画面で済んで
// いるので、接続先の sshc には尋ねさせない。
var actionArguments = map[EngineAction]string{
	ActionStop:    "stop",
	ActionRestart: "service restart --yes",
	ActionUpdate:  "update --yes",
}

// actionLimit は、操作が終わるまでの上限である。更新は新しい版を取ってきて入れ、
// エンジンを起動し直すまで待つので、回線が遅くても終わる長さにする。
const actionLimit = 10 * time.Minute

// maxActionOutput は、操作の出力のうち画面へ返す末尾の長さである。失敗の理由が読める
// だけあればよい。
const maxActionOutput = 4 << 10

// ErrUnknownAction は、知らない操作を指したことを表す。
var ErrUnknownAction = errors.New("unknown engine action")

// ActionResult は、実行先のエンジンへの操作の結果である。
type ActionResult struct {
	Succeeded bool
	// Output は、接続先の sshc が書いた標準出力と標準エラーの末尾である。
	Output string
}

// ControlEngine は、実行先のエンジンを止める・起動し直す・更新する。どれも接続先の
// sshc コマンドで行い、接続先の CLI と同じ確かめ方をそのまま使う。終わったあとは、
// 受け取っていた資格を捨てる（エンジンが起動し直すと使えなくなる）。
func (m *Manager) ControlEngine(ctx context.Context, alias string, action EngineAction) (ActionResult, error) {
	arguments, known := actionArguments[action]
	if !known {
		return ActionResult{}, ErrUnknownAction
	}
	engine, err := m.engine(alias)
	if err != nil {
		return ActionResult{}, err
	}
	link, err := engine.link.live(ctx)
	if err != nil {
		return ActionResult{}, err
	}
	result, err := link.Run(ctx, remoteSSHC(arguments), actionLimit)
	engine.forgetAny()
	if err != nil {
		return ActionResult{}, err
	}
	if result.ExitCode == notInstalledExit {
		return ActionResult{}, ErrNotInstalled
	}
	output := string(result.Stdout) + string(result.Stderr)
	if len(output) > maxActionOutput {
		output = output[len(output)-maxActionOutput:]
	}
	return ActionResult{Succeeded: result.ExitCode == 0, Output: output}, nil
}
