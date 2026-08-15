//go:build unix

package enginelock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// acquire は flock(LOCK_EX|LOCK_NB) を使う。open file description ごとに効くので、
// 同じプロセスが開き直した 2 本目でも、別プロセスと同じように弾かれる。
func acquire(path string) (func() error, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	if err := ensurePrivateDirectory(directory); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	// 既にあるファイルには OpenFile の perm が効かない。開いた fd 越しに直すので、
	// パスを取り違える隙が無い。
	if err := file.Chmod(0o600); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if lockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); lockErr != nil {
		closeErr := file.Close()
		if errors.Is(lockErr, syscall.EWOULDBLOCK) {
			lockErr = ErrRunning
		}
		return nil, errors.Join(lockErr, closeErr)
	}
	descriptor := int(file.Fd())
	return newRelease(file, func() error { return syscall.Flock(descriptor, syscall.LOCK_UN) }), nil
}

// ensurePrivateDirectory は、緩いときだけ締め直す。
//
// MkdirAll は umask を通り、既にあるディレクトリの権限には触れない。他人が書ける
// ディレクトリに置かれたロックは、別のファイルに差し替えられる — そうなれば 2 台の
// エンジンがそれぞれ別の inode を握り、ロックの意味が消える。
//
// 既に閉じているなら何もしない。無条件に chmod すると、mode を持たないファイル
// システムでは、それだけを理由にエンジンが起動できなくなる。
func ensurePrivateDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	// シンボリックリンクは Lstat では IsDir にならない。リンク越しの chmod は、
	// 誰が差し替えたか分からない先の権限を書き換えることになる。
	if !info.IsDir() {
		return fmt.Errorf("%w: %s", ErrUnsafeStateDirectory, directory)
	}
	if info.Mode().Perm()&0o077 == 0 {
		return nil
	}
	return os.Chmod(directory, 0o700)
}
