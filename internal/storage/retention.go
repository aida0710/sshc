package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	// RetainedHistoryRecords は、完了した変更の記録（history/<id>.json）を、新しい方から
	// 何件まで残すか。記録の控え（backups/<id>/）も同じ記録の分だけ残る。控えのファイルの
	// 数は、これとは別に RetainedBackupFiles で抑える。
	RetainedHistoryRecords = 200
	// RetainedBackupFiles は、完了した変更の控え（backups/<id>/ の中のファイル）を、新しい
	// 記録から数えて合わせて何件まで残すか。
	//
	// マスターパスワードの変更と、未対応の Vault の復旧・リセットは、残した控えをすべて
	// 1 回の変更で暗号化し直す。1 件の記録が持つ控えの数に上限は無く、同期の受信やフォルダ
	// の削除のように 1 回で多くのファイルを書く記録もあるので、記録の件数だけでは抑え
	// られない。SATA SSD の ext4 での実測（8192 件で約 7 分、1 件あたり約 50 ミリ秒）から
	// 見積もると、512 件を暗号化し直すのは約 26 秒で、そのあいだ待たされるほかの保存の
	// 待ちの上限（mutationLockWait の 30 秒）に収まる。1 回の変更の上限
	// （maxTransactionEntries の 8192 件）にも届かない。ふだんの保存が残す控えは 1 回に
	// 2 件ほどなので、ふだんは記録の件数の上限が先に効く。
	//
	// いちばん新しい記録だけは、控えがこれを超えていても残す（pruneHistory）。
	RetainedBackupFiles = 512
	// HistoryRetentionPeriod は、完了した変更の記録と控えを残す期間。
	//
	// 控えは、Vault から消したパスワードや変更前の秘密鍵を、暗号化したまま持ち続ける。
	// 誤った変更に気付いて戻すには足り、消したシークレットを控えに残し続けない長さと
	// して 90 日にした。
	HistoryRetentionPeriod = 90 * 24 * time.Hour
)

// PruneHistory は、保持の上限（RetainedHistoryRecords、RetainedBackupFiles、
// HistoryRetentionPeriod）を超えた完了した変更の記録と、その控えを消す。
//
// 変更が完了したときにも同じ規則で消すので、これを呼ぶのは sshcエンジンの起動時
// だけでよい。長く変更の無かったワークスペースでも、期間を過ぎた控えを次の変更まで
// 残さない。
func (m *Manager) PruneHistory() error {
	unlock, err := m.workspace.lockMutation()
	if err != nil {
		return err
	}
	defer unlock()
	return m.pruneHistory("")
}

// pruneHistory は PruneHistory の本体で、workspace の変更のロックを持って呼ぶ。
// justFinished は、この片付けの直前に完了か取り消しで片付けた変更の ID（起動時は空）。
//
// 消すのは完了した記録と、その控えだけである。中断した変更の記録（journal/<id>.json）
// とその控えは、完了か取り消しで片付くまで数えも消しもしない。記録の中身は読まず、
// ID の先頭の時刻で新しさと古さを決める。バージョンの違う記録や、記録を失った控えも
// 同じ規則で消す。
//
// 新しい記録から順に見て、次のどれかに当たる記録を消す。一度どれかに当たると、それより
// 古い記録もすべて当たる。
//   - 新しい方から RetainedHistoryRecords 件より後ろにある。
//   - 変更を始めた時刻が HistoryRetentionPeriod より前。
//   - それより新しい記録と合わせた控えのファイルが RetainedBackupFiles 件を超える。ただし
//     いちばん新しい記録は、超えていても残す。フォルダの削除のように 1 回で多くのファイルを
//     消した直後に、その変更を戻せなくしないためである。
//
// justFinished の記録は、始めた時刻が古くてもこの回は消さない。90 日より前に始めて中断
// していた変更を完了させたとき、同じ完了の中で記録と控えを消して、すぐに戻せなくする
// ことを避ける。その記録も、次の変更の完了か sshcエンジンの起動で同じ規則で消える。
//
// 消せなかった記録は飛ばして次へ進む。一覧は新しい順なので、そこで止まると、それより
// 古い記録が毎回残り続ける。失敗はまとめて返す。
func (m *Manager) pruneHistory(justFinished string) error {
	finished, err := m.finishedRecordIdentifiers()
	if err != nil {
		return err
	}
	oldest := m.now().UTC().Add(-HistoryRetentionPeriod)
	var failures []error
	backupFiles := 0
	for index, identifier := range finished {
		// 上限を超えたあとの記録はどれも消すので、控えを数えるのは超えるまででよい。
		if backupFiles <= RetainedBackupFiles {
			count, countErr := m.backupFileCount(identifier)
			if countErr != nil {
				failures = append(failures, fmt.Errorf("count the backups of %s: %w", identifier, countErr))
			}
			backupFiles += count
		}
		tooMany := index >= RetainedHistoryRecords
		tooOld := startedBefore(identifier, oldest)
		tooManyBackups := index > 0 && backupFiles > RetainedBackupFiles
		if identifier == justFinished || !(tooMany || tooOld || tooManyBackups) {
			continue
		}
		if err := m.removeFinishedRecord(identifier); err != nil {
			failures = append(failures, fmt.Errorf("remove the change record %s: %w", identifier, err))
		}
	}
	return errors.Join(failures...)
}

