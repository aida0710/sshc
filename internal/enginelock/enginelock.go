// Package enginelock は engine の単一プロセス制約を OS のファイルロックで実装する。
// 状態確認後の起動には競合があるため、取得が原子的でプロセス終了時に解放される
// OS ロックを使用する。
package enginelock

import (
	"errors"
	"os"
	"sync"
)

// ErrRunning は別プロセスが engine lock を保持していることを表す。
var ErrRunning = errors.New("an sshc engine is already running")

// ErrUnsafeStateDirectory は lock の親パスが実ディレクトリでないことを表す。
var ErrUnsafeStateDirectory = errors.New("engine lock directory is not a directory")

// Acquire は path の lock を待機せず取得する。親ディレクトリは所有者だけが
// アクセスできる権限へ設定される。release は並行かつ複数回呼び出せる。
func Acquire(path string) (func() error, error) {
	return acquire(path)
}

// newRelease は unlock と close を一度だけ実行し、結果を後続呼び出しへ返す。
func newRelease(file *os.File, unlock func() error) func() error {
	var once sync.Once
	var result error
	return func() error {
		once.Do(func() { result = errors.Join(unlock(), file.Close()) })
		return result
	}
}
