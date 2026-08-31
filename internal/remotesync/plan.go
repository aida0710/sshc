package remotesync

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"

	"sshc/internal/storage"
)

// ErrNothingToApply は、リモートのスナップショットがすでにこのディスクと一致して
// いることを報告する。これは失敗ではなく結果である。
var ErrNothingToApply = errors.New("this workspace already matches the snapshot")

// Resolution は、競合したファイルでローカルとリモートのどちらを採用するかを表す。
// 既定では選択せず、競合として報告する。
type Resolution string

const (
	// ResolveNone は、衝突を報告して止まる。
	ResolveNone Resolution = ""
	// ResolveLocal はローカルの内容を維持する。
	ResolveLocal Resolution = "local"
	// ResolveRemote はリモートの内容で置き換え、以前の内容を History に残す。
	ResolveRemote Resolution = "remote"
)

// Conflict は、前回の同期以降に両側で変わったファイルひとつ。
//
// ダイジェストを運び、内容は決して運ばない。三方向のビューは、呼び出し側が自分で
// 読めるファイルから組み立てる。秘密鍵のバイト列を運ぶ衝突レコードは、レスポンス
// 本文の中にあるその鍵のコピーになってしまう。
type Conflict struct {
	Path         string
	BaseDigest   string
	LocalDigest  string
	RemoteDigest string
	BaseMode     string
	LocalMode    string
	RemoteMode   string
}

// LocalEntry is the exact local state used by a three-way pull plan. Mode is
// part of the synchronized state: equal bytes with different permissions are
// still a change.
type LocalEntry struct {
	SHA256       string
	Mode         string
	ObservedMode fs.FileMode
	ModeObserved bool
}

func entryState(digest, mode string) LocalEntry {
	// Unit-level callers which predate mode-aware planning omit Mode. Production
	// manifests have already passed snapshot validation and always carry it.
	if mode == "" {
		mode = "0600"
	}
	return LocalEntry{SHA256: digest, Mode: mode}
}

func synchronizedEqual(left, right LocalEntry) bool {
	return left.SHA256 == right.SHA256 && left.Mode == right.Mode
}

func preconditionMode(entry LocalEntry) fs.FileMode {
	if entry.ModeObserved {
		return entry.ObservedMode
	}
	return modeBits(entry.Mode)
}

// Plan は、復号したスナップショットと現在のワークスペースから、このマシンをそれに
// 一致させるトランザクション、または衝突を導き出す。
//
// pull 全体を 1 つの storage.Request にし、再解析、事前条件、ジャーナル、
// 世代バックアップを通常の書き込みと同じ境界で適用する。
//
//   - base は、このマシンが最後に同期したスナップショットのマニフェスト。base が
//     nil なら、このマシンは一度も同期していないので、何も削除とは呼べず、
//     ファイルはひとつも取り除かれない。
//   - local は、ワークスペース相対のパスを、いまこのディスク上のダイジェストへ対応付ける。
//   - remote は、いま取得したマニフェストで、contents はそのファイル群。
func Plan(root string, base *Manifest, local map[string]string, remote Manifest, contents map[string][]byte, resolve Resolution) (storage.Request, []Conflict, error) {
	return PlanWithIgnore(root, base, local, remote, contents, resolve, nil)
}

// PlanWithIgnore keeps paths selected by the shared exclusion rules outside
// both writes and removals. The local copy of an excluded path is never touched.
func PlanWithIgnore(root string, base *Manifest, local map[string]string, remote Manifest, contents map[string][]byte, resolve Resolution, ignored func(string) bool) (storage.Request, []Conflict, error) {
	entries := make(map[string]LocalEntry, len(local))
	for path, digest := range local {
		entries[path] = entryState(digest, "0600")
	}
	return PlanEntriesWithIgnore(root, base, entries, remote, contents, resolve, ignored)
}

