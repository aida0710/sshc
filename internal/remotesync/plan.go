package remotesync

import (
	"errors"
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
	isIgnored := func(path string) bool { return ignored != nil && ignored(path) }
	baseDigests := map[string]string{}
	if base != nil {
		for _, item := range base.Files {
			if isIgnored(item.Path) {
				continue
			}
			baseDigests[item.Path] = item.SHA256
		}
	}
	remoteDigests := map[string]string{}
	for _, item := range remote.Files {
		if isIgnored(item.Path) {
			continue
		}
		remoteDigests[item.Path] = item.SHA256
	}

	request := storage.Request{Operation: "sync.pull"}
	var conflicts []Conflict

	for _, item := range remote.Files {
		if isIgnored(item.Path) {
			continue
		}
		localDigest, present := local[item.Path]
		if present && localDigest == item.SHA256 {
			continue
		}
		baseDigest, hadBase := baseDigests[item.Path]
		contested := present && (!hadBase || localDigest != baseDigest)
		if contested && resolve == ResolveNone {
			// 両側で変更された内容は自動マージせず、digest だけを競合として報告する。
			conflict := Conflict{Path: item.Path, LocalDigest: localDigest, RemoteDigest: item.SHA256}
			if hadBase {
				conflict.BaseDigest = baseDigest
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
			precondition = storage.Precondition{Exists: true, Digest: localDigest}
		}
		request.Changes = append(request.Changes, storage.Change{
			Path:         filepath.Join(root, filepath.FromSlash(item.Path)),
			Contents:     contents[item.Path],
			Precondition: precondition,
			// 秘密鍵を含め、置き換えるファイルは暗号化された世代バックアップへ残す。
		})
	}

	// ローカルに存在しスナップショットに存在しないファイルは、「別のマシンで削除
	// された」か「前回の同期以降ここで作られた」かのどちらかである。両者を区別
	// できるのは、最後に同期したマニフェストだけである。base にあって remote に
	// ないなら削除、どちらにもないならここで新しく作られたものであり、手を触れ
	// ない。
	for path, localDigest := range local {
		if isIgnored(path) {
			continue
		}
		if _, stillRemote := remoteDigests[path]; stillRemote {
			continue
		}
		baseDigest, hadBase := baseDigests[path]
		if !hadBase {
			continue
		}
		if localDigest != baseDigest {
			// リモートで削除され、ローカルで編集された競合。
			switch resolve {
			case ResolveLocal:
				// 残す。消さない。
				continue
			case ResolveRemote:
				// リモートを採用し、削除前の内容を History に残す。
			default:
				conflicts = append(conflicts, Conflict{
					Path: path, BaseDigest: baseDigest, LocalDigest: localDigest,
				})
				continue
			}
		}
		request.Removals = append(request.Removals, storage.Removal{
			Path:         filepath.Join(root, filepath.FromSlash(path)),
			Precondition: storage.Precondition{Exists: true, Digest: localDigest},
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
