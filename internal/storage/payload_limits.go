package storage

import (
	"path/filepath"
	"strings"
)

// MaxAssetFileSize is the supported background asset size. Config loaders keep
// MaxFileSize; only transaction payloads and explicit asset readers use this.
const MaxAssetFileSize = 1024 << 20

// BackgroundsDirectory は、ターミナルの背景画像を置くディレクトリ。ワークスペース
// からのスラッシュ区切りの相対パス。ここのファイルだけが MaxAssetFileSize まで
// 大きくてよいので、画像を置く application も、上限を決める PayloadLimit も、
// この定数からパスを組み立てる。
const BackgroundsDirectory = StateDirectoryName + "/backgrounds"

// PayloadLimit は、ワークスペースからのスラッシュ区切りの相対パス relative に置く
// ファイルの大きさの上限を返す。背景画像は MaxAssetFileSize、ほかは MaxFileSize。
// トランザクションの書き込みと、同期の収集・受信が同じ上限を使う。
func PayloadLimit(relative string) int64 {
	if strings.HasPrefix(relative, BackgroundsDirectory+"/") {
		return MaxAssetFileSize
	}
	return MaxFileSize
}

// TransactionFileLimit also recognizes backups by their original workspace
// path. The envelope's base64 body and header fit within twice the plain bound.
func (w *Workspace) TransactionFileLimit(path string) int64 {
	relative, err := filepath.Rel(w.Root(), filepath.Clean(path))
	if err != nil {
		return MaxFileSize
	}
	relative = filepath.ToSlash(relative)
	backup, found := strings.CutPrefix(relative, "sshc/"+BackupDirectoryName+"/")
	if found {
		identifier, original, valid := strings.Cut(backup, "/")
		if valid && validJournalIdentifier(identifier) {
			return 2 * PayloadLimit(original)
		}
	}
	return PayloadLimit(relative)
}

func (w *Workspace) ReadTransactionFile(path string) ([]byte, error) {
	return ReadFileLimited(w.FileSystem(), path, w.TransactionFileLimit(path))
}
