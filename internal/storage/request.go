package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path/filepath"
	"slices"

	"sshc/internal/platform/nativepath"
)

// Precondition は、呼び出し側が新しい内容の前提とした状態を記録する。
type Precondition struct {
	Exists bool
	Digest string
	// Mode is checked only when non-zero. It lets callers bind a write to both
	// the bytes and the owner permission bits they inspected.
	Mode fs.FileMode
}

// Change は、トランザクションが置き換えるか新規作成するファイルひとつ。
//
// SkipBackupは、この変更が置き換える内容の世代バックアップを取らない。使うのは次の
// 2つの場合だけにする。
//   - 同期の記録や鍵の交換の記録のような、このアプリケーション自身の状態ファイル。
//     書くたびに世代が増えると、バックアップディレクトリの中の雑音にしかならない。
//   - 復旧とresetの封じ直し。前の暗号文を封じた鍵が新しい鍵と違い、控えに残しても
//     開けない入れ子になる。
//
// これを選んだ変更も、ジャーナルには残り、中断後に完了させることもできるが、もはや
// 巻き戻すことはできない。そして Rollback は、できるふりをせずにその旨を述べる。
type Change struct {
	Path         string
	Contents     []byte
	Precondition Precondition
	// Mode selects the exact owner-only permission set for the new file. Zero
	// preserves the existing secure mode (or uses FilePermission for a new file).
	// Sync uses this to restore executable 0700 files without broadening access.
	Mode       fs.FileMode
	SkipBackup bool
}

// Move は、ワークスペース内でファイルひとつを rename(2) により移す。
//
// 移動はバイトをコピーしないので、秘密鍵が世代バックアップのディレクトリへ複製
// されることはない。また rename は、ファイルの既存の権限ビットをそのまま正確に
// 保つ。
type Move struct {
	From         string
	To           string
	Precondition Precondition
}

// Removal はファイルをひとつ削除する。
//
// 既定ではバックアップを書かない。最初の呼び出し側が、ユーザーが二度確認した恒久
// 削除であり、鍵素材をバックアップディレクトリへコピーすればその判断を台無しに
// してしまうからだ。そうした削除は中断後に完了させられるが、巻き戻すことはでき
// ない。そして Rollback は、できるふりをせずにその旨を
// 述べる。
//
// Backup は世代コピーを明示的に選ぶ。エクスプローラから取り除いた設定ファイルなど、
// 鍵素材ではないものを削除する呼び出し側のためである。その
// 削除は、このアプリケーションが行う他のすべての変更と同じ振る舞いになる。History
// に載り、取り消せる。
type Removal struct {
	Path         string
	Precondition Precondition
	Backup       bool
}

// DirectoryCreate は、ディレクトリひとつと、ルートより下で欠けている親を作る。
//
// ファイルの配置と、その置き場所を作ることをひとつのトランザクションにするために
// ある。ジャーナルの外で EnsureDirectory を呼ぶと、mkdir とコミットのあいだで
// クラッシュしたときに空のディレクトリが残る。
type DirectoryCreate struct {
	Path string
}

// ParentDirectoryCreates は、paths の置き場所（直接の親）を作る DirectoryCreate を、
// 重複なく返す。
//
// 直接の親だけでよい。DirectoryCreate はルートより下で欠けている親も作る。root
// そのものは作る対象ではないので含めない。同じディレクトリを二度渡すと、
// トランザクションは ErrDuplicatePath で断る。
func ParentDirectoryCreates(root string, paths []string) []DirectoryCreate {
	seen := map[string]bool{}
	var directories []DirectoryCreate
	for _, path := range paths {
		parent := filepath.Dir(path)
		identity := nativepath.Identity(parent)
		if parent == root || seen[identity] {
			continue
		}
		seen[identity] = true
		directories = append(directories, DirectoryCreate{Path: parent})
	}
	return directories
}

// DirectoryRemoval は、空のディレクトリをひとつ取り除く。
//
// 空のものだけである。再帰的な削除は、トランザクションが一度も読んでいない内容を
// 復元しない限り巻き戻せない。したがって木をまるごと消したい呼び出し側は、
// ファイルを Removal として、ディレクトリをここに、深いものから順に列挙する。
type DirectoryRemoval struct {
	Path string
}

// Request は、任意の数のファイルにまたがる論理的な編集ひとつ。
//
// この順序だけが成立しうる。変更には置き場所が要るのでディレクトリを最初に作り、
// 次に変更・移動・削除を行う。ディレクトリの削除を最後にするのは、それらを空に
// したのがこのリクエストだからである。
type Request struct {
	Operation   string
	Directories []DirectoryCreate
	Changes     []Change
	Moves       []Move
	Removals    []Removal
	// RemoveDirectories は他のすべてのあとで、深いものから順に適用され、その時点で
	// それぞれ空でなければならない。
	RemoveDirectories []DirectoryRemoval
	// FinalChanges are staged with every other write, but applied only after all
	// moves, removals, and directory removals have succeeded. Generation markers
	// such as sync-state belong here so they can never acknowledge a partially
	// applied workspace.
	FinalChanges []Change
	// Validation は、この要求に固有の検査の文脈で、Manager.Validate へそのまま渡す。
	// storage は中身を見ない。Manager は複数のサービスが共有するので、文脈を
	// 要求の外（サービスのフィールドなど）に置くと、別の goroutine の要求の
	// 検査がそれを読んでしまう。
	Validation any
}

// hasChangeWithoutBackup は、置き換える内容の控えを取らない変更（SkipBackup）を
// 含むかを報告する。控えの無い変更は、失敗したときに巻き戻せない。
func (r Request) hasChangeWithoutBackup() bool {
	skipsBackup := func(change Change) bool { return change.SkipBackup }
	return slices.ContainsFunc(r.Changes, skipsBackup) || slices.ContainsFunc(r.FinalChanges, skipsBackup)
}

// Result は、完了したトランザクションを記述する。
type Result struct {
	ID        string
	BackupDir string
	Written   []string
}

// ConflictError は、ディスク上のファイルが呼び出し側の編集したファイルではないと
// 報告する。Current はディスク上の内容を運ぶので、呼び出し側は三方向の差分を作れる。
// Error がファイルの内容を含むことは決してない。
type ConflictError struct {
	Path     string
	Expected string
	Actual   string
	Current  []byte
}

func (e *ConflictError) Error() string {
	return "external change detected for " + e.Path
}

// Digest は、事前条件とジャーナルエントリに使う内容ハッシュ。
func Digest(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}
