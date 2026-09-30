package storage

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// commitBuilder は、ひとつのコミットを組み立てる途中の状態である。
//
// フェーズをまたいで持ち回るものを、名前のある値にまとめてある。以前これらは
// commit の 380 行の中に生の変数として並んでおり、どのフェーズが何を触るのかは、
// 全体を頭に入れないと分からなかった。
type commitBuilder struct {
	manager *Manager
	request Request
	plan    *journalPlan
	// written は、このコミットが触ったと呼び出し側へ報告する表記である。
	written []string
	// claimed は、すでに扱った表記である。同じ表記を二度含むリクエストは、
	// 順序で結果が変わるので受け付けない。
	claimed claimedPaths
	// planned は、このリクエストが作るディレクトリと、ルートより下にあるその祖先で
	// ある。まだディスクに無い場所への書き込みを解決できるのは、これがあるからである。
	planned map[string]bool
}

// planCommit は、request を journal に記録する計画へ落とす。written は、この commit が
// 触ったと呼び出し側へ報告する表記である。ファイルには何も書かない。
func (m *Manager) planCommit(request Request) (*journalPlan, []string, error) {
	capacity := len(request.Changes) + len(request.FinalChanges) + len(request.Moves) + len(request.Removals) +
		len(request.Directories) + len(request.RemoveDirectories)
	// 1 つの依頼は多くとも 1 エントリになる。ファイルを読む前に断る。
	if capacity > maxTransactionEntries {
		return nil, nil, ErrTransactionTooLarge
	}
	builder := &commitBuilder{
		manager: m, request: request,
		plan:    newJournalPlan(capacity),
		written: make([]string, 0, capacity),
		claimed: newClaimedPaths(capacity * 2),
		planned: map[string]bool{},
	}
	for _, create := range request.Directories {
		cleaned, err := m.workspace.ResolveDirectory(create.Path)
		if err != nil {
			return nil, nil, err
		}
		for current := cleaned; m.workspace.Contains(current) && current != m.workspace.Root(); current = filepath.Dir(current) {
			builder.planned[current] = true
		}
	}
	if err := builder.stage(); err != nil {
		return nil, nil, err
	}
	return builder.plan, builder.written, nil
}

// claim は、この表記を扱うのが初めてであることを確かめて台帳に載せる。
func (b *commitBuilder) claim(path string) error {
	if !b.claimed.claim(path) {
		return ErrDuplicatePath
	}
	return nil
}

// stage は、リクエストの全体を計画へ落とす。並びに意味がある。
func (b *commitBuilder) stage() error {
	for _, phase := range []func() error{
		b.stageDirectories, b.stageChanges, b.stageMoves,
		b.stageRemovals, b.stageDirectoryRemovals, b.stageFinalChanges,
	} {
		if err := phase(); err != nil {
			return err
		}
	}
	return nil
}

// stageDirectories は、ディレクトリを作る計画を立てる。
//
// これが先である。変更には置き場所が要り、移動には存在する行き先が要る。
func (b *commitBuilder) stageDirectories() error {
	// ディレクトリが先。変更には置き場所が要り、移動には存在する
	// 行き先が要る。
	for _, create := range b.request.Directories {
		target, err := b.manager.workspace.ResolveDirectory(create.Path)
		if err != nil {
			return err
		}
		if err := b.claim(target); err != nil {
			return err
		}
		// すでにそこにあるかどうかが、巻き戻しの内容を決める。このトランザクションが
		// 作っていないディレクトリを取り除けば、誰も触れてくれと頼んでいないものを
		// 削除することになる。
		existed := false
		if _, statErr := b.manager.workspace.FileSystem().Lstat(target); statErr == nil {
			existed = true
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return statErr
		}
		b.plan.add(journalEntry{
			Action:      actionMakeDir,
			Path:        target,
			HadPrevious: existed,
			Mode:        uint32(DirectoryPermission),
		}, nil, nil)
	}
	return nil
}

// stageChanges は、ファイルの置き換えを計画する。前提条件が合わなければ、ここで衝突を返す。
func (b *commitBuilder) stageChanges() error {
	return b.stageChangeSet(b.request.Changes)
}

// stageFinalChanges uses the same validation and staging contract as an
// ordinary write. Its position in stage() is the only difference and is the
// durable guarantee: recovery replays the recorded entry order unchanged.
func (b *commitBuilder) stageFinalChanges() error {
	return b.stageChangeSet(b.request.FinalChanges)
}

func (b *commitBuilder) stageChangeSet(changes []Change) error {
	for _, change := range changes {
		target, err := b.manager.workspace.ResolveForWriteUnder(change.Path, b.planned)
		if err != nil {
			return err
		}
		if err := b.claim(target); err != nil {
			return err
		}

		if int64(len(change.Contents)) > b.manager.workspace.TransactionFileLimit(target) {
			return ErrFileTooLarge
		}
		previous, mode, exists, err := b.manager.currentState(target)
		if err != nil {
			return err
		}
		actual := ""
		expected := ""
		if exists {
			actual = Digest(previous)
		}
		if change.Precondition.Exists {
			expected = change.Precondition.Digest
		}
		if actual != expected {
			return &ConflictError{Path: target, Expected: expected, Actual: actual, Current: previous}
		}
		if change.Precondition.Mode != 0 && (!exists || !b.manager.ownerModesMatch(mode, change.Precondition.Mode)) {
			return &ConflictError{Path: target, Expected: expected, Actual: actual, Current: previous}
		}
		targetMode := mode
		if !exists {
			targetMode = FilePermission
		}
		if change.Mode != 0 {
			if change.Mode != FilePermission && change.Mode != DirectoryPermission {
				return invalidJournal("invalid write mode")
			}
			targetMode = change.Mode
		}

		entry := journalEntry{
			Action:      actionWrite,
			Path:        target,
			NoBackup:    change.SkipBackup,
			HadPrevious: exists,
			Mode:        uint32(targetMode),
			Digest:      Digest(change.Contents),
		}
		if exists {
			entry.PreviousDigest = actual
			entry.PreviousMode = uint32(mode)
		}
		b.plan.add(entry, change.Contents, previous)
		b.written = append(b.written, target)
	}
	return nil
}

