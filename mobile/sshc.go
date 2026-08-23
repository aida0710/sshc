// Package mobile は、gomobile で Android アプリから engine を操作するための境界。
// 公開 API は gomobile が扱える文字列、数値、error に限定する。
package mobile

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"

	"sshc/internal/app"
	"sshc/internal/enginelock"
)

// version は、この AAR のバージョンである。
//
// cmd/sshc の version とは別に持つ。AAR は cmd/sshc とは別の成果物であり、
// 同じ変数を共有すると、どちらのビルドが値を入れたのか分からなくなる。
var version = "dev"

func Version() string { return version }

var (
	// ErrAlreadyStarted は、このプロセスの engine が既に受け付けていることを言う。
	ErrAlreadyStarted = errors.New("an engine is already running in this process")
	ErrNotStarted     = errors.New("no engine is running in this process")

	errListenFailed       = errors.New("the engine could not take a loopback port")
	errEngineStoppedEarly = errors.New("the engine stopped before it announced an entrance")
)

// running は、このプロセスで唯一の engine の状態を保持する。
var running struct {
	sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	release func() error
	// lastKind は、gomobile の error 変換で型情報が失われないよう、直前の
	// Start の失敗理由を数値で保持する。
	lastKind int
}

// Start は engine を起動し、入口の URL を返す。
//
// 返るのは listener が bind され Announce が呼ばれた後である。呼び出し側は
// この URL を即座に WebView へ渡すので、早く返せば空のページが出る。
func Start(home, cache string) (string, error) {
	running.Lock()
	defer running.Unlock()
	if running.cancel != nil {
		return "", fail(KindAlreadyStarted, ErrAlreadyStarted)
	}

	// app.Run の前にロックを取得し、同じ状態ディレクトリの同時利用を防ぐ。
	release, err := enginelock.Acquire(filepath.Join(app.HandoffDir(home), "engine.lock"))
	if err != nil {
		return "", fail(KindAlreadyStarted, err)
	}

	// gomobile bind は標準 log の出力先を logcat へ差し替える。slog をそこへ
	// 流し込めば、cgo を 1 行も書かずに logcat へ出る。
	logger := slog.New(slog.NewTextHandler(log.Writer(), &slog.HandlerOptions{Level: slog.LevelInfo}))

	entrance := make(chan string, 1)
	dependencies, err := newDependencies(runtime.GOOS, home, cache, logger, func(readiness app.Readiness) error {
		entrance <- readiness.Entrance
		return nil
	})
	if err != nil {
		return "", fail(KindUnknown, errors.Join(err, release()))
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	failed := make(chan error, 1)
	go func() {
		defer close(done)
		if runErr := app.Run(ctx, dependencies, version); runErr != nil && !errors.Is(runErr, context.Canceled) {
			failed <- runErr
		}
	}()

	select {
	case url := <-entrance:
		running.cancel, running.done, running.release = cancel, done, release
		running.lastKind = KindNone
		return url, nil
	case err := <-failed:
		cancel()
		<-done
		return "", fail(KindListenFailed, errors.Join(errListenFailed, err, release()))
	case <-done:
		cancel()
		return "", fail(KindStoppedEarly, errors.Join(errEngineStoppedEarly, release()))
	}
}

// Stop は app.Run の終了を待ってから engine lock を解放する。
func Stop() error {
	running.Lock()
	defer running.Unlock()
	if running.cancel == nil {
		return ErrNotStarted
	}
	running.cancel()
	<-running.done
	err := running.release()
	running.cancel, running.done, running.release = nil, nil, nil
	return err
}

// Start の失敗理由。gomobile が同じ定数を Java 側へ公開する。
const (
	KindNone = iota
	KindUnknown
	KindAlreadyStarted
	KindListenFailed
	KindStoppedEarly
)

// fail は running の mutex を保持した状態で失敗理由を記録する。
func fail(kind int, err error) error {
	running.lastKind = kind
	return err
}

// LastStartFailureKind は直前の Start の失敗理由を返す。エラー文に含まれ得る
// bootstrap fragment を Java や logcat へ渡さない。
func LastStartFailureKind() int {
	running.Lock()
	defer running.Unlock()
	return running.lastKind
}
