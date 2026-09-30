package remotesync

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"sshc/internal/browserauth"
	"sshc/internal/handoff"
	"sshc/internal/keys"
	"sshc/internal/recent"
	"sshc/internal/secret"
	"sshc/internal/storage"
	terminalworkspace "sshc/internal/workspace"
)

// neverTravels は、ワークスペースに存在しても同期対象から除外するファイル。
// オブジェクトストア資格情報や端末固有の状態をスナップショットへ含めない。
// パスは持ち主のパッケージの定数から組み立て、持ち主が名前を変えても除外が外れない
// ようにする。
var neverTravels = slices.Concat([]string{
	// パスワードなしの Vault を開く、このマシンだけの鍵。
	secret.LocalKeyPath,
	// オブジェクトストアの資格情報。暗号化されてはいるが、自分のバケットへの鍵を運ぶ
	// スナップショットは、スナップショットをひとつ入手した者が以後のすべてを取得
	// できることを意味する。
	secret.SettingsPath,
	// ハンドオフ。あるマシンのある実行のための URL と秘密であり、他のどこでも
	// 何の意味も持たない。
	path.Join(storage.StateDirectoryName, handoff.FileName),
	// handoff文書を原子的に更新するための端末固有ロック。公開文書の兄弟fileなので、
	// sshc/cliの子path除外だけでは拾えない。
	path.Join(storage.StateDirectoryName, handoff.MutationLockName),
	// このマシンで消した鍵の退避先。
	keys.TrashPathRelative,
	// 接続履歴はこの端末での操作状態であり、別の端末へ移さない。
	recent.PathRelative,
	// Browser enrolment is a capability for this device's loopback origin. Only
	// hashes are stored, but moving them would still be meaningless and unsafe.
	browserauth.PathRelative,
	// pane layout is device-local and contains no process/session state that can
	// be meaningfully restored on another engine.
	terminalworkspace.PathRelative,
	// 次の 2 つの持ち主は app で、app は remotesync を使う側なので、ここからは参照
	// できない。app のテストが、持ち主の定数と NeverTravels を照合する。
	//
	// VPNコンテナが差し出す中継のソケット。この端末のこの実行のためのものであり、
	// 他のどこでも意味を持たない。socketはそもそも運べない。
	"sshc/vpn",
	// Transfer jobs, resume checkpoints and device-local paths belong to one
	// engine and must never be copied to another machine.
	"sshc/transfers.json",
	// vault の暗号文は端末固有のマスターパスワードで封印される。同期では復号済み文書を
	// スナップショット全体の暗号化内に一度だけ載せ、受信側で再封印する。
	VaultPath,
	TravelPath,
	SnippetsPath,
	StatePath,
	KeyRecoveryPath,
},
	// このマシン自身の帳簿（ジャーナル、履歴、バックアップ、書き込みのロック）。
	storage.DeviceLocalPaths(),
)

// NeverTravels は、relative（ワークスペースからのスラッシュ区切りの相対パス）が、
// .sshcignore にかかわらずスナップショットへ入らないパスかを返す。真になるパスは、
// Vault と Snippet の論理文書を除き、受信したスナップショットにあっても断られる。
// remotesync から参照できない持ち主（app）が、自分のパスが除外されていることを
// 確かめるために使う。
func NeverTravels(relative string) bool { return excluded(relative) }

// ErrLocalPathUnportable は、送る対象のローカルのファイルが、同期するOSのどれかで
// 同じ名前として再現できない名前を持つことを報告する。受け取ったスナップショットの
// 検証の失敗（ErrUnsafePath）とは違い、直すのはこのマシンのファイル名か除外設定である。
var ErrLocalPathUnportable = errors.New("a local file to synchronize has a name that is not portable")

// UnportablePathError は、ErrLocalPathUnportable に当たったファイルを、利用者が
// 名前を変えるか除外できるように、ワークスペースからの相対パスで示す。
type UnportablePathError struct {
	Path string
}

func (e *UnportablePathError) Error() string {
	return fmt.Sprintf("%v: %s", ErrLocalPathUnportable, e.Path)
}

