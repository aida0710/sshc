// Package mobile は、この engine を Android アプリの中から起こすための境界である。
//
// **ここに置くものは gomobile が bind できる形でなければならない。** 渡せるのは
// 文字列と数値と error だけなので、構造体も interface も公開しない。それは制約
// ではなく、この境界に必要なものがそれだけだという事実の反映である。
package mobile

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"path/filepath"
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

// running は、このプロセスの唯一の engine である。
//
// **構造体を bind して Kotlin にインスタンスを持たせない。** 1 プロセスに
// engine は 1 台という制約は Android では設計判断ではなく事実であり、複数
// 持てる形を見せれば、持てないものを持てるように見せることになる。
var running struct {
	sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	release func() error
	// lastKind は、直前の Start が失敗した理由である。
	//
	// **なぜ error ではなく番号を別に持つのか。** gomobile は Go の error を
	// Java の Exception へ写すとき、メッセージ文字列しか運ばない。Kotlin が
	// 受け取った Exception を Go へ返しても、それは元の error ではなく同じ
	// 文面を持つ別物になるので、errors.Is はこの境界を越えられない。理由の
	// 区別は Go 側で確定させ、Kotlin は番号だけを取りに来る。
	lastKind int
}

// Start は engine を起こし、入口の URL を返す。
//
// 返るのは listener が bind され Announce が呼ばれた後である。呼び出し側は
// この URL を即座に WebView へ渡すので、早く返せば空のページが出る。
func Start(home, cache string) (string, error) {
	running.Lock()
	defer running.Unlock()
	if running.cancel != nil {
		return "", fail(kindAlreadyStarted, ErrAlreadyStarted)
	}

	// **ロックは engine より先である。** 2 台目が app.Run へ入ってから落ちると、
	// その一瞬だけ同じ状態ディレクトリを 2 つが握る。
	release, err := enginelock.Acquire(filepath.Join(app.HandoffDir(home), "engine.lock"))
	if err != nil {
		return "", fail(kindAlreadyStarted, err)
	}

	// gomobile bind は標準 log の出力先を logcat へ差し替える。slog をそこへ
	// 流し込めば、cgo を 1 行も書かずに logcat へ出る。
	logger := slog.New(slog.NewTextHandler(log.Writer(), &slog.HandlerOptions{Level: slog.LevelInfo}))

	entrance := make(chan string, 1)
	dependencies, err := newDependencies(home, cache, logger, func(readiness app.Readiness) error {
		entrance <- readiness.Entrance
		return nil
	})
	if err != nil {
		return "", fail(kindUnknown, errors.Join(err, release()))
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
		running.lastKind = kindNone
		return url, nil
	case err := <-failed:
		cancel()
		<-done
		return "", fail(kindListenFailed, errors.Join(errListenFailed, err, release()))
	case <-done:
		cancel()
		return "", fail(kindStoppedEarly, errors.Join(errEngineStoppedEarly, release()))
	}
}

// Stop は engine を止め、ロックを手放す。
//
// **app.Run が戻るまで待つ。** 待たずにロックを手放すと、次の Start が
// まだ握られているポートに bind しに行く。
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

// Start が失敗した理由。**Kotlin はこの番号だけを見る。**
const (
	kindNone = iota
	kindUnknown
	kindAlreadyStarted
	kindListenFailed
	kindStoppedEarly
)

// fail は、失敗の理由を記録してからその error を返す。
//
// **running の mutex を握ったまま呼ぶこと。** Start と LastStartFailureKind が
// 同じ錠の下に居るので、Kotlin が catch してから番号を取りに来るまでの間に
// 別の Start が理由を上書きすることはない。
func fail(kind int, err error) error {
	running.lastKind = kind
	return err
}

// LastStartFailureKind は、直前の Start が失敗した理由を番号ひとつで返す。
//
// **error そのものを Kotlin へ渡して番号に畳ませない。** gomobile は Go の
// error を Java の Exception へ写すときメッセージ文字列しか運ばないので、
// 戻ってきた値に errors.Is は効かない。それに、engine の error は入口の URL を
// 含み得る——文面を境界の向こうへ出せば logcat とエラー画面に bootstrap
// fragment が残る。文面は Android 側が持ち、こちらは区別だけを渡す。
func LastStartFailureKind() int {
	running.Lock()
	defer running.Unlock()
	return running.lastKind
}
