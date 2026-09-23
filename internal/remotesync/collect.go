package remotesync

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"sshc/internal/storage"
)

// neverTravels は、ワークスペースに存在しても同期対象から除外するファイル。
// オブジェクトストア資格情報や端末固有の状態をスナップショットへ含めない。
var neverTravels = []string{
	"sshc/local-vault-key",
	// オブジェクトストアの資格情報。暗号化されてはいるが、自分のバケットへの鍵を運ぶ
	// スナップショットは、スナップショットをひとつ入手した者が以後のすべてを取得
	// できることを意味する。
	SettingsPathRelative,
	// ハンドオフ。あるマシンのある実行のための URL と秘密であり、他のどこでも
	// 何の意味も持たない。
	"sshc/cli",
	// handoff文書を原子的に更新するための端末固有ロック。公開文書の兄弟fileなので、
	// sshc/cliの子path除外だけでは拾えない。
	"sshc/.cli.mutation.lock",
	// VPNコンテナが差し出す中継のソケット。この端末のこの実行のためのものであり、
	// 他のどこでも意味を持たない。socketはそもそも運べない。
	"sshc/vpn",
	// このマシン自身の帳簿。別のマシンのジャーナルやバックアップは、ここでは
	// 一度も起きていない書き込みを記述している。
	"sshc/journal",
	"sshc/backups",
	"sshc/history",
	"sshc/trash",
	// 接続履歴はこの端末での操作状態であり、別の端末へ移さない。
	"sshc/recent-connections.json",
	// Browser enrolment is a capability for this device's loopback origin. Only
	// hashes are stored, but moving them would still be meaningless and unsafe.
	"sshc/browser-registrations.json",
	// pane layout is device-local and contains no process/session state that can
	// be meaningfully restored on another engine.
	"sshc/workspaces.json",
	// Transfer jobs, resume checkpoints and device-local paths belong to one
	// engine and must never be copied to another machine.
	"sshc/transfers.json",
	// The workspace-wide process lock is runtime coordination state. Including
	// it would make every serialized local mutation look like a user edit and
	// can also surface a meaningless lock file on another machine.
	"sshc/mutation.lock",
	// vault の暗号文は端末固有のマスターパスワードで封印される。同期では復号済み文書を
	// スナップショット全体の暗号化内に一度だけ載せ、受信側で再封印する。
	VaultPath,
	TravelPath,
	SnippetsPath,
	StatePath,
	KeyRecoveryPath,
}

// SettingsPathRelative は、暗号化されたオブジェクトストア設定の相対パス。
// secret パッケージとの循環依存を避けるため、ここでも定義する。
const SettingsPathRelative = "sshc/sync-settings"

func excluded(relative string) bool {
	for _, segment := range strings.Split(relative, "/") {
		// storage transaction の一時ファイル。クラッシュ後に残っても別端末へ運ばない。
		if strings.HasPrefix(strings.ToLower(segment), ".sshc-") {
			return true
		}
	}
	for _, name := range neverTravels {
		if strings.EqualFold(relative, name) ||
			(len(relative) > len(name) && relative[len(name)] == '/' && strings.EqualFold(relative[:len(name)], name)) {
			return true
		}
	}
	return false
}

// inboundReserved applies the device-local outbound denylist to untrusted
// snapshots as well. The two logical documents are the only exceptions: they
// are validated and re-sealed by their owning services before commit.
func inboundReserved(relative string) bool {
	if relative == TravelPath || relative == SnippetsPath {
		return false
	}
	return excluded(relative)
}

