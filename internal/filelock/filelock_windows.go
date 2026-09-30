//go:build windows

package filelock

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

var afterLockDirectoryOpen func()

// lockedByteCount は 1 である。ロックされた範囲そのものは読まれない。必要なのは
// 「誰かが握っている」という OS の事実だけであり、ファイルの中身ではない。
const lockedByteCount = 1

// tryAcquire は LockFileEx を LOCKFILE_FAIL_IMMEDIATELY で使う。プロセスが死ねば
// OS がハンドルを閉じ、ロックはそこで必ず外れる。
//
// ディレクトリとファイルは windowsacl の同一ハンドル owner/DACL/reparse 契約を
// 通す。ロックファイルは秘密を持たないが所有の証拠であり、別のユーザーが
// 書けるロックは直列化そのものを歪められる。
func tryAcquire(path string) (func() error, error) {
	directory, err := windowsacl.OpenPrivateDirectory(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	closeDirectory := true
	defer func() {
		if closeDirectory {
			_ = directory.Close()
		}
	}()
	if afterLockDirectoryOpen != nil {
		afterLockDirectoryOpen()
	}
	file, err := windowsacl.OpenOrCreateFileAt(directory, filepath.Base(path))
	if err != nil {
		// 共有違反は ErrHeld にしない。ここの開き方は常に
		// FILE_SHARE_READ|WRITE|DELETE なので、sshc 同士がこれを起こすことは
		// ない。起こすのはウイルス対策やインデクサであり、それを「別の取得が
		// 握っている」と言えば、誰も握っていないのに誰も取れなくなる。
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
			lockErr = ErrHeld
		}
		return nil, errors.Join(lockErr, closeErr)
	}
	closeDirectory = false
	return newReleaseWithDirectory(file, directory, func() error {
		return windows.UnlockFileEx(handle, 0, lockedByteCount, 0, overlapped)
	}), nil
}
