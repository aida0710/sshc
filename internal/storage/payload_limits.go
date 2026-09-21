package storage

import (
	"path/filepath"
	"strings"
)

// MaxAssetFileSize is the supported background asset size. Config loaders keep
// MaxFileSize; only transaction payloads and explicit asset readers use this.
const MaxAssetFileSize = 1024 << 20

func payloadLimit(relative string) int64 {
	if strings.HasPrefix(relative, "sshc/backgrounds/") {
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
			return 2 * payloadLimit(original)
		}
	}
	return payloadLimit(relative)
}

func (w *Workspace) ReadTransactionFile(path string) ([]byte, error) {
	return ReadFileLimited(w.FileSystem(), path, w.TransactionFileLimit(path))
}
