//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockEngineStart は、エンジンを起こす資格をひとつだけにする。
//
// **二つのアプリが同時に起動しても、エンジンはひとつでなければならない。**
// 二つ動くと、あとから handoff を書いた方が勝ち、先に繋いだ画面は誰も
// 見ていないエンジンを見続ける。
//
// flock を使うのは、**プロセスが死ねば必ず外れる**からである。O_EXCL で
// 作ったファイルは、強制終了された起動が置いていったものと、いま握られて
// いるものを区別できない——前者は永久に誰も起こせなくする。
func lockEngineStart(stateDir string) (func(), error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(stateDir, "engine.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
