//go:build windows

package storage

import (
	"os"

	"golang.org/x/sys/windows"
)

// stagedTemporary は、一時ファイルと、それを作ったディレクトリのハンドルである。
// Windows では rename も一時ファイルのハンドルに対して行う（renamePrivateFileHandle）。
// 作成から rename までを同じディレクトリのハンドルに対して行い、その間にパスの途中が
// junction などへ差し替えられても、別のディレクトリへ公開しない。
type stagedTemporary struct {
	parent windows.Handle
	file   *os.File
}

func stageFileNative(request StageRequest) (*StagedFile, error) {
	absolute, err := cleanAbsoluteDOSPath(request.Directory)
	if err != nil {
		return nil, err
	}
	parent, err := openNoReparseDirectoryWithAccess(absolute, windows.FILE_TRAVERSE|windows.FILE_WRITE_DATA|fileDeleteChild)
	if err != nil {
		return nil, err
	}
	file, err := createPrivateTempRelative(parent, absolute, request.Prefix)
	if err != nil {
		_ = windows.CloseHandle(parent)
		return nil, err
	}
	temporary := &stagedTemporary{parent: parent, file: file}
	size, err := copyAndFlush(file, request)
	if err != nil {
		temporary.remove()
		temporary.release()
		return nil, err
	}
	return &StagedFile{directory: request.Directory, size: size, temporary: temporary}, nil
}

func (t *stagedTemporary) rename(name string) error {
	return renamePrivateFileHandle(windows.Handle(t.file.Fd()), t.parent, name)
}

// syncRename は、一時ファイルのハンドルを閉じ、その失敗を返す。Windows には
// ディレクトリの fsync に当たる API が無い（syncDirectory を参照）。中身は
// copyAndFlush の Sync で、rename は FILE_WRITE_THROUGH で開いたハンドルで
// ディスクへ流れるので、それ以上の永続性は装わない。
func (t *stagedTemporary) syncRename() error { return t.file.Close() }

func (t *stagedTemporary) remove() { _ = discardFileHandle(windows.Handle(t.file.Fd())) }

func (t *stagedTemporary) release() {
	_ = t.file.Close()
	_ = windows.CloseHandle(t.parent)
}