// stageMoves は、ファイルの移動を計画する。行き先に何かあれば断る。
func (b *commitBuilder) stageMoves() error {
	for _, move := range b.request.Moves {
		source, err := b.manager.workspace.ResolveForWrite(move.From)
		if err != nil {
			return err
		}
		target, err := b.manager.workspace.ResolveForWriteUnder(move.To, b.planned)
		if err != nil {
			return err
		}
		if err := b.claim(source); err != nil {
			return err
		}
		if err := b.claim(target); err != nil {
			return err
		}
		if _, statErr := b.manager.workspace.FileSystem().Lstat(target); statErr == nil {
			return ErrMoveTargetExists
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return statErr
		}

		digest, mode, err := b.manager.sourceState(source, move.Precondition)
		if err != nil {
			return err
		}
		b.plan.add(journalEntry{
			Action:         actionMove,
			Path:           source,
			Target:         target,
			HadPrevious:    true,
			Mode:           uint32(mode),
			Digest:         digest,
			PreviousDigest: digest,
		}, nil, nil)
		b.written = append(b.written, target)
	}
	return nil
}

// stageRemovals は、ファイルの削除を計画する。バックアップを取るかは呼び出し側が決める。
func (b *commitBuilder) stageRemovals() error {
	for _, removal := range b.request.Removals {
		target, err := b.manager.workspace.ResolveForWrite(removal.Path)
		if err != nil {
			return err
		}
		if err := b.claim(target); err != nil {
			return err
		}
		digest, mode, err := b.manager.sourceState(target, removal.Precondition)
		if err != nil {
			return err
		}
		var previous []byte
		if removal.Backup {
			if previous, err = b.manager.workspace.ReadTransactionFile(target); err != nil {
				return err
			}
		}
		b.plan.add(journalEntry{
			Action:         actionRemove,
			Path:           target,
			NoBackup:       !removal.Backup,
			HadPrevious:    true,
			Mode:           uint32(mode),
			Digest:         digest,
			PreviousDigest: digest,
		}, nil, previous)
		b.written = append(b.written, target)
	}
	return nil
}

// stageDirectoryRemovals は、ディレクトリの削除を計画する。
//
// 最後である。実行される時点でそれぞれ空でなければならず、その検査は
// 「このリクエストが残すディスクの状態」に対して行う。
func (b *commitBuilder) stageDirectoryRemovals() error {
	// ディレクトリの削除は最後で、実行される時点でそれぞれ空でなければならない。
	// 検査は、このリクエストが残すことになるディスクの状態に対して行う。この同じ
	// リクエストが移動させ、削除し、あるいはディレクトリとして取り除くエントリは、
	// その親を生かし続けない。現状のディスクに対して検査すると、呼び出し側は一方の
	// トランザクションで木を空にし、次のトランザクションで取り除くしかなくなる。
	// 二つのあいだでクラッシュすれば、空の抜け殻が残る。
	//
	// 深いものから順が唯一成立する順序であり、ここで整列しておくことが、呼び出し側に
	// それを知らせずに済ませている。
	ordered := append([]DirectoryRemoval(nil), b.request.RemoveDirectories...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return strings.Count(ordered[i].Path, string(filepath.Separator)) >
			strings.Count(ordered[j].Path, string(filepath.Separator))
	})
	for _, removal := range ordered {
		target, err := b.manager.workspace.ResolveDirectory(removal.Path)
		if err != nil {
			return err
		}
		if err := b.claim(target); err != nil {
			return err
		}
		info, statErr := b.manager.workspace.FileSystem().Lstat(target)
		if errors.Is(statErr, fs.ErrNotExist) {
			// 何もすることがない。エラーでもない。すでに消えているディレクトリを
			// 取り除くことは、呼び出し側が求めた状態である。
			continue
		}
		if statErr != nil {
			return statErr
		}
		if !info.IsDir() {
			return ErrNotDirectory
		}
		contents, err := b.manager.workspace.FileSystem().ReadDir(target)
		if err != nil {
			return err
		}
		for _, entry := range contents {
			// claimed は、このリクエストがすでに責任を引き受けたすべてのパスを
			// 保持する。移動の元、削除、そしてこれより深いところに列挙された
			// ディレクトリの削除である。
			if !b.claimed.contains(filepath.Join(target, entry.Name())) {
				return ErrDirectoryNotEmpty
			}
		}
		b.plan.add(journalEntry{
			Action:      actionRemoveDir,
			Path:        target,
			HadPrevious: true,
			Mode:        uint32(info.Mode().Perm()),
		}, nil, nil)
	}
	return nil
}
