package storage

import (
	"errors"
	"io/fs"
	"path/filepath"
)

// commitRun は、journal に記録した commit ひとつを、段ごと（ディレクトリの作成、
// 控え、ステージ、staged の記録、置き換え、完了）に進める。
//
// どの段の失敗も fail を通す。失敗したときに記録を残すか巻き戻すかは mode だけで
// 決まり、段ごとに扱いを変えない。
type commitRun struct {
	manager *Manager
	mode    commitMode
	plan    *journalPlan
	// record は、journal に書いた記録と、このプロセスが知っている進捗である。
	// record.Entries の添字は plan の添字である。ジャーナルへ書いたのは plan.entries
	// そのものなので、以前の内容もステージする内容も、同じ添字で引ける。
	record          journalRecord
	journalPath     string
	backupDirectory string
	result          Result
}

// begin は、この commit の記録を staging の状態で journal に書く。ここより前の失敗は
// 何も残さずに返り、ここから先の失敗は fail が扱う。
func (run *commitRun) begin(operation string, written []string) error {
	m := run.manager
	identifier, err := m.newIdentifier()
	if err != nil {
		return err
	}
	// backups/<id> はここでは作らない。最初の控えを書くときに作る。控えを書かない
	// トランザクション（SkipBackup だけのもの、新しいファイルだけのもの、移動だけの
	// もの）のたびに、空のディレクトリを残さないためである。
	run.backupDirectory = filepath.Join(m.workspace.StateDir(), backupDirectoryName, identifier)
	for _, directory := range []string{m.journalDirectory(), m.historyDirectory()} {
		if err := m.workspace.EnsureDirectory(directory); err != nil {
			return err
		}
	}

	run.record = journalRecord{
		ID:             identifier,
		Version:        journalVersion,
		Operation:      operation,
		Status:         statusStaging,
		StartedAt:      m.now().UTC(),
		Entries:        run.plan.entries,
		Atomic:         run.mode.rollbackOnError,
		DiscardBackups: run.mode.discardBackups,
	}
	run.journalPath = filepath.Join(m.journalDirectory(), identifier+".json")
	if err := m.writeRecord(run.journalPath, run.record); err != nil {
		return err
	}
	run.result = Result{ID: identifier, BackupDir: run.backupDirectory, Written: written}
	return nil
}

// apply は、記録した commit を最後まで進める。
func (run *commitRun) apply() (Result, error) {
	for _, stage := range []func() error{
		run.makeDirectories, run.writeBackups, run.stageWrites, run.markStaged, run.commitStaged,
	} {
		if err := stage(); err != nil {
			return run.fail(err)
		}
	}
	if run.mode.discardBackups {
		return run.finishDiscardingBackups()
	}
	if err := run.manager.finish(&run.record, run.journalPath, statusCompleted); err != nil {
		return run.fail(err)
	}
	return run.result, nil
}

// makeDirectories は、記録したディレクトリを作る。
//
// ディレクトリはここで作る。バリデータがリクエストを受理したあとなので、拒否
// されたリクエストは何も作らない。そして一時ファイルがステージされる前なので、
// ステージされるファイルには親が存在する。これらはジャーナルのエントリなので、
// 中断されたコミットは巻き戻せる。これがトランザクションの外の EnsureDirectory で
// ない理由のすべてである。
func (run *commitRun) makeDirectories() error {
	for index := range run.record.Entries {
		entry := run.record.Entries[index]
		if entry.Action != actionMakeDir {
			continue
		}
		if err := run.manager.workspace.EnsureDirectory(entry.Path); err != nil {
			return err
		}
		run.record.Committed = index + 1
	}
	return nil
}

// writeBackups は、何かが置き換えられたり unlink されたりする前に、以前の内容を
// コピーする。移動にコピーは不要だ。ファイルの唯一のコピーをそのまま保つからである。
// 置き換えには常に必要で、削除には呼び出し側が求めたときにちょうど必要になる。
func (run *commitRun) writeBackups() error {
	m := run.manager
	for index := range run.record.Entries {
		entry := &run.record.Entries[index]
		if entry.Action != actionWrite && entry.Action != actionRemove {
			continue
		}
		if !entry.HadPrevious || entry.NoBackup {
			continue
		}
		relative, err := filepath.Rel(m.workspace.Root(), entry.Path)
		if err != nil {
			return err
		}
		backupPath := filepath.Join(run.backupDirectory, relative)
		if err := m.workspace.EnsureDirectory(filepath.Dir(backupPath)); err != nil {
			return err
		}
		contents := run.plan.previous[index]
		if m.Seal != nil && !run.mode.discardBackups {
			sealed, err := m.Seal(contents)
			if err != nil {
				return err
			}
			contents = sealed
		}
		backupMode := entry.Mode
		if entry.Action == actionWrite {
			backupMode = entry.PreviousMode
		}
		if err := m.writeFile(backupPath, contents, fs.FileMode(backupMode)); err != nil {
			return err
		}
		entry.Backup = backupPath
	}
	return nil
}

