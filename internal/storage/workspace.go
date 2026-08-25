package storage

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sshc/internal/enginelock"
	"sshc/internal/platform/nativepath"
)

var (
	ErrOutsideWorkspace = errors.New("path is outside the sshc workspace")
	ErrSymlinkPath      = errors.New("path contains a symbolic link")
	ErrMissingDirectory = errors.New("parent directory does not exist")
	ErrNotDirectory     = errors.New("path component is not a directory")
	ErrInvalidHome      = errors.New("home directory must be an absolute path")
	ErrWorkspaceBusy    = errors.New("another sshc process is updating this workspace")
)

const (
	mutationLockName = "mutation.lock"
	mutationLockWait = 30 * time.Second
)

var workspaceMutationLockDirectory = func(workspace *Workspace) (string, error) {
	// The lock belongs to the workspace identity, not to process environment.
	// XDG_CACHE_HOME/HOME can legitimately differ between two processes that
	// edit the same explicit workspace; using either would silently split them.
	// sshc state is private, symlink-checked by Workspace, and excluded from sync.
	return workspace.StateDir(), nil
}

// Workspace は、すべての書き込みを、解決済みのユーザーの ~/.ssh ディレクトリに固定する。
//
// ルートは EvalSymlinks を通して一度だけ解決されるので、~/.ssh を別のボリューム
// に置いているユーザーでも動く。一方、ルートより下の構成要素はすべて本物の
// ディレクトリでなければならない。ルート下のシンボリックリンクは UI に表示される
// が、そこを通して書かれることはなく、リンクが書き込み可能な範囲を広げられない。
type Workspace struct {
	fileSystem FileSystem
	home       string
	root       string
	// mutation はこのWorkspaceを共有するManagerを直列化する。別processは同じmutexを
	// 共有できないため、lockMutationはさらにStateDir内のOS file lockも取得する。
	mutation sync.Mutex
}

