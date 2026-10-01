//go:build !windows

package storage

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// stagedTemporaryは、一時ファイルの名前と、それを作ったディレクトリのfdである。作成から
// renameまでを同じfdに対して行い、その間にパスの途中がシンボリックリンクへ差し替えられても、
// 別のディレクトリへ公開しない。
type stagedTemporary struct {
	parent *os.File
	name   string
}

// cleanStageDirectoryは、StageFileの置き場所を絶対パスに均す。
func cleanStageDirectory(directory string) (string, error) {
	cleaned := filepath.Clean(directory)
	if !filepath.IsAbs(cleaned) {
		return "", os.ErrInvalid
	}
	return cleaned, nil
}

// stageFileNativeは、StageFileとwriteAtomicFileNativeWithが共有する手順である。
// request.Directoryは、呼び出し側が確かめた絶対パスである（cleanStageDirectory、
// cleanAtomicTarget）。afterParentOpenは、ディレクトリを開いた直後に呼ぶテストの差し込み口で、
// そこでパスを差し替えても、開いたディレクトリへ書くことを確かめるのに使う。本番ではnilである。
func stageFileNative(request StageRequest, afterParentOpen func()) (*StagedFile, error) {
	parent, err := openDirectoryNoFollow(request.Directory)
	if err != nil {
		return nil, err
	}
	if afterParentOpen != nil {
		afterParentOpen()
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

// finishPublishは、renameしたディレクトリの変更をディスクへ流す。
func (t *stagedTemporary) finishPublish() error { return t.parent.Sync() }

func (t *stagedTemporary) remove() { _ = unix.Unlinkat(int(t.parent.Fd()), t.name, 0) }

func (t *stagedTemporary) release() { _ = t.parent.Close() }
