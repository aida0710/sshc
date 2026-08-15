//go:build windows

package enginelock

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

// lockedByteCount は 1 である。ロックされた範囲そのものは読まれない。必要なのは
// 「誰かが握っている」という OS の事実だけであり、ファイルの中身ではない。
const lockedByteCount = 1

// acquire は LockFileEx を LOCKFILE_FAIL_IMMEDIATELY で使う。プロセスが死ねば
// OS がハンドルを閉じ、ロックはそこで必ず外れる。
//
// ディレクトリとファイルは windowsacl の同一ハンドル owner/DACL/reparse 契約を
// 通す。ロックファイルは秘密を持たないが engine 所有権の証拠であり、他人が
// 書けるロックは所有の直列化そのものを歪められる。
func acquire(path string) (func() error, error) {
	if err := windowsacl.EnsureDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	file, err := windowsacl.OpenOrCreateFile(path)
	if err != nil {
		// 共有違反は、他の誰かがこのファイルを排他で開いているという観測である。
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, ErrRunning
		}
		return nil, err
	}
	handle := windows.Handle(file.Fd())
	overlapped := new(windows.Overlapped)
	if lockErr := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		lockedByteCount,
		0,
		overlapped,
	); lockErr != nil {
		closeErr := file.Close()
		if errors.Is(lockErr, windows.ERROR_LOCK_VIOLATION) {
			lockErr = ErrRunning
		}
		return nil, errors.Join(lockErr, closeErr)
	}
	return newRelease(file, func() error {
		return windows.UnlockFileEx(handle, 0, lockedByteCount, 0, overlapped)
	}), nil
}
