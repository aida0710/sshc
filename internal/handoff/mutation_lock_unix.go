//go:build unix

package handoff

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockMutation は Write 全体と Remove の比較・削除を一つの臨界区間にする。
// flock はプロセス終了時に必ず外れるため、強制終了した engine の lock が次の
// cleanup を永久に止めない。
func lockMutation(directory string) (func(), error) {
	file, err := os.OpenFile(filepath.Join(directory, mutationLockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
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
