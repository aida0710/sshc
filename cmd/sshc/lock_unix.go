//go:build unix

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// lockEngineStart は、エンジンを起こす資格をひとつだけにする。
//
// **エンジンは 1 台でなければならない。** 2 台上がると、あとから handoff を
// 書いた方が勝ち、先に繋いだ画面は誰も見ていないエンジンを見続ける。見えない
// まま残るのは、生きた SSH 接続・PTY・転送したポート、そして解錠された vault
// である。
//
// **これを構造で塞ぐのは、状態を読んで決められないからである。** 「エンジンは
// 居るか」を尋ねてから起こすと、同時に起動した 2 つが両方「居ない」と読み、
// 両方が上がる。ロックは取れるか取れないかのどちらかで、その間に隙間が無い。
//
// flock を使うのは、**プロセスが死ねば必ず外れる**からである。O_EXCL で作った
// ファイルは、強制終了された起動が置いていったものと、いま握られているものを
// 区別できない——前者は永久に誰も起こせなくする。
//
// LOCK_NB を付けるのは、待つ意味が無いからである。既に 1 台居るなら、待って
// 2 台目になるのではなく、居る方の入口を出して終わるのが答えである。
func lockEngineStart(stateDir string) (func(), error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(stateDir, "engine.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errEngineRunning
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
