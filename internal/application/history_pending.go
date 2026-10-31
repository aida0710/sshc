package application

import (
	"bytes"
	"errors"
	"strings"
	"unicode/utf8"

	"sshc/internal/secret"
	"sshc/internal/storage"
)

// maxPendingDiffBytes は、History で前と後を比べるときに中身を送る1ファイルの上限である。
// 比べる画面（Monaco の diff）と応答の JSON が、読める速さで扱える大きさにとどめる。
const maxPendingDiffBytes = 1 << 20

// 中身を見せないときの理由。画面はこの値で文言を選ぶ。
const (
	pendingContentsText        = "text"
	pendingContentsNone        = "none"
	pendingContentsSecret      = "secret"
	pendingContentsBinary      = "binary"
	pendingContentsTooLarge    = "too_large"
	pendingContentsVaultLocked = "vault_locked"
	pendingContentsUnavailable = "unavailable"
)

// revealedStateFiles は、~/.ssh/sshc の中で中身を見せてよいファイルである。ほかの状態の
// ファイル（Vault、ローカルの Vault の鍵、ごみ箱に移した秘密鍵、同期の状態など）は、
// 名前が増えても見せない側に倒すため、見せるものだけを並べる。
var revealedStateFiles = map[string]bool{
	storage.StateDirectoryName + "/metadata.json": true,
	storage.StateDirectoryName + "/snippets.json": true,
}

const revealedMachinesPrefix = storage.StateDirectoryName + "/machines/"

// PendingChanges は、中断した変更 identifier のファイルごとの変更前と変更後を、History で
// 比べる形にして返す。秘密のファイル、文字でないファイル、大きすぎるファイルは中身を
// 入れず、理由だけを入れる。
func (s *Service) PendingChanges(identifier string) (PendingChangesView, error) {
	contents, err := s.manager.PendingContents(identifier)
	if err != nil {
		return PendingChangesView{}, err
	}
	view := PendingChangesView{ID: identifier, Files: make([]PendingFileChange, 0, len(contents))}
	for _, item := range contents {
		view.Files = append(view.Files, s.pendingFileChange(item))
		clear(item.Before.Contents)
		clear(item.After.Contents)
	}
	return view, nil
}

func (s *Service) pendingFileChange(item storage.PendingContents) PendingFileChange {
	change := PendingFileChange{
		Path:         s.displayPath(item.Path),
		Action:       item.Action,
		Committed:    item.Committed,
		BeforeExists: item.Before.Exists,
		AfterExists:  item.After.Exists,
		Contents:     pendingContentsNone,
	}
	if item.Target != "" {
		change.Target = s.displayPath(item.Target)
	}
	if !item.Before.Exists && !item.After.Exists {
		return change
	}
	change.Contents = s.pendingContentsKind(item)
	if change.Contents == pendingContentsText {
		change.Before = string(item.Before.Contents)
		change.After = string(item.After.Contents)
	}
	return change
}

// pendingContentsKind は、前と後の中身を見せるか、見せないならその理由を決める。秘密かどうかを
// 先に決めるのは、読めなかった理由や大きさから、秘密のファイルの様子を伝えないためである。
func (s *Service) pendingContentsKind(item storage.PendingContents) string {
	if !s.revealsContentsOf(item.Path) || looksLikePrivateKey(item.Before.Contents) || looksLikePrivateKey(item.After.Contents) {
		return pendingContentsSecret
	}
	for _, side := range []storage.PendingSide{item.Before, item.After} {
		switch {
		case errors.Is(side.Err, secret.ErrLocked):
			return pendingContentsVaultLocked
		case side.Err != nil:
			return pendingContentsUnavailable
		}
	}
	for _, side := range []storage.PendingSide{item.Before, item.After} {
		if len(side.Contents) > maxPendingDiffBytes {
			return pendingContentsTooLarge
		}
		if !utf8.Valid(side.Contents) || bytes.IndexByte(side.Contents, 0) >= 0 {
			return pendingContentsBinary
		}
	}
	return pendingContentsText
}

// revealsContentsOf は、path の中身を History で見せてよいかを返す。~/.ssh/sshc の中は
// revealedStateFiles とマシンごとの設定だけ、外（ssh の設定、known_hosts など）は、
// 秘密鍵でなければ見せる。
func (s *Service) revealsContentsOf(path string) bool {
	relative, err := RelativePath(s.workspace.Root(), path)
	if err != nil {
		return false
	}
	if !strings.HasPrefix(relative, storage.StateDirectoryName+"/") {
		return true
	}
	if revealedStateFiles[relative] {
		return true
	}
	machine, found := strings.CutPrefix(relative, revealedMachinesPrefix)
	return found && !strings.Contains(machine, "/") && strings.HasSuffix(machine, ".json")
}

// privateKeyMarkers は、秘密鍵のファイルの先頭に現れる印である（OpenSSH・PEM・PuTTY）。
var privateKeyMarkers = [][]byte{
	[]byte("PRIVATE KEY-----"),
	[]byte("PuTTY-User-Key-File-"),
}

func looksLikePrivateKey(contents []byte) bool {
	for _, marker := range privateKeyMarkers {
		if bytes.Contains(contents, marker) {
			return true
		}
	}
	return false
}
