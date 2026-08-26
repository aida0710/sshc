// Package mobile は、gomobile で Android アプリから engine を操作するための境界。
// 公開 API は gomobile が扱える文字列、数値、error に限定する。
package mobile

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"sshc/internal/app"
	"sshc/internal/storage"
)

// version は、この AAR のバージョンである。
//
// cmd/sshc の version とは別に持つ。AAR は cmd/sshc とは別の成果物であり、
// 同じ変数を共有すると、どちらのビルドが値を入れたのか分からなくなる。
var version = "dev"

func Version() string { return version }

var (
	ErrNotStarted = errors.New("no engine is running in this process")

	errListenFailed       = errors.New("the engine could not take a loopback port")
	errEngineStoppedEarly = errors.New("the engine stopped before it announced an entrance")
	errPrivateState       = errors.New("the Android private storage path is unavailable")
)

// running は、このプロセスで唯一の engine の状態を保持する。
var running struct {
	sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
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
	// Androidでは一つのapp processだけがengineを所有する。Serviceの再生成などで
	// Startが重複した場合は、利用者へ二重起動errorを見せず、古いengineの停止を
	// 待って同じ直列区間で置き換える。
	if running.cancel != nil {
		stopLocked()
	}
	resolvedHome, err := canonicalPrivateDirectory(home)
	if err != nil {
		return "", fail(KindStorageUnavailable, errors.Join(errPrivateState, err))
	}
	resolvedCache, err := canonicalPrivateDirectory(cache)
	if err != nil {
		return "", fail(KindStorageUnavailable, errors.Join(errPrivateState, err))
	}
	home, cache = resolvedHome, resolvedCache
	// gomobile bind は標準 log の出力先を logcat へ差し替える。slog をそこへ
	// 流し込めば、cgo を 1 行も書かずに logcat へ出る。
	logger := slog.New(slog.NewTextHandler(log.Writer(), &slog.HandlerOptions{Level: slog.LevelInfo}))

	entrance := make(chan string, 1)
	dependencies, err := newDependencies(runtime.GOOS, home, cache, logger, func(readiness app.Readiness) error {
		entrance <- readiness.Entrance
		return nil
	})
	if err != nil {
		return "", fail(KindUnknown, err)
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
		running.cancel, running.done = cancel, done
		running.lastKind = KindNone
		return url, nil
	case err := <-failed:
		cancel()
		<-done
		kind := KindListenFailed
		reason := errListenFailed
		if errors.Is(err, storage.ErrSymlinkPath) || errors.Is(err, storage.ErrNotDirectory) ||
			errors.Is(err, storage.ErrOutsideWorkspace) || errors.Is(err, storage.ErrInvalidHome) {
			kind = KindStorageUnavailable
			reason = errPrivateState
		}
		return "", fail(kind, errors.Join(reason, err))
	case <-done:
		cancel()
		return "", fail(KindStoppedEarly, errEngineStoppedEarly)
	}
}

// canonicalPrivateDirectory は /data/user/0 など、Android framework が返す
// private directory の別名だけを解決する。解決後のアプリ用rootより下では、
// engine側のdescriptor-relative walkerが引き続きsymlinkを拒否する。
func canonicalPrivateDirectory(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return "", errors.New("private storage path is not absolute")
	}
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("private storage path is not a directory")
	}
	return resolved, nil
}

// Stop は app.Run の終了を待つ。
func Stop() error {
	running.Lock()
	defer running.Unlock()
	if running.cancel == nil {
		return ErrNotStarted
	}
	stopLocked()
	return nil
}

// stopLocked はrunningのmutexを保持した呼び出し元から、現在のengineを停止する。
func stopLocked() {
	running.cancel()
	<-running.done
	running.cancel, running.done = nil, nil
}

// Start の失敗理由。gomobile が同じ定数を Java 側へ公開する。
const (
	KindNone = iota
	KindUnknown
	KindListenFailed
	KindStoppedEarly
	KindStorageUnavailable
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