func (w *Workspace) lockMutation() (func(), error) {
	w.mutation.Lock()
	lockDirectory, err := workspaceMutationLockDirectory(w)
	if err != nil {
		w.mutation.Unlock()
		return nil, err
	}
	// The production lock lives below the workspace root. Validate and create
	// that path through Workspace before handing it to the generic OS lock: the
	// generic API accepts arbitrary paths and must not turn a replaced ~/.ssh
	// symlink into an out-of-workspace mkdir, chmod, or lock-file creation.
	if w.Contains(lockDirectory) {
		if err := w.EnsureDirectory(lockDirectory); err != nil {
			w.mutation.Unlock()
			return nil, err
		}
	}

	lockPath := filepath.Join(lockDirectory, mutationLockName)
	deadline := time.Now().Add(mutationLockWait)
	for {
		release, err := enginelock.Acquire(lockPath)
		if err == nil {
			return func() {
				_ = release()
				w.mutation.Unlock()
			}, nil
		}
		if !errors.Is(err, enginelock.ErrRunning) {
			w.mutation.Unlock()
			return nil, err
		}
		if time.Now().After(deadline) {
			w.mutation.Unlock()
			return nil, ErrWorkspaceBusy
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// privateFileReader は任意実装とし、FileSystem の fake と呼び出し側へ Windows 固有の
// メソッドを要求しない。Windows の実装だけが提供し、Unix とユーザー提供の fake は
// 明示的に実装しない限り既存の ReadFile 方針を維持する。
type privateFileReader interface {
	ReadPrivateFile(path string) ([]byte, error)
}

type workspaceFileSystem struct {
	FileSystem
	stateDirectory string
	privateReader  privateFileReader
}

func (fileSystem workspaceFileSystem) ReadFile(path string) ([]byte, error) {
	if privateStateContains(fileSystem.stateDirectory, path) {
		return fileSystem.privateReader.ReadPrivateFile(path)
	}
	return fileSystem.FileSystem.ReadFile(path)
}

func (fileSystem workspaceFileSystem) WriteAtomic(path, prefix string, permission fs.FileMode, contents []byte) error {
	if writer, ok := fileSystem.FileSystem.(atomicFileWriter); ok {
		return writer.WriteAtomic(path, prefix, permission, contents)
	}
	return writeAtomicFileFallback(fileSystem.FileSystem, path, prefix, permission, contents)
}

// NewWorkspace は home/.ssh を解決する。ディレクトリがないことはエラーではない。
// 最初の書き込み時に作られる。
func NewWorkspace(fileSystem FileSystem, home string) (*Workspace, error) {
	if !filepath.IsAbs(home) {
		return nil, ErrInvalidHome
	}
	cleanedHome := filepath.Clean(home)
	root := filepath.Join(cleanedHome, ".ssh")
	resolved, err := fileSystem.EvalSymlinks(root)
	switch {
	case err == nil:
		root = filepath.Clean(resolved)
	case errors.Is(err, fs.ErrNotExist):
		// リテラルのパスを保つ。EnsureDirectory があとで作る。
	default:
		return nil, err
	}
	workspace := &Workspace{fileSystem: fileSystem, home: cleanedHome, root: root}
	if privateReader, ok := fileSystem.(privateFileReader); ok {
		workspace.fileSystem = workspaceFileSystem{
			FileSystem:     fileSystem,
			stateDirectory: filepath.Join(root, "sshc"),
			privateReader:  privateReader,
		}
	}
	return workspace, nil
}

func (w *Workspace) FileSystem() FileSystem { return w.fileSystem }

func (w *Workspace) Home() string { return w.home }

func (w *Workspace) Root() string { return w.root }

// StateDir は、ジャーナル・履歴・バックアップを保持するディレクトリ。
func (w *Workspace) StateDir() string { return filepath.Join(w.root, "sshc") }

// Contains は、candidate がルートであるか、その下にあるかを報告する。
//
// 判断は nativepath に任せる。ここで素の文字列前置比較をすると、Windows では
// 大小文字だけが違う同じディレクトリが層によって内と外に分かれ、UI が編集を
// 提示したのにストレージが拒む、という食い違いが起きる。
func (w *Workspace) Contains(candidate string) bool {
	return nativepath.Contains(w.root, candidate)
}

// Normalise は、与えられたままのホームディレクトリを基準に表現されたパスを、
// 解決済みのルートを基準に表現されたパスへ書き換える。
//
// Root は EvalSymlinks を通して解決され、Home は意図的にそうしない。Home は、
// このプロセスとその子が HOME に持つ値であり、それが ssh の表示するものであり、
// したがって SanitiseHomePaths が一致させなければならないものだからだ。そのため、
// ~/.ssh がリンク経由で到達される場合は常に、両者は同じディレクトリを二通りに
// 指定することになる。dotfiles のチェックアウトや、/var が private/var への
// リンクである macOS のあらゆる一時ディレクトリで、そうなる。
//
// "~" や "%d" を展開する呼び出し側は、ホームの表記に着地する。それを Root と
// 比べると、ワークスペースそのものであるパスがワークスペースの外にあると言われて
// しまい、それを尋ねる二か所（鍵の参照インデックスと、IdentityFile 行を書き換える
// 再配置）の両方が「いいえ」と応答していた。目に見える結果は、ファイルは移動する
// のにそれらを指定するディレクティブを何ひとつ書き換えない鍵の名前変更が、暗黙に
// 起きることであり、そして Keys 画面が設定全体を解決不能として報告することで
// あった。
//
// ホーム配下にないパスは、正規化されたうえで、それ以外は手を触れずに返る。
func (w *Workspace) Normalise(candidate string) string {
	cleaned := filepath.Clean(candidate)
	homeRoot := filepath.Join(w.home, ".ssh")
	if !nativepath.Contains(homeRoot, cleaned) {
		return cleaned
	}
	relative, err := filepath.Rel(homeRoot, cleaned)
	if err != nil {
		return cleaned
	}
	if relative == "." {
		return w.root
	}
	return filepath.Join(w.root, relative)
}

// ResolveForWrite は、candidate がルートより下の絶対パスであり、その親が本物の
// ディレクトリであり、かつ存在しないか通常ファイルであることを検証する。正規化
// されたパスを返す。
func (w *Workspace) ResolveForWrite(candidate string) (string, error) {
	return w.ResolveForWriteUnder(candidate, nil)
}

// ResolveForWriteUnder は、呼び出し側が同じトランザクションでこれから作る
// ディレクトリの集合を伴う ResolveForWrite である。
//
// これがないと、connections/work/ を作り、同じコミットで connections/work/lon.conf
// を書くリクエストは拒否される。リクエストが検査される時点で、まだ親が存在しない
// からだ。代案（トランザクションの前にディレクトリを作る）は、まさにこれが
// 取り除くために存在する、ジャーナル外の mkdir そのもので
// ある。
func (w *Workspace) ResolveForWriteUnder(candidate string, planned map[string]bool) (string, error) {
	cleaned := filepath.Clean(candidate)
	if !filepath.IsAbs(cleaned) || !w.Contains(cleaned) {
		return "", ErrOutsideWorkspace
	}
	if err := w.validateRoot(false); err != nil {
		return "", err
	}
	relative, err := filepath.Rel(w.root, cleaned)
	// ルートそのものは書き込み先ではない。素の文字列比較で弾かない。
	// まわりの包含判断は大小文字を畳むので、Windows ではルートの別表記だけが
	// そこをすり抜け、ワークスペースのルート自身が書き込み先になってしまう。
	if err != nil || relative == "." {
		return "", ErrOutsideWorkspace
	}

	// 確かめた表記を返す。呼び出し側の表記をそのまま返すと、検査したのは
	// ルートから組み立てた鎖なのに、書き込むのは別の表記ということになる。
	validated := filepath.Join(w.root, relative)
	segments := strings.Split(relative, string(filepath.Separator))
	current := w.root
	for index, segment := range segments {
		current = filepath.Join(current, segment)
		info, statErr := w.fileSystem.Lstat(current)
		last := index == len(segments)-1
		switch {
		case errors.Is(statErr, fs.ErrNotExist):
			if last {
				return validated, nil
			}
			if planned[current] {
				// このトランザクションがそれを作るので、残りのセグメントもすべて
				// このトランザクションの管轄である。
				continue
			}
			return "", ErrMissingDirectory
		case statErr != nil:
			return "", statErr
		case info.Mode()&fs.ModeSymlink != 0:
			return "", ErrSymlinkPath
		case last && !info.Mode().IsRegular():
			return "", ErrNotRegularFile
		case !last && !info.IsDir():
			return "", ErrNotDirectory
		}
	}
	return validated, nil
}

// ResolveDirectory は、candidate がルートより下の絶対パスであり、存在しないか
// 本物のディレクトリであり、かつ既存の祖先がシンボリックリンクではなく本物の
// ディレクトリであることを検証する。
//
// ファイルではなくディレクトリであるパスのための、ResolveForWrite の兄弟である。
// 親が存在しないことを意図的に許容する。ひとつのリクエストで connections/work/ と
// connections/work/eu/ を作るトランザクションがありえ、後者はリクエストが検査
// される時点でディスク上に親を持たないからだ。
func (w *Workspace) ResolveDirectory(candidate string) (string, error) {
	cleaned := filepath.Clean(candidate)
	if !filepath.IsAbs(cleaned) || !w.Contains(cleaned) {
		return "", ErrOutsideWorkspace
	}
	if err := w.validateRoot(true); err != nil {
		return "", err
	}
	relative, err := filepath.Rel(w.root, cleaned)
	// ルートそのものは書き込み先ではない。素の文字列比較で弾かない。
	// まわりの包含判断は大小文字を畳むので、Windows ではルートの別表記だけが
	// そこをすり抜け、ワークスペースのルート自身が書き込み先になってしまう。
	if err != nil || relative == "." {
		return "", ErrOutsideWorkspace
	}

	validated := filepath.Join(w.root, relative)
	current := w.root
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		info, statErr := w.fileSystem.Lstat(current)
		switch {
		case errors.Is(statErr, fs.ErrNotExist):
			// ここから下はまだ存在しない。作成の場合はそれが普通の
			// ケースである。
			return validated, nil
		case statErr != nil:
			return "", statErr
		case info.Mode()&fs.ModeSymlink != 0:
			return "", ErrSymlinkPath
		case !info.IsDir():
			return "", ErrNotDirectory
		}
	}
	return validated, nil
}

// EnsureDirectory は、candidate と、ルートより下で欠けている親を
// DirectoryPermission で作る。シンボリックリンクをたどることは拒否する。
func (w *Workspace) EnsureDirectory(candidate string) error {
	cleaned := filepath.Clean(candidate)
	if !w.Contains(cleaned) {
		return ErrOutsideWorkspace
	}
	rootInfo, rootErr := w.fileSystem.Lstat(w.root)
	if errors.Is(rootErr, fs.ErrNotExist) {
		if err := w.fileSystem.MkdirAll(w.root, DirectoryPermission); err != nil {
			return err
		}
	} else if rootErr != nil {
		return rootErr
	} else if rootInfo.Mode()&fs.ModeSymlink != 0 {
		return ErrSymlinkPath
	} else if !rootInfo.IsDir() {
		return ErrNotDirectory
	}

	relative, err := filepath.Rel(w.root, cleaned)
	if err != nil {
		return ErrOutsideWorkspace
	}
	current := w.root
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		if segment == "." {
			continue
		}
		current = filepath.Join(current, segment)
		info, statErr := w.fileSystem.Lstat(current)
		switch {
		case errors.Is(statErr, fs.ErrNotExist):
			if err := w.fileSystem.MkdirAll(current, DirectoryPermission); err != nil {
				return err
			}
		case statErr != nil:
			return statErr
		case info.Mode()&fs.ModeSymlink != 0:
			return ErrSymlinkPath
		case !info.IsDir():
			return ErrNotDirectory
		case current == w.StateDir() || strings.HasPrefix(current, w.StateDir()+string(filepath.Separator)):
			// 既存 state が親から緩い ACL を継承していた場合も、秘密を次に書く前に
			// OS adapter で締め直す。ユーザー管理の ~/.ssh 配下は対象外にする。
			if err := w.fileSystem.MkdirAll(current, DirectoryPermission); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Workspace) validateRoot(allowMissing bool) error {
	info, err := w.fileSystem.Lstat(w.root)
	if errors.Is(err, fs.ErrNotExist) && allowMissing {
		return nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return ErrMissingDirectory
	}
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return ErrSymlinkPath
	}
	if !info.IsDir() {
		return ErrNotDirectory
	}
	return nil
}
