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
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

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
	errEngineStartFailed  = errors.New("the engine could not initialize")
	errEngineStoppedEarly = errors.New("the engine stopped before it announced an entrance")
	errPrivateState       = errors.New("the Android private storage path is unavailable")
)

const maxFailureDetailBytes = 1024

var (
	credentialParameter = regexp.MustCompile(`(?i)\b(bootstrap|token|secret[_-]?access[_-]?key|access[_-]?key(?:[_-]?id)?|secret|password|passphrase|authorization)\s*[=:]\s*([^\s&#]+)`)
	bearerCredential    = regexp.MustCompile(`(?i)\b(bearer)\s+([A-Za-z0-9._~+/=-]+)`)
	urlUserInfo         = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/@\s]+@`)
)

// running は、このプロセスで唯一の engine の状態を保持する。
var running struct {
	sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	// lastKind は、gomobile の error 変換で型情報が失われないよう、直前の
	// Start の失敗理由を数値で保持する。
	lastKind   int
	lastDetail string
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
		running.lastDetail = ""
		return url, nil
	case err := <-failed:
		cancel()
		<-done
		kind, reason := classifyStartFailure(err)
		return "", fail(kind, errors.Join(reason, err))
	case <-done:
		cancel()
		// app.Runは具体的な失敗をfailedへ送ってからdoneを閉じる。selectがdoneを
		// 先に選んでも、既に届いている理由をStoppedEarlyへ潰さない。
		select {
		case err := <-failed:
			kind, reason := classifyStartFailure(err)
			return "", fail(kind, errors.Join(reason, err))
		default:
		}
		return "", fail(KindStoppedEarly, errEngineStoppedEarly)
	}
}

func classifyStartFailure(err error) (int, error) {
	switch {
	case errors.Is(err, storage.ErrSymlinkPath), errors.Is(err, storage.ErrNotDirectory),
		errors.Is(err, storage.ErrOutsideWorkspace), errors.Is(err, storage.ErrInvalidHome):
		return KindStorageUnavailable, errPrivateState
	case errors.Is(err, app.ErrListen):
		return KindListenFailed, errListenFailed
	default:
		return KindEngineStartFailed, errEngineStartFailed
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
	KindEngineStartFailed
)

// fail は running の mutex を保持した状態で失敗理由を記録する。
func fail(kind int, err error) error {
	running.lastKind = kind
	running.lastDetail = safeFailureDetail(err)
	return err
}

// safeFailureDetail は画面、clipboard、logcatへ渡せる診断文へ縮約する。
// 入口資格情報、一般的なcredential parameter、URL userinfo、制御文字を除去する。
func safeFailureDetail(err error) string {
	if err == nil {
		return ""
	}
	detail := credentialParameter.ReplaceAllString(err.Error(), `${1}=[redacted]`)
	detail = bearerCredential.ReplaceAllString(detail, `${1} [redacted]`)
	detail = urlUserInfo.ReplaceAllString(detail, `${1}[redacted]@`)
	detail = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, detail)
	detail = strings.Join(strings.Fields(detail), " ")
	if len(detail) <= maxFailureDetailBytes {
		return detail
	}
	cut := maxFailureDetailBytes - len("…")
	for cut > 0 && !utf8.RuneStart(detail[cut]) {
		cut--
	}
	return detail[:cut] + "…"
}

// LastStartFailureKind は直前の Start の失敗理由を返す。エラー文に含まれ得る
// bootstrap fragment を Java や logcat へ渡さない。
func LastStartFailureKind() int {
	running.Lock()
	defer running.Unlock()
	return running.lastKind
}

// LastStartFailureCode は利用者が共有できる安定した診断codeを返す。
func LastStartFailureCode() string {
	running.Lock()
	defer running.Unlock()
	switch running.lastKind {
	case KindNone:
		return "none"
	case KindListenFailed:
		return "port_unavailable"
	case KindStoppedEarly:
		return "engine_stopped_early"
	case KindStorageUnavailable:
		return "storage_unavailable"
	case KindEngineStartFailed:
		return "engine_start_failed"
	default:
		return "unknown"
	}
}

// LastStartFailureDetail は伏せ字化・長さ制限済みの診断文を返す。
func LastStartFailureDetail() string {
	running.Lock()
	defer running.Unlock()
	return running.lastDetail
}