// Collect は、スナップショットに含めるべきすべてのファイルを読む。
//
// すなわち、~/.ssh 配下の通常ファイルすべてと、パスワードの vault 文書である。
// 同期資格情報、端末固有の状態、処理中の一時ファイルは除外し、symlink と socket、
// FIFO、device はたどらず運ばない。
func (s *Service) Collect() (Manifest, map[string][]byte, error) {
	var manifest Manifest
	var contents map[string][]byte
	collect := func() error {
		var err error
		manifest, contents, err = s.collect()
		return err
	}
	if s.integrations.StableSnapshot != nil {
		if err := s.integrations.StableSnapshot(collect); err != nil {
			return Manifest{}, nil, err
		}
	} else if err := collect(); err != nil {
		return Manifest{}, nil, err
	}
	return manifest, contents, nil
}

func (s *Service) collect() (Manifest, map[string][]byte, error) {
	if s.integrations.OpenVault == nil {
		return Manifest{}, nil, ErrVaultCodec
	}
	ignoreRules, _, _, err := s.loadIgnoreRules()
	if err != nil {
		return Manifest{}, nil, err
	}
	relatives, err := s.walkWorkspaceMatching(ignoreRules.Match)
	if err != nil {
		return Manifest{}, nil, err
	}
	seen := map[string]bool{}
	contents := map[string][]byte{}
	var entries []Entry
	for _, relative := range relatives {
		relative = filepath.ToSlash(relative)
		if seen[relative] || excluded(relative) || ignoreRules.Match(relative) {
			continue
		}
		if err := checkPath(relative); err != nil {
			return Manifest{}, nil, err
		}
		seen[relative] = true

		absolute := filepath.Join(s.workspace.Root(), filepath.FromSlash(relative))
		body, err := storage.ReadFileLimited(s.workspace.FileSystem(), absolute, maxEntryBytes(relative))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return Manifest{}, nil, err
		}
		mode := "0600"
		if info, err := s.workspace.FileSystem().Lstat(absolute); err == nil && info.Mode().Perm()&0o100 != 0 {
			mode = "0700"
		}
		contents[relative] = body
		entries = append(entries, Entry{Path: relative, SHA256: Digest(body), Mode: mode})
	}
	current, err := s.readState()
	if err != nil {
		return Manifest{}, nil, err
	}
	// 保管庫は中身として載る。ディスク上のどのファイルとも対応しないので、
	// ここだけは読むのではなく尋ねる。
	document, err := s.integrations.OpenVault()
	if err != nil {
		return Manifest{}, nil, err
	}
	if document != nil || manifestContains(current.Base, TravelPath) {
		// A zero-length logical document is an authenticated tombstone only after
		// this installation has previously acknowledged a vault entry. A pristine
		// empty vault stays absent so a new installation does not manufacture an
		// edit or conflict merely by joining the target.
		contents[TravelPath] = document
		entries = append(entries, Entry{
			Path: TravelPath, SHA256: Digest(document), Mode: "0600",
		})
	}
	if s.integrations.OpenSnippets != nil {
		document, err := s.integrations.OpenSnippets()
		if err != nil {
			return Manifest{}, nil, err
		}
		if document != nil {
			contents[SnippetsPath] = document
			entries = append(entries, Entry{Path: SnippetsPath, SHA256: Digest(document), Mode: "0600"})
		}
	}
	if len(entries) > MaxEntries {
		return Manifest{}, nil, ErrSnapshotTooLarge
	}
	paths := newPortablePathSet()
	for _, entry := range entries {
		if err := paths.add(entry.Path); err != nil {
			return Manifest{}, nil, err
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	return Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     s.now(),
		Origin:        current.Origin,
		Files:         entries,
	}, contents, nil
}

// walkWorkspace は ~/.ssh を root として再帰的に通常ファイルだけを集める。
// Lstat を使うため symlink はディレクトリであっても追跡しない。
func (s *Service) walkWorkspace() ([]string, error) {
	return s.walkWorkspaceMatching(nil)
}

