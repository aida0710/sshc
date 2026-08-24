// Package sftp は、SSH 接続上のリモートファイル操作を提供する。
package sftp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"time"
)

const MaxEditableFileBytes = 2 << 20

var (
	ErrUnavailable      = errors.New("sftp service is unavailable")
	ErrInvalidAlias     = errors.New("ssh alias is required")
	ErrInvalidPath      = errors.New("remote path must be an absolute POSIX path")
	ErrRootOperation    = errors.New("operation on the remote root is not allowed")
	ErrNotRegularFile   = errors.New("remote path is not a regular file")
	ErrNotDirectory     = errors.New("remote path is not a directory")
	ErrTextTooLarge     = errors.New("remote file is too large to edit")
	ErrNotUTF8          = errors.New("remote file is not UTF-8 text")
	ErrConflict         = errors.New("remote file changed since it was read")
	ErrAlreadyExists    = errors.New("remote path already exists")
	ErrRevisionRequired = errors.New("content revision is required")
	ErrTransferTooLarge = errors.New("uploaded file exceeds the requested size limit")
	ErrInvalidTransfer  = errors.New("invalid transfer identifier")
	ErrOffsetMismatch   = errors.New("upload offset does not match the remote part file")
	ErrUploadIncomplete = errors.New("upload part file is incomplete")
	ErrTransferNotFound = errors.New("transfer job was not found")
	ErrTransferState    = errors.New("transfer job state transition is invalid")
	ErrTransferLimit    = errors.New("transfer concurrency limit reached")
)

type EntryType string

const (
	EntryFile      EntryType = "file"
	EntryDirectory EntryType = "directory"
	EntrySymlink   EntryType = "symlink"
	EntryOther     EntryType = "other"
)

// Entry は、file browser が表示するひとつのリモート項目である。
// Revision は metadata revision であり、upload の競合検査に使える。
type Entry struct {
	Name       string
	Path       string
	Type       EntryType
	Size       int64
	Mode       fs.FileMode
	ModifiedAt time.Time
	Revision   string
}

// TextFile は、editor に渡せる UTF-8 ファイルである。
// Revision は内容を含む revision であり、SaveText へそのまま返す。
type TextFile struct {
	Entry    Entry
	Contents string
	Revision string
}

type Transfer struct {
	Path     string
	Bytes    int64
	Revision string
}

type ResumableUpload struct {
	ID               string
	Path             string
	Offset           int64
	Size             int64
	ExpectedRevision string
}

type StartUploadOptions struct {
	Size             int64
	Overwrite        bool
	ExpectedRevision string
}

type WriteSeekCloser interface {
	io.Writer
	io.Seeker
	io.Closer
}

type UploadOptions struct {
	// Overwrite は、既存ファイルを置換してよいことを呼び出し側が確認した場合だけ指定する。
	Overwrite bool
	// ExpectedRevision があれば、既存ファイルの metadata revision が一致する場合だけ置換する。
	ExpectedRevision string
	// MaxBytes は 0 なら転送量を制限しない。
	MaxBytes int64
	// Mode は新規ファイルへ適用する。0 なら 0600。既存ファイルでは現在の mode を維持する。
	Mode fs.FileMode
}

// Remote は SFTP client のうち service が使う操作だけを表す。
// 実装は同じ接続に対する呼び出しを直列化する必要はない。Service は操作ごとに接続を開く。
type Remote interface {
	io.Closer
	ReadDir(ctx context.Context, path string) ([]fs.FileInfo, error)
	Lstat(path string) (fs.FileInfo, error)
	ReadLink(path string) (string, error)
	Open(path string) (io.ReadCloser, error)
	Create(path string) (io.WriteCloser, error)
	OpenFile(path string, flags int) (WriteSeekCloser, error)
	Mkdir(path string) error
	Chmod(path string, mode fs.FileMode) error
	Replace(oldPath, newPath string) error
	Rename(oldPath, newPath string) error
	Remove(path string) error
	RemoveDirectory(path string) error
}

type OpenRemote func(ctx context.Context, alias string) (Remote, error)