// finishedRecordIdentifiers は、完了した記録と控えの ID を、新しいものから順に返す。
// 中断した変更の ID は含めない。
func (m *Manager) finishedRecordIdentifiers() ([]string, error) {
	pending, err := m.recordFileIdentifiers(m.journalDirectory())
	if err != nil {
		return nil, err
	}
	history, err := m.recordFileIdentifiers(m.historyDirectory())
	if err != nil {
		return nil, err
	}
	backups, err := m.backupDirectoryIdentifiers()
	if err != nil {
		return nil, err
	}
	// counted は、数えない ID（中断した変更）と、数え終えた ID。記録と控えの両方が
	// ある変更を 2 回数えない。
	counted := make(map[string]bool, len(pending)+len(history))
	for _, identifier := range pending {
		counted[identifier] = true
	}
	finished := make([]string, 0, len(history))
	for _, identifier := range append(history, backups...) {
		if !counted[identifier] {
			counted[identifier] = true
			finished = append(finished, identifier)
		}
	}
	// ID は変更を始めた時刻（UTC）で始まるので、辞書順の逆が新しい順になる。
	slices.Sort(finished)
	slices.Reverse(finished)
	return finished, nil
}

// recordFileIdentifiers は、journal か history のディレクトリにある記録の ID を返す。
// 記録の名前でないファイルは、sshc の書いたものではないので数えない。
func (m *Manager) recordFileIdentifiers(directory string) ([]string, error) {
	entries, err := m.workspace.FileSystem().ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	identifiers := make([]string, 0, len(entries))
	for _, entry := range entries {
		identifier, isRecord := strings.CutSuffix(entry.Name(), ".json")
		if isRecord && !entry.IsDir() && validJournalIdentifier(identifier) {
			identifiers = append(identifiers, identifier)
		}
	}
	return identifiers, nil
}

// backupDirectoryIdentifiers は、控えのディレクトリ（backups/<id>/）の ID を返す。
func (m *Manager) backupDirectoryIdentifiers() ([]string, error) {
	entries, err := m.workspace.FileSystem().ReadDir(m.backupsDirectory())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	identifiers := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && validJournalIdentifier(entry.Name()) {
			identifiers = append(identifiers, entry.Name())
		}
	}
	return identifiers, nil
}

func (m *Manager) backupsDirectory() string {
	return filepath.Join(m.workspace.StateDir(), backupDirectoryName)
}

// startedBefore は、ID の変更を始めた時刻が moment より前かを返す。
func startedBefore(identifier string, moment time.Time) bool {
	started, err := time.Parse(journalIdentifierTimeLayout, identifier[:len(journalIdentifierTimeLayout)])
	return err == nil && started.Before(moment)
}

// removeFinishedRecord は、完了した記録ひとつと、その控えを消す。
//
// 記録を先に消す。控えを消している途中で止まっても、履歴の画面が一部の消えた控えを
// 復元の候補に出さない。残った控えは古さも順位も変わらないので、次の片付けが消す。
// 消したことは fsync しない。消えなかったものは、次の片付けがもう一度消す。
func (m *Manager) removeFinishedRecord(identifier string) error {
	record := filepath.Join(m.historyDirectory(), identifier+".json")
	if err := m.workspace.FileSystem().Remove(record); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return m.removeBackupTree(filepath.Join(m.backupsDirectory(), identifier))
}

// backupFileCount は、記録の控え（backups/<id>/）にあるファイルの数を返す。ディレクトリ
// でないものをすべて数えるので、暗号化し直す処理が飛ばす一時ファイルの分だけ多めになる。
func (m *Manager) backupFileCount(identifier string) (int, error) {
	count := 0
	err := m.visitBackupTree(filepath.Join(m.backupsDirectory(), identifier), func(_ string, directory bool) error {
		if !directory {
			count++
		}
		return nil
	})
	return count, err
}

// removeBackupTree は、控えのディレクトリを中身ごと消す。
func (m *Manager) removeBackupTree(directory string) error {
	return m.visitBackupTree(directory, func(path string, _ bool) error {
		if err := m.workspace.FileSystem().Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	})
}

// visitBackupTree は、path とその下にあるものを visit へ渡す。ディレクトリは中身を
// 渡してから、それ自身を渡す。directory は、渡したものが本物のディレクトリかどうか。
//
// 入るのは、Lstat で本物のディレクトリと確かめたものだけである。シンボリックリンクなどは
// たどらず、それ自身を渡す。backups/<id> そのものがリンクでも、リンク先には触れない。
func (m *Manager) visitBackupTree(path string, visit func(path string, directory bool) error) error {
	fileSystem := m.workspace.FileSystem()
	info, err := fileSystem.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	directory := info.Mode().Type() == fs.ModeDir
	if directory {
		entries, err := fileSystem.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := m.visitBackupTree(filepath.Join(path, entry.Name()), visit); err != nil {
				return err
			}
		}
	}
	return visit(path, directory)
}
