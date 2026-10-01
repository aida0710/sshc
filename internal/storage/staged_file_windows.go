//go:build windows

package storage

import (
	"os"

	"golang.org/x/sys/windows"
)

// stagedTemporaryは、一時ファイルと、それを作ったディレクトリのハンドルである。Windowsでは
// renameも一時ファイルのハンドルに対して行う（renamePrivateFileHandle）。作成からrenameまでを
// 同じディレクトリのハンドルに対して行い、その間にパスの途中がjunctionなどへ差し替えられても、
// 別のディレクトリへ公開しない。
type stagedTemporary struct {
	parent windows.Handle
	// fileは、finishPublishが閉じたあとはnilである。
	file *os.File
}

// cleanStageDirectoryは、StageFileの置き場所を、ボリュームを持つ絶対パスに均す。デバイスの
// パスや、代替データストリームの名前は断る（cleanAbsoluteDOSPath）。
func cleanStageDirectory(directory string) (string, error) {
	return cleanAbsoluteDOSPath(directory)
}

// stageFileNativeは、StageFileとwriteAtomicFileNativeWithが共有する手順である。
// request.Directoryは、呼び出し側が確かめた絶対パスである（cleanStageDirectory、
// cleanAtomicTarget）。afterParentOpenは、ディレクトリを開いた直後に呼ぶテストの差し込み口で、
// そこでパスを差し替えても、開いたディレクトリへ書くことを確かめるのに使う。本番ではnilである。
func stageFileNative(request StageRequest, afterParentOpen func()) (*StagedFile, error) {
	parent, err := openNoReparseDirectoryWithAccess(request.Directory, windows.FILE_TRAVERSE|windows.FILE_WRITE_DATA|fileDeleteChild)
	if err != nil {
		return nil, err
	}
	if afterParentOpen != nil {
		afterParentOpen()
	}
	file, err := createPrivateTempRelative(parent, request.Directory, request.Prefix)
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

// finishPublishは、公開した一時ファイルのハンドルを閉じ、その失敗を返す。Windowsには
// ディレクトリのfsyncに当たるAPIが無い（syncDirectoryを参照）。中身はcopyAndFlushのSyncで、
// renameはFILE_WRITE_THROUGHで開いたハンドルでディスクへ流れるので、それ以上の永続性は
// 装わない。
func (t *stagedTemporary) finishPublish() error {
	file := t.file
	t.file = nil
	return file.Close()
}

func (t *stagedTemporary) remove() { _ = discardFileHandle(windows.Handle(t.file.Fd())) }

func (t *stagedTemporary) release() {
	if t.file != nil {
		_ = t.file.Close()
	}
	_ = windows.CloseHandle(t.parent)
}