// PlanEntriesWithIgnore is the mode-aware planner used by the sync service.
func PlanEntriesWithIgnore(root string, base *Manifest, local map[string]LocalEntry, remote Manifest, contents map[string][]byte, resolve Resolution, ignored func(string) bool) (storage.Request, []Conflict, error) {
	isIgnored := func(path string) bool { return ignored != nil && ignored(path) }
	baseEntries := map[string]LocalEntry{}
	if base != nil {
		for _, item := range base.Files {
			if isIgnored(item.Path) {
				continue
			}
			baseEntries[item.Path] = entryState(item.SHA256, item.Mode)
		}
	}
	remoteEntries := map[string]LocalEntry{}
	for _, item := range remote.Files {
		if isIgnored(item.Path) {
			continue
		}
		remoteEntries[item.Path] = entryState(item.SHA256, item.Mode)
	}

	request := storage.Request{Operation: "sync.pull"}
	var conflicts []Conflict

	for _, item := range remote.Files {
		if isIgnored(item.Path) {
			continue
		}
		localEntry, present := local[item.Path]
		remoteEntry := entryState(item.SHA256, item.Mode)
		if present && synchronizedEqual(localEntry, remoteEntry) {
			continue
		}
		baseEntry, hadBase := baseEntries[item.Path]
		contested := present && (!hadBase || !synchronizedEqual(localEntry, baseEntry))
		if contested && resolve == ResolveNone {
			// 両側で変更された内容は自動マージせず、digest だけを競合として報告する。
			conflict := Conflict{
				Path: item.Path, LocalDigest: localEntry.SHA256, RemoteDigest: item.SHA256,
				LocalMode: localEntry.Mode, RemoteMode: item.Mode,
			}
			if hadBase {
				conflict.BaseDigest = baseEntry.SHA256
				conflict.BaseMode = baseEntry.Mode
			}
			conflicts = append(conflicts, conflict)
			continue
		}
		if contested && resolve == ResolveLocal {
			// ローカルを採用する場合は変更を追加しない。
			continue
		}
		precondition := storage.Precondition{}
		if present {
			precondition = storage.Precondition{Exists: true, Digest: localEntry.SHA256, Mode: preconditionMode(localEntry)}
		}
		request.Changes = append(request.Changes, storage.Change{
			Path:         filepath.Join(root, filepath.FromSlash(item.Path)),
			Contents:     contents[item.Path],
			Precondition: precondition,
			Mode:         modeBits(item.Mode),
			// 秘密鍵を含め、置き換えるファイルは暗号化された世代バックアップへ残す。
		})
	}

	// ローカルに存在しスナップショットに存在しないファイルは、「別のマシンで削除
	// された」か「前回の同期以降ここで作られた」かのどちらかである。両者を区別
	// できるのは、最後に同期したマニフェストだけである。base にあって remote に
	// ないなら削除、どちらにもないならここで新しく作られたものであり、手を触れ
	// ない。
	for path, localEntry := range local {
		if isIgnored(path) {
			continue
		}
		if _, stillRemote := remoteEntries[path]; stillRemote {
			continue
		}
		baseEntry, hadBase := baseEntries[path]
		if !hadBase {
			continue
		}
		if !synchronizedEqual(localEntry, baseEntry) {
			// リモートで削除され、ローカルで編集された競合。
			switch resolve {
			case ResolveLocal:
				// 残す。消さない。
				continue
			case ResolveRemote:
				// リモートを採用し、削除前の内容を History に残す。
			default:
				conflicts = append(conflicts, Conflict{
					Path: path, BaseDigest: baseEntry.SHA256, LocalDigest: localEntry.SHA256,
					BaseMode: baseEntry.Mode, LocalMode: localEntry.Mode,
				})
				continue
			}
		}
		request.Removals = append(request.Removals, storage.Removal{
			Path:         filepath.Join(root, filepath.FromSlash(path)),
			Precondition: storage.Precondition{Exists: true, Digest: localEntry.SHA256, Mode: preconditionMode(localEntry)},
			// リモート削除を適用する前に、暗号化された世代バックアップを残す。
			Backup: true,
		})
	}

	sort.Slice(request.Changes, func(i, j int) bool { return request.Changes[i].Path < request.Changes[j].Path })
	sort.Slice(request.Removals, func(i, j int) bool { return request.Removals[i].Path < request.Removals[j].Path })
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Path < conflicts[j].Path })

	if len(conflicts) == 0 && len(request.Changes) == 0 && len(request.Removals) == 0 {
		return storage.Request{}, nil, ErrNothingToApply
	}
	return request, conflicts, nil
}
