package storage

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// MaxFileSize は、設定ファイルひとつをメモリへ読み込む量の上限。実際の
	// クライアント設定はこれよりはるかに小さい。
	MaxFileSize = 1 << 20

	// DirectoryPermission は、このアプリケーションが作るディレクトリに適用される。
	DirectoryPermission fs.FileMode = 0o700
	// FilePermission は、管理対象ファイルが持ちうる最大の権限。既存のより厳しい権限は
	// そのまま保たれる。
	FilePermission fs.FileMode = 0o600
)

var (
	ErrFileTooLarge   = errors.New("file is larger than the supported maximum")
	ErrNotRegularFile = errors.New("path is not a regular file")
)

// FileSystem は、このパッケージがディスクに触れる唯一の経路。テストは
// OSFileSystem を包み、任意の段階で失敗を注入する。
type FileSystem interface {
	// ReadFile は、シンボリックリンクをたどらずに通常ファイルを読む。
	ReadFile(path string) ([]byte, error)
	Lstat(path string) (fs.FileInfo, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	// Glob は辞書順で一致を返す。
	Glob(pattern string) ([]string, error)
	MkdirAll(path string, permission fs.FileMode) error
	// WriteTemp は directory に新しいファイルを作り、contents を書き、permission を
	// 適用し、ディスクへフラッシュして、そのパスを返す。
	WriteTemp(directory, prefix string, permission fs.FileMode, contents []byte) (string, error)
	Rename(oldPath, newPath string) error
	// MovePrivate はオブジェクトの同一性と非公開状態のセキュリティ規則を維持したまま、
	// 既存の機密ファイルを移動する。
	MovePrivate(oldPath, newPath string) error
	Remove(path string) error
	SyncDir(path string) error
	EvalSymlinks(path string) (string, error)
}

// OSFileSystem は FileSystem のネイティブ OS 実装。
type OSFileSystem struct{}

// atomicFileWriter は、一時ファイルの作成から rename と親 directory の同期まで、
// 同じ検証済み directory handle に固定できるネイティブ実装である。FileSystem の
// fault-injection fake は従来どおり各段階を包めるよう、任意 interface にしておく。
type atomicFileWriter interface {
	WriteAtomic(path, prefix string, permission fs.FileMode, contents []byte) error
}

func (OSFileSystem) WriteAtomic(path, prefix string, permission fs.FileMode, contents []byte) error {
	return writeAtomicFileNative(path, prefix, permission, contents)
}

func (OSFileSystem) ReadFile(path string) ([]byte, error) {
	file, err := openRegularNoFollow(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readBoundedRegularFile(file)
}

func readBoundedRegularFile(file *os.File) ([]byte, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNotRegularFile
	}
	contents, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > MaxFileSize {
		return nil, ErrFileTooLarge
	}
	return contents, nil
}

func (OSFileSystem) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }

func (OSFileSystem) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }

func (OSFileSystem) Glob(pattern string) ([]string, error) { return filepath.Glob(pattern) }

func (OSFileSystem) MkdirAll(path string, permission fs.FileMode) error {
	return makePrivateDirectories(path, permission)
}

func (OSFileSystem) WriteTemp(directory, prefix string, permission fs.FileMode, contents []byte) (string, error) {
	return writeTemp(directory, prefix, permission, contents, createPrivateTemp)
}

type privateTempCreator func(directory, prefix string) (*os.File, error)

func writeTemp(directory, prefix string, permission fs.FileMode, contents []byte, create privateTempCreator) (string, error) {
	file, err := create(directory, prefix)
	if err != nil {
		if file != nil {
			path := file.Name()
			_ = file.Close()
			_ = removeNoFollow(path)
		}
		return "", err
	}
	if file == nil {
		return "", os.ErrInvalid
	}
	path := file.Name()
	if err := writeAndFlush(file, permission, contents); err != nil {
		_ = file.Close()
		_ = removeNoFollow(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = removeNoFollow(path)
		return "", err
	}
	return path, nil
}

func writeAndFlush(file *os.File, permission fs.FileMode, contents []byte) error {
	if err := file.Chmod(permission); err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		return err
	}
	return file.Sync()
}

func (OSFileSystem) Rename(oldPath, newPath string) error { return replaceFile(oldPath, newPath) }

func (OSFileSystem) MovePrivate(oldPath, newPath string) error {
	return movePrivateFile(oldPath, newPath)
}

func (OSFileSystem) Remove(path string) error { return removeNoFollow(path) }

func (OSFileSystem) SyncDir(path string) error { return syncDirectory(path) }

func (OSFileSystem) EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }

// WriteAtomicFile は同じディレクトリの非公開一時ファイルへ書き、1回のrenameで公開する。
// 失敗経路では一時ファイルを除去し、公開後は親ディレクトリを同期する。
func WriteAtomicFile(fileSystem FileSystem, path, prefix string, permission fs.FileMode, contents []byte) error {
	if writer, ok := fileSystem.(atomicFileWriter); ok {
		return writer.WriteAtomic(path, prefix, permission, contents)
	}
	return writeAtomicFileFallback(fileSystem, path, prefix, permission, contents)
}

func writeAtomicFileFallback(fileSystem FileSystem, path, prefix string, permission fs.FileMode, contents []byte) error {
	directory := filepath.Dir(path)
	temporary, err := fileSystem.WriteTemp(directory, prefix, permission, contents)
	if err != nil {
		return err
	}
	defer func() { _ = fileSystem.Remove(temporary) }()
	if err := fileSystem.Rename(temporary, path); err != nil {
		return err
	}
	return fileSystem.SyncDir(directory)
}
