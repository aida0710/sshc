//go:build !windows

package storage

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// stagedTemporary は、一時ファイルの名前と、それを作ったディレクトリの fd である。
// 作成から rename までを同じ fd に対して行い、その間にパスの途中がシンボリック
// リンクへ差し替えられても、別のディレクトリへ公開しない。
type stagedTemporary struct {
	parent *os.File
	name   string
}

func stageFileNative(request StageRequest) (*StagedFile, error) {
	parent, err := openDirectoryNoFollow(request.Directory)
	if err != nil {
		return nil, err
	}
	file, err := createPrivateTempAt(parent, request.Directory, request.Prefix)
	if err != nil {
		_ = parent.Close()
		return nil, err
	}
	temporary := &stagedTemporary{parent: parent, name: filepath.Base(file.Name())}
	size, err := copyAndFlush(file, request)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		temporary.remove()
		temporary.release()
		return nil, err
	}
	return &StagedFile{directory: request.Directory, size: size, temporary: temporary}, nil
}

func (t *stagedTemporary) rename(name string) error {
	return unix.Renameat(int(t.parent.Fd()), t.name, int(t.parent.Fd()), name)
}

// syncRename は、rename したディレクトリの変更をディスクへ流す。
func (t *stagedTemporary) syncRename() error { return t.parent.Sync() }

func (t *stagedTemporary) remove() { _ = unix.Unlinkat(int(t.parent.Fd()), t.name, 0) }

func (t *stagedTemporary) release() { _ = t.parent.Close() }
