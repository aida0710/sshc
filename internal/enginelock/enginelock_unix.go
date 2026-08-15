//go:build unix

package enginelock

import (
	"errors"
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
	// MkdirAll は umask を通り、既にあるディレクトリの権限を変えない。所有権の
	// 直列化を他人が書ける場所に置かないため、ここで締め直す。
	if err := os.Chmod(directory, 0o700); err != nil {
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
