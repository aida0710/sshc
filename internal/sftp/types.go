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

// MaxPreviewFileBytes は、一覧の詳細モーダルが画像を表示するために engine が
// 丸ごと読んでよい上限である。編集用より大きいのは写真が 2 MiB を超えるのが
// 普通だからで、これを超えるものは preview ではなく download に回す。
//
// data: URL は base64 なので、browser 側では約 4/3 倍の文字列になる。
const MaxPreviewFileBytes = 8 << 20

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
	ErrTransferTooLarge = errors.New("transferred file exceeds the requested size limit")
	ErrInvalidTransfer  = errors.New("invalid transfer identifier")
	ErrOffsetMismatch   = errors.New("upload offset does not match the remote part file")
	ErrUploadIncomplete = errors.New("upload part file is incomplete")
	ErrTransferNotFound = errors.New("transfer job was not found")
	ErrTransferState    = errors.New("transfer job state transition is invalid")
	ErrTransferLimit    = errors.New("transfer concurrency limit reached")
	ErrPreviewTooLarge  = errors.New("remote file is too large to preview")
	ErrPreviewType      = errors.New("remote file has no previewable type")
	ErrInvalidQuery     = errors.New("search needs something to look for")
	ErrUnsupportedEntry = errors.New("remote entry type cannot be copied")
	ErrCompareLimit     = errors.New("directory comparison exceeded its safety limit")
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

// Listing は、SFTP serverが正規化した絶対パスと、その直下の項目をまとめる。
// Pathは初期表示でserverの作業ディレクトリを解決した場合にも絶対パスになる。
type Listing struct {
	Path    string
	Entries []Entry
}

// TextFile は、editor に渡せる UTF-8 ファイルである。
// Revision は内容を含む revision であり、SaveText へそのまま返す。
type TextFile struct {
	Entry    Entry
	Contents string
	Revision string
}

// SearchResult は、あるディレクトリ配下の名前一致である。
//
// Truncated は、予算のどれかに当たって歩き切らずに戻ったことを言う。
// 「これで全部だ」と言えないことを、画面がそのまま言えるようにする。
type SearchResult struct {
	Path      string
	Query     string
	Entries   []Entry
	Truncated bool
}

type DirectoryDifferenceStatus string

const (
	DirectorySame         DirectoryDifferenceStatus = "same"
	DirectoryDifferent    DirectoryDifferenceStatus = "different"
	DirectoryLeftOnly     DirectoryDifferenceStatus = "left_only"
	DirectoryRightOnly    DirectoryDifferenceStatus = "right_only"
	DirectoryTypeMismatch DirectoryDifferenceStatus = "type_mismatch"
)

// DirectoryDifference describes one relative path below two independently
// connected SFTP roots. Entries are pointers because one side may not exist.
type DirectoryDifference struct {
	RelativePath string
	Status       DirectoryDifferenceStatus
	Left         *Entry
	Right        *Entry
}

type DirectoryComparison struct {
	LeftPath  string
	RightPath string
	Entries   []DirectoryDifference
}

type RemoteTransferOperation string

const (
	RemoteCopy RemoteTransferOperation = "copy"
	RemoteMove RemoteTransferOperation = "move"
)

type RemoteTransferRequest struct {
	SourceAlias string
	SourcePath  string
	TargetAlias string
	TargetPath  string
	Operation   RemoteTransferOperation
	Overwrite   bool
}

type RemoteTransferPlan struct {
	Kind       TransferKind
	Name       string
	TotalBytes int64
}

// Preview は、詳細モーダルがそのまま描ける bytes である。ContentType は
// 中身から決まった許可済みの型だけを取り、名前も SFTP server の申告も見ない。
type Preview struct {
	Entry       Entry
	ContentType string
	Contents    []byte
	Revision    string
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
	Size              int64
	Overwrite         bool
	ExpectedRevision  string
	SourceFingerprint string
}

type WriteSeekCloser interface {
	io.Writer
	io.Seeker
	io.Closer
}

// Remote は SFTP client のうち service が使う操作だけを表す。
// 実装は同じ接続に対する呼び出しを直列化する必要はない。Service は操作ごとに接続を開く。
type Remote interface {
	io.Closer
	Getwd(ctx context.Context) (string, error)
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

// RangeRemote は、ひとつのremote fileを指定offsetから読める実装である。
// 大きなdownloadは複数の独立したSFTP connectionから別々の範囲を読む。
// 対応しないRemoteは従来どおり1本のstreamへ戻す。
type RangeRemote interface {
	OpenRange(path string, offset int64) (io.ReadCloser, error)
}

type OpenRemote func(ctx context.Context, alias string) (Remote, error)