// stageWrites は、新しいファイルをすべて対象の隣にステージし、あとの rename が
// 原子的になるようにする。
func (run *commitRun) stageWrites() error {
	fileSystem := run.manager.workspace.FileSystem()
	for index := range run.record.Entries {
		entry := &run.record.Entries[index]
		if entry.Action != actionWrite {
			continue
		}
		temporaryPath, err := fileSystem.WriteTemp(
			filepath.Dir(entry.Path),
			temporaryPrefix+run.record.ID+"-",
			fs.FileMode(entry.Mode),
			run.plan.staged[index],
		)
		if err != nil {
			return err
		}
		entry.Temp = temporaryPath
	}
	return nil
}

// markStaged は、ステージを終えたことを journal に書く。
func (run *commitRun) markStaged() error {
	run.record.Status = statusStaged
	if err := run.manager.writeRecord(run.journalPath, run.record); err != nil {
		// staged の記録を書けなかった。非 atomic の記録はディスクでは staging の
		// ままなので、一時ファイルの名前を持たない記録として片付ける。atomic の
		// 記録は、進捗を書き直してから巻き戻す fail の経路が一時ファイルも消す。
		if !run.mode.rollbackOnError {
			run.record.Status = statusStaging
		}
		return err
	}
	return nil
}

// commitStaged は、ステージ済みのエントリを対象へ置き換える。
func (run *commitRun) commitStaged() error {
	return run.manager.commitStaged(&run.record)
}

// finishDiscardingBackups は、以前の内容を中断したときの巻き戻しのためだけに残した
// commit を、commit point の記録を書いてから片付ける。
func (run *commitRun) finishDiscardingBackups() (Result, error) {
	// This durable marker is the commit point. Before it, recovery rolls all
	// targets back. Once it exists, every target is already the new generation,
	// so recovery only finishes deleting rollback material.
	run.record.Status = statusApplied
	if err := run.manager.writeRecord(run.journalPath, run.record); err != nil {
		return run.fail(err)
	}
	if run.mode.publish != nil {
		run.mode.publish()
	}
	// Cleanup is idempotent. A failure here leaves the applied marker for a
	// later Complete call, but cannot make the committed targets mixed again.
	_ = run.manager.finishApplied(&run.record, run.journalPath)
	return run.result, nil
}

// fail は、記録したあとの失敗を mode に従って扱う。非 atomic なら記録を残して利用者に
// 完了か復元を選ばせ、atomic ならこのプロセスが適用した分を巻き戻す。
func (run *commitRun) fail(commitErr error) (Result, error) {
	m, record := run.manager, &run.record
	if !run.mode.rollbackOnError && record.Status == statusStaging {
		// 永続化した staging の記録は一時ファイルの名前を持たない。ここで
		// 消さなければ、Rollback も Complete も見つけられない一時ファイルが
		// 対象の隣に残り、鍵の一覧や Include の glob に紛れ込む。
		if cleanupErr := m.removeStagedTemps(record); cleanupErr != nil {
			commitErr = errors.Join(commitErr, cleanupErr)
		}
		return run.result, commitErr
	}
	if !run.mode.rollbackOnError {
		// 対象の変更は、それを記録するジャーナルの書き換えより先に起きる。
		// したがって永続化された記録はファイルシステムより遅れうる。この
		// プロセスが知っている進捗をここで残す。この書き込み自体が失敗した
		// 場合は、復旧が対象の状態から照合し直す。
		if record.Status == statusStaged {
			if progressErr := m.writeRecord(run.journalPath, *record); progressErr != nil {
				commitErr = errors.Join(commitErr, progressErr)
			}
		}
		return run.result, commitErr
	}
	// finish は履歴の公開前にメモリ上の記録を変更する。履歴の公開または current
	// journal の削除に失敗した場合は、終端状態ではなく復旧可能な current 状態を保存する。
	if record.Status == statusCompleted || record.Status == statusRolledBack || record.Status == statusApplied {
		record.Status = statusStaged
		record.FinishedAt = nil
	}
	// ロールバックを試す前にプロセス内の進捗を保存する。対象の rename 後に SyncDir
	// または journal の再書き込みが失敗する場合があるため、再保存しないと
	// ロールバック失敗後の永続記録がファイルシステムより遅れた状態になる。
	if progressErr := m.writeRecord(run.journalPath, *record); progressErr != nil {
		commitErr = errors.Join(commitErr, progressErr)
	}
	if rollbackErr := m.rollbackRecord(record, run.journalPath); rollbackErr != nil {
		return run.result, errors.Join(commitErr, rollbackErr)
	}
	return Result{}, commitErr
}
