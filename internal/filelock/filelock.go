// Package filelock は、別プロセスとの排他を OS のファイルロックで取る。
//
// 状態を確かめてから作る方式は、確かめてから作るまでのあいだに競合する。OS の
// ファイルロックは取得が原子的で、プロセスが終われば OS が必ず外すので、強制終了した
// プロセスのロックが残り続けることもない。
//
// 待ち方は、握っている相手が何をしているかで選ぶ。
//   - TryAcquire: 相手が握っているなら、その相手が同じ仕事をしている。待たずに断る
//     （engine の二重起動、サービスの登録と解除）。
//   - AcquireWithin: 相手の臨界区間は短い。終わるまで決めた時間だけ待つ
//     （ワークスペースの書き込み、handoff の書き込みと削除）。
package filelock

import (
	"errors"
	"os"
	"sync"
	"time"
)

// ErrHeld は、別の取得がロックを握っていることを表す。同じプロセスの別の取得でも
// 同じように断る。
var ErrHeld = errors.New("the lock is held by another process")

// ErrUnsafeDirectory は、ロックファイルの親パスが実ディレクトリでないことを表す。
var ErrUnsafeDirectory = errors.New("lock directory is not a directory")

// retryInterval は、AcquireWithin が取り直すまでの間隔である。OS には、別プロセスが
// ロックを外したことを待てる手段がない（待つ取得は取り消せない）ので取り直す。
// 臨界区間は数ミリ秒で終わるので、それより長く待たせない程度に短くする。
const retryInterval = 10 * time.Millisecond

// TryAcquire は path のロックを待たずに取る。握られていれば ErrHeld を返す。
// 親ディレクトリは無ければ作り、所有者だけが使える権限にする。返す release は
// 並行にも複数回にも呼べる。
func TryAcquire(path string) (func() error, error) {
	return tryAcquire(path)
}

// AcquireWithin は path のロックを、wait のあいだ取り直しながら取る。wait を過ぎても
// 握られていれば ErrHeld を返す。ErrHeld 以外の失敗は待たずに返す。
func AcquireWithin(path string, wait time.Duration) (func() error, error) {
	deadline := time.Now().Add(wait)
	for {
		release, err := tryAcquire(path)
		if !errors.Is(err, ErrHeld) || time.Now().After(deadline) {
			return release, err
		}
		time.Sleep(retryInterval)
	}
}

// newReleaseWithDirectory keeps the directory identity alive until the lock is
// released. The lock file was opened relative to this handle, so both resources
// are one acquisition and must be closed exactly once together.
func newReleaseWithDirectory(file, directory *os.File, unlock func() error) func() error {
	var once sync.Once
	var result error
	return func() error {
		once.Do(func() {
			result = errors.Join(unlock(), file.Close(), directory.Close())
		})
		return result
	}
}
