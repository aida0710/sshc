package storage

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"sshc/internal/platform/nativepath"
)

const (
	backupDirectoryName = "backups"
	// BackupDirectoryName は同じ名前を公開する。このパッケージの外で、ディレクトリを
	// 自分で読まなければならない唯一の呼び出し側のためである。マスターパスワードが
	// 変わったときにすべてのバックアップを暗号化し直す処理は ReadBackup を通せない。
	// ReadBackup は、暗号化したときの鍵ではなく、サービスがいま持っている鍵で開く
	// からだ。
	BackupDirectoryName = backupDirectoryName
)

// ReadBackup は世代バックアップをひとつ読み、それを開く。
//
// 読み手はすべてここを通る。下の巻き戻しと、ファイルひとつの復元を提案する履歴
// 画面である。したがって「バックアップは暗号文である」ことを知る場所はひとつだけ
// になり、それを忘れて暗号化されたままのバイト列を誰かの設定の上に書いてしまう
// 呼び出し側は存在しない。
func (m *Manager) ReadBackup(path string) ([]byte, error) {
	if !m.validBackupReadPath(path) {
		return nil, invalidJournal("backup path is outside the expected tree")
	}
	contents, err := m.workspace.ReadTransactionFile(path)
	if err != nil {
		return nil, err
	}
	if m.Unseal == nil {
		return contents, nil
	}
	plaintext, err := m.Unseal(contents)
	// 暗号化された側も同じ秘密の写しである。開いたあとまで抱えない。
	clear(contents)
	return plaintext, err
}

func (m *Manager) validBackupReadPath(path string) bool {
	if !m.validLoadedWorkspacePath(path) {
		return false
	}
	backupRoot := filepath.Join(m.workspace.StateDir(), backupDirectoryName)
	relative, ok := nativepath.RelativeBelow(backupRoot, path)
	if !ok {
		return false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	return len(parts) >= 2 && validJournalIdentifier(parts[0])
}

func (m *Manager) discardRollbackBackups(record *journalRecord) error {
	fileSystem := m.workspace.FileSystem()
	directories := map[string]bool{}
	backupRoot := filepath.Join(m.workspace.StateDir(), backupDirectoryName, record.ID)
	for index := range record.Entries {
		backup := record.Entries[index].Backup
		if backup == "" {
			continue
		}
		if err := fileSystem.Remove(backup); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := fileSystem.SyncDir(filepath.Dir(backup)); err != nil {
			return err
		}
		record.Entries[index].Backup = ""
		for directory := filepath.Dir(backup); privateStateContains(backupRoot, directory); directory = filepath.Dir(directory) {
			directories[directory] = true
			if sameJournalPath(directory, backupRoot) {
				break
			}
		}
	}
	ordered := make([]string, 0, len(directories))
	for directory := range directories {
		ordered = append(ordered, directory)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	for _, directory := range ordered {
		// 控えのファイルは上で消したので、ここでは空になったディレクトリだけを消す。
		// 消せなかったディレクトリは、エラーの種類によらず残して先へ進む。ここは
		// 完了か巻き戻しを記録に書く finish の後片付けで、失敗を返すとその記録が
		// 書かれずに止まる。中に何かが残っていても、再帰削除に広げて巻き込まない。
		_ = fileSystem.Remove(directory)
	}
	return nil
}
