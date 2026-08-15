//go:build windows

package handoff

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

// lockMutation は Windows でも同じファイル範囲を排他的に握る。LockFileEx は
// blocking で待つため、別 process の Write と Remove の比較・削除にも隙間がない。
func lockMutation(directory string) (func(), error) {
	file, err := os.OpenFile(filepath.Join(directory, mutationLockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := windowsacl.RestrictFile(file.Name()); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	handle := windows.Handle(file.Fd())
	overlapped := windows.Overlapped{}
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() {
		_ = windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
		_ = file.Close()
	}, nil
}
