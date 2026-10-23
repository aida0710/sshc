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

// actionLimit は、操作が終わるまでの上限である。更新は新しいバージョンを取ってきて入れ、
// エンジンを起動し直すまで待つので、回線が遅くても終わる長さにする。
const actionLimit = 10 * time.Minute

// ErrUnknownAction は、知らない操作を指したことを表す。
var ErrUnknownAction = errors.New("unknown engine action")

// StartAction は、実行先のエンジンを止める・起動し直す・更新する操作を始め、その世代を返す。
// どれも接続先の sshc コマンドで行い、接続先の CLI と同じ確かめ方をそのまま使う。
//
// 接続先への SSH の接続までは、ここで済ませる。実行先に足していない、接続できない、と
// いった失敗は、操作を始める前に返す。操作そのものは、要求ではなく Manager の寿命で走る。
// 画面を離れても、更新を途中で止めないためである。経過と出力は ActionLog で読む。
func (m *Manager) StartAction(ctx context.Context, alias string, action EngineAction) (uint64, error) {
	arguments, known := actionArguments[action]
	if !known {
		return 0, ErrUnknownAction
	}
	engine, err := m.engine(alias)
	if err != nil {
		return 0, err
	}
	link, err := engine.link.live(ctx)
	if err != nil {
		return 0, err
	}
	log, err := m.actionLog(alias)
	if err != nil {
		return 0, err
	}
	generation, err := log.begin(action)
	if err != nil {
		return 0, err
	}
	err = m.goAction(func(ctx context.Context) {
		result, err := link.Run(ctx, RunRequest{Command: remoteSSHC(arguments), Limit: actionLimit, Observe: log})
		// エンジンが起動し直すと、受け取っていた資格は使えなくなる。
		engine.forgetAny()
		log.finish(actionOutcome(result, err))
	})
	if err != nil {
		log.finish(false, err)
		return 0, err
	}
	return generation, nil
}

// goAction は、run を Manager の寿命の中で走らせる。run の ctx は、Manager が閉じると
// 取り消される。閉じていれば走らせず ErrClosed を返す。
func (m *Manager) goAction(run func(ctx context.Context)) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.closed {
		return ErrClosed
	}
	// Close は closed にしてから待つので、mutex の中で数えれば、待ち始めたあとに増えない。
	m.actionsRunning.Add(1)
	go func() {
		defer m.actionsRunning.Done()
		run(m.actionContext)
	}()
	return nil
}

// actionOutcome は、接続先の sshc の結果を、成功したかと、終了コード以外で終わった理由にする。
func actionOutcome(result RunResult, err error) (bool, error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return false, ErrActionTimedOut
	case errors.Is(err, context.Canceled):
		// Manager が閉じた。このマシンのエンジンが止まりかけている。
		return false, ErrClosed
	case err != nil:
		return false, err
	case result.ExitCode == notInstalledExit:
		return false, ErrNotInstalled
	}
	return result.ExitCode == 0, nil
}

// ActionLog は、alias で最後に始めた操作の経過と、その出力の cursor から limit バイトまでを返す。
func (m *Manager) ActionLog(alias string, cursor uint64, limit int) (ActionLogRead, error) {
	if _, err := m.engine(alias); err != nil {
		return ActionLogRead{}, err
	}
	log, err := m.actionLog(alias)
	if err != nil {
		return ActionLogRead{}, err
	}
	return log.read(cursor, limit)
}

// actionLog は、alias の操作の記録を返す。無ければ作る。
func (m *Manager) actionLog(alias string) (*actionLog, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	log := m.actions[alias]
	if log == nil {
		log = &actionLog{}
		m.actions[alias] = log
	}
	return log, nil
}