// walkWorkspaceMatching counts only files that can enter the returned set.
// Ignored trees may contain more than MaxEntries temporary files without
// making the synchronized snapshot exceed its entry limit.
func (s *Service) walkWorkspaceMatching(ignore func(string) bool) ([]string, error) {
	root := s.workspace.Root()
	var found []string
	var walk func(string, string) error
	walk = func(absolute, relativeDirectory string) error {
		entries, err := s.workspace.FileSystem().ReadDir(absolute)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			relative := entry.Name()
			if relativeDirectory != "" {
				relative = relativeDirectory + "/" + entry.Name()
			}
			if err := checkPath(relative); err != nil {
				return err
			}
			if excluded(relative) {
				continue
			}
			path := filepath.Join(absolute, entry.Name())
			info, err := s.workspace.FileSystem().Lstat(path)
			if err != nil {
				return err
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				continue
			}
			if info.IsDir() {
				if err := walk(path, relative); err != nil {
					return err
				}
				continue
			}
			if !info.Mode().IsRegular() {
				continue
			}
			if ignore != nil && ignore(filepath.ToSlash(relative)) {
				continue
			}
			found = append(found, relative)
			if len(found) > MaxEntries {
				return ErrSnapshotTooLarge
			}
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

// changeDirectories は、変更が着地する先のディレクトリを重複なく返す。
//
// 直接の親だけでよい。DirectoryCreate はルートより下で欠けている親も作るので、
// 設定エンジンの書き手が渡しているのと同じものである。
func changeDirectories(root string, changes []storage.Change) []storage.DirectoryCreate {
	seen := map[string]bool{}
	var directories []storage.DirectoryCreate
	for _, change := range changes {
		parent := filepath.Dir(change.Path)
		if parent == root || seen[parent] {
			continue
		}
		seen[parent] = true
		directories = append(directories, storage.DirectoryCreate{Path: parent})
	}
	return directories
}

// localDigests は、どちらかの側が知っているすべてのパスをハッシュする。これにより、
// このディスク上にあってどちらのマニフェストにもないファイルは、参照も変更もされない。
func (s *Service) localDigests(remote Manifest, base *Manifest, ignoreRules IgnoreRules) (map[string]LocalEntry, error) {
	paths := map[string]bool{}
	for _, item := range remote.Files {
		paths[item.Path] = true
	}
	if base != nil {
		for _, item := range base.Files {
			paths[item.Path] = true
		}
	}

	digests := map[string]LocalEntry{}
	for path := range paths {
		if ignoreRules.Match(path) {
			continue
		}
		// TravelPath はディスク上に存在しないため、復号済み vault 文書の digest を使う。
		if path == TravelPath && s.integrations.OpenVault != nil {
			document, err := s.integrations.OpenVault()
			if err != nil {
				return nil, err
			}
			if document != nil || manifestContains(base, TravelPath) {
				digests[path] = LocalEntry{SHA256: Digest(document), Mode: "0600"}
			}
			continue
		}
		if path == SnippetsPath && s.integrations.OpenSnippets != nil {
			document, err := s.integrations.OpenSnippets()
			if err != nil {
				return nil, err
			}
			if document != nil {
				digests[path] = LocalEntry{SHA256: Digest(document), Mode: "0600"}
			}
			continue
		}
		localPath := filepath.Join(s.workspace.Root(), filepath.FromSlash(path))
		body, err := storage.ReadFileLimited(s.workspace.FileSystem(), localPath, maxEntryBytes(path))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		info, err := s.workspace.FileSystem().Lstat(localPath)
		if err != nil {
			return nil, err
		}
		observedMode := info.Mode().Perm() & 0o700
		mode := "0600"
		if observedMode&0o100 != 0 {
			mode = "0700"
		}
		digests[path] = LocalEntry{
			SHA256: Digest(body), Mode: mode,
			ObservedMode: observedMode, ModeObserved: true,
		}
	}
	return digests, nil
}

func manifestContains(manifest *Manifest, wanted string) bool {
	if manifest == nil {
		return false
	}
	for _, entry := range manifest.Files {
		if entry.Path == wanted {
			return true
		}
	}
	return false
}