func (e *UnportablePathError) Unwrap() error { return ErrLocalPathUnportable }

func excluded(relative string) bool {
	for _, segment := range strings.Split(relative, "/") {
		// storage transaction の一時ファイル。クラッシュ後に残っても別のマシンへ運ばない。
		if storage.IsTemporaryName(segment) {
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
	contents := map[string][]byte{}
	manifest, err := s.collectStable(contents)
	if err != nil {
		return Manifest{}, nil, err
	}
	return manifest, contents, nil
}

// collectManifest は、Collect と同じ manifest を、ファイルの中身を持たずに求める。
// 変更があるかだけを知りたい呼び手（自動同期の毎分の巡回、送信の下書き）が、
// 大きな背景画像まで丸ごとメモリへ読まないために使う。
func (s *Service) collectManifest() (Manifest, error) {
	return s.collectStable(nil)
}

// collectStable は、ほかの書き手を止めたあいだに collect を走らせる。
func (s *Service) collectStable(contents map[string][]byte) (Manifest, error) {
	var manifest Manifest
	collect := func() error {
		var err error
		manifest, err = s.collect(contents)
		return err
	}
	if err := s.integrations.StableSnapshot(collect); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// collect は、送る対象を集めて manifest を作る。contents が nil でなければ、各ファイルと
// 論理文書の中身もそこへ入れる。nil なら、ファイルは中身を持たずに digest だけを求める。
func (s *Service) collect(contents map[string][]byte) (Manifest, error) {
	ignoreRules, _, _, err := s.loadIgnoreRules()
	if err != nil {
		return Manifest{}, err
	}
	relatives, err := s.walkWorkspaceMatching(ignoreRules.Match)
	if err != nil {
		return Manifest{}, err
	}
	current, err := s.readState()
	if err != nil {
		return Manifest{}, err
	}
	synchronizedModes := manifestModes(current.Base)
	seen := map[string]bool{}
	var entries []Entry
	for _, relative := range relatives {
		relative = filepath.ToSlash(relative)
		if seen[relative] || excluded(relative) || ignoreRules.Match(relative) {
			continue
		}
		if err := checkPath(relative); err != nil {
			return Manifest{}, &UnportablePathError{Path: relative}
		}
		seen[relative] = true

		absolute := filepath.Join(s.workspace.Root(), filepath.FromSlash(relative))
		digest, err := s.digestEntry(absolute, relative, contents)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return Manifest{}, err
		}
		var observed fs.FileMode
		if info, err := s.workspace.FileSystem().Lstat(absolute); err == nil {
			observed = info.Mode()
		}
		entries = append(entries, Entry{
			Path: relative, SHA256: digest, Mode: s.logicalMode(observed, synchronizedModes[relative]),
		})
	}
	// 保管庫は中身として載る。ディスク上のどのファイルとも対応しないので、
	// ここだけは読むのではなく尋ねる。
	document, err := s.integrations.OpenVault()
	if err != nil {
		return Manifest{}, err
	}
	if document != nil || manifestContains(current.Base, TravelPath) {
		// A zero-length logical document is an authenticated tombstone only after
		// this installation has previously acknowledged a vault entry. A pristine
		// empty vault stays absent so a new installation does not manufacture an
		// edit or conflict merely by joining the target.
		if contents != nil {
			contents[TravelPath] = document
		}
		entries = append(entries, Entry{
			Path: TravelPath, SHA256: Digest(document), Mode: "0600",
		})
	}
	snippets, err := s.integrations.OpenSnippets()
	if err != nil {
		return Manifest{}, err
	}
	if snippets != nil {
		if contents != nil {
			contents[SnippetsPath] = snippets
		}
		entries = append(entries, Entry{Path: SnippetsPath, SHA256: Digest(snippets), Mode: "0600"})
	}
	if len(entries) > MaxEntries {
		return Manifest{}, ErrSnapshotTooLarge
	}
	paths := newPortablePathSet()
	for _, entry := range entries {
		if err := paths.add(entry.Path); err != nil {
			return Manifest{}, &UnportablePathError{Path: entry.Path}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	return Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     s.now(),
		Origin:        current.Origin,
		Files:         entries,
	}, nil
}

// digestEntry は、ワークスペースのファイルひとつの digest を求める。contents が nil で
// なければ中身を読んでそこへ入れ、nil なら中身を持たずにハッシュへ流す。
func (s *Service) digestEntry(absolute, relative string, contents map[string][]byte) (string, error) {
	limit := storage.PayloadLimit(relative)
	if contents == nil {
		return storage.DigestFileLimited(s.workspace.FileSystem(), absolute, limit)
	}
	body, err := storage.ReadFileLimited(s.workspace.FileSystem(), absolute, limit)
	if err != nil {
		return "", err
	}
	contents[relative] = body
	return Digest(body), nil
}

// walkWorkspace は ~/.ssh を root として再帰的に通常ファイルだけを集める。
// Lstat を使うため symlink はディレクトリであっても追跡しない。
func (s *Service) walkWorkspace() ([]string, error) {
	return s.walkWorkspaceMatching(nil)
}

// walkWorkspaceMatching counts only files that can enter the returned set.
// Ignored trees may contain more than MaxEntries temporary files without
// making the synchronized snapshot exceed its entry limit.
//
// 名前が移植できるかはここでは見ない。除外したファイルや symlink の名前で走査全体を
// 止めないためで、送る対象の名前は collect が検査する。ReadDir が返す名前は「/」も
// 「..」も含まないので、ここで見なくてもワークスペースの外へは出ない。
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
func changeDirectories(root string, changes []storage.Change) []storage.DirectoryCreate {
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		paths = append(paths, change.Path)
	}
	return storage.ParentDirectoryCreates(root, paths)
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

	// 実行ビットを読めないときに引き継ぐ mode。前回の同期を優先し、初めて受け取る
	// ファイルは remote の mode を使う。
	synchronizedModes := manifestModes(&remote)
	for path, mode := range manifestModes(base) {
		synchronizedModes[path] = mode
	}
	digests := map[string]LocalEntry{}
	for path := range paths {
		if ignoreRules.Match(path) {
			continue
		}
		// TravelPath はディスク上に存在しないため、復号済み vault 文書の digest を使う。
		if path == TravelPath {
			document, err := s.integrations.OpenVault()
			if err != nil {
				return nil, err
			}
			if document != nil || manifestContains(base, TravelPath) {
				digests[path] = LocalEntry{SHA256: Digest(document), Mode: "0600"}
			}
			continue
		}
		if path == SnippetsPath {
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
		body, err := storage.ReadFileLimited(s.workspace.FileSystem(), localPath, storage.PayloadLimit(path))
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
		digests[path] = LocalEntry{
			SHA256: Digest(body), Mode: s.logicalMode(info.Mode(), synchronizedModes[path]),
			ObservedMode: info.Mode().Perm() & 0o700, ModeObserved: true,
		}
	}
	return digests, nil
}

// logicalMode は、ディスクで観測した権限を同期上の mode（"0600" か "0700"）にする。
// 実行ビットを読み返せない FileSystem（Windows）では、観測した権限は実行ビットに
// ついて何も語らない。そのため synchronized（前回までに同期した mode）を引き継ぐ。
// 観測どおりに 0600 と読むと、受信した 0700 のファイルを毎回 0600 として送り返し、
// ほかのマシンの実行ビットを落としてしまう。
func (s *Service) logicalMode(observed fs.FileMode, synchronized string) string {
	if !storage.ReportsExecutableBit(s.workspace.FileSystem()) {
		if synchronized != "" {
			return synchronized
		}
		return "0600"
	}
	if observed.Perm()&0o100 != 0 {
		return "0700"
	}
	return "0600"
}

// manifestModes は、manifest の各パスの mode を返す。manifest が無ければ空を返す。
func manifestModes(manifest *Manifest) map[string]string {
	modes := map[string]string{}
	if manifest == nil {
		return modes
	}
	for _, entry := range manifest.Files {
		modes[entry.Path] = entry.Mode
	}
	return modes
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
