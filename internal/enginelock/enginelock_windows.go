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
// 通す。ロックファイルは秘密を持たないが engine 所有権の証拠であり、別のユーザーが
// 書けるロックは所有の直列化そのものを歪められる。
func acquire(path string) (func() error, error) {
	if err := windowsacl.EnsureDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	file, err := windowsacl.OpenOrCreateFile(path)
	if err != nil {
		// 共有違反は ErrRunning にしない。エンジンの開き方は常に
		// FILE_SHARE_READ|WRITE|DELETE なので、エンジン同士がこれを起動することは
		// ない。起動するのはウイルス対策やインデクサであり、それを「既にエンジンが
		// 居る」と言えば、誰も居ないのに誰も起動できなくなる。
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
