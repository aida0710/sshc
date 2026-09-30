package knownhosts

import (
	"context"
	"crypto/subtle"
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"sshc/internal/storage"
)

var (
	ErrUnverifiedCandidate = errors.New("a scanned key needs a matching fingerprint or an explicit acknowledgement")
	ErrEntryChanged        = errors.New("the entry on disk is not the entry that was displayed")
	ErrNoSuchEntry         = errors.New("no such known_hosts entry")
	ErrUnsupportedKeyType  = errors.New("unsupported host key type")
	// ErrNotWritable は、sshc が書かない場所の known_hosts へ鍵を書こうとしたことを
	// 報告する。書き込みはワークスペース（~/.ssh）の中だけで、journal と世代
	// バックアップを残す。/dev/null を指す UserKnownHostsFile もここに当たる。
	ErrNotWritable = errors.New("sshc writes known_hosts files only inside ~/.ssh")
)

// supportedKeyTypes は、このアプリケーションが known_hosts に書き込む種別の集合。
// それ以外は、検査せずに通すのではなく拒否する。
var supportedKeyTypes = map[string]bool{
	"ssh-ed25519":                        true,
	"ssh-rsa":                            true,
	"rsa-sha2-256":                       true,
	"rsa-sha2-512":                       true,
	"ecdsa-sha2-nistp256":                true,
	"ecdsa-sha2-nistp384":                true,
	"ecdsa-sha2-nistp521":                true,
	"sk-ssh-ed25519@openssh.com":         true,
	"sk-ecdsa-sha2-nistp256@openssh.com": true,
}

var base64Pattern = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,3}$`)

// Target は削除するエントリひとつを特定する。Digest はユーザーが見た行そのものの
// ハッシュなので、その間に編集されたファイルで違う行を失うことはない。
type Target struct {
	Line   int
	Digest string
}

// Listing は、このファイルの検索可能なビュー。
type Listing struct {
	Path  string
	Lines []Line
}

// storage.Request の操作名。known_hosts は ssh_config ではないので、設定と共有する
// storage.Manager の検証は、この操作が書くファイルを ssh_config として読まない。
const (
	OperationAdd    = "known_hosts.add"
	OperationDelete = "known_hosts.delete"
)

// Service は、トランザクションマネージャを通して known_hosts を読み書きする。
type Service struct {
	Workspace *storage.Workspace
	Manager   *storage.Manager
	Scanner   Scanner
}

// NewService は本番用の依存を配線する。
func NewService(workspace *storage.Workspace, manager *storage.Manager, scanner Scanner) *Service {
	return &Service{Workspace: workspace, Manager: manager, Scanner: scanner}
}

// Path は、このサービスが管理する known_hosts ファイル。
func (s *Service) Path() string { return filepath.Join(s.Workspace.Root(), "known_hosts") }

func (s *Service) read() ([]byte, error) { return readInWorkspace(s.Workspace, s.Path()) }

// Listing は query に一致するエントリを返す。
func (s *Service) Listing(query string) (Listing, error) {
	contents, err := s.read()
	if err != nil {
		return Listing{}, err
	}
	return Listing{Path: s.Path(), Lines: Search(ParseFile(contents), query)}, nil
}

// Evidence は現在のファイルのダイジェスト。known_hosts の変更に対するアクション
// トークンはこれに結び付けられるので、外部からの編集は確認を無効にする。
func (s *Service) Evidence() (string, error) {
	contents, err := s.read()
	if err != nil {
		return "", err
	}
	return storage.Digest(contents), nil
}

// Scan は、あるホストの鍵を ssh-keyscan に尋ねる。返される候補は、この呼び出しに
// おいて信頼されることはない。その判断は Add が別途行う。
func (s *Service) Scan(ctx context.Context, host string, port int) ([]Candidate, error) {
	return s.Scanner.Scan(ctx, host, port)
}

// Delete は、求められた行を取り除き、それ以外のバイトには一切触れない。
//
// 各 target は、ユーザーに表示された行のダイジェストを持つ。もはやそのハッシュに
// ならない行は拒否される。したがって、確認とリクエストのあいだに編集されたファイル
// で、誰も削除に同意していない行が失われることはない。
func (s *Service) Delete(targets []Target) (storage.Result, error) {
	contents, err := s.read()
	if err != nil {
		return storage.Result{}, err
	}
	file := ParseFile(contents)

	removing := make(map[int]bool, len(targets))
	for _, target := range targets {
		found := false
		for _, line := range file.Lines {
			if line.Number != target.Line {
				continue
			}
			found = true
			if storage.Digest([]byte(line.Raw)) != target.Digest {
				return storage.Result{}, ErrEntryChanged
			}
			removing[line.Number] = true
		}
		if !found {
			return storage.Result{}, ErrNoSuchEntry
		}
	}

	remaining := &File{}
	for _, line := range file.Lines {
		if removing[line.Number] {
			continue
		}
		remaining.Lines = append(remaining.Lines, line)
	}
	return s.commit(OperationDelete, replacement(s.Path(), contents, remaining.Render()))
}

// Add は、ユーザーが意図した鍵であると証明したうえで、スキャンした鍵を 1 行追加する。
//
// expectedFingerprint が鍵の実際のフィンガープリントと一致するか、ユーザーがその鍵
// は未検証であると明示的に承認したかのいずれかである。行は、クライアントが送って
// きたテキストを信用せず、検証済みの部品から組み立て直される。
func (s *Service) Add(candidate Candidate, expectedFingerprint string, acknowledged bool) (storage.Result, error) {
	fingerprint, err := candidateFingerprint(candidate)
	if err != nil {
		return storage.Result{}, err
	}
	switch {
	case expectedFingerprint != "":
		if subtle.ConstantTimeCompare([]byte(expectedFingerprint), []byte(fingerprint)) != 1 {
			return storage.Result{}, ErrUnverifiedCandidate
		}
	case !acknowledged:
		return storage.Result{}, ErrUnverifiedCandidate
	}
	return s.appendEntry(s.Path(), candidate)
}

// Remember は、接続の握手で受け入れたホスト鍵を path の known_hosts へ 1 行追加する。
//
// 鍵を受け入れるかは、接続がすでに StrictHostKeyChecking と利用者の確認で決めて
// いる。path は UserKnownHostsFile の最初のファイルである。sshc が書けるのは
// ワークスペース（~/.ssh）の中だけで、外のファイルには ErrNotWritable を返す。
// ~/.ssh がシンボリックリンクでも、ホームの表記の path はリンク先の実体へ書く。
func (s *Service) Remember(path string, candidate Candidate) error {
	if _, err := candidateFingerprint(candidate); err != nil {
		return err
	}
	resolved := s.Workspace.Normalise(path)
	if !s.Workspace.Contains(resolved) {
		return ErrNotWritable
	}
	_, err := s.appendEntry(resolved, candidate)
	return err
}

// ReadFile は、照合に使う known_hosts のファイルひとつを読む（パッケージの ReadFile）。
// UserKnownHostsFile と GlobalKnownHostsFile はワークスペースの外を指してよい。
func (s *Service) ReadFile(path string) ([]byte, error) { return ReadFile(s.Workspace, path) }

// candidateFingerprint は、書く前に鍵の種類と表記を確かめてから、フィンガープリントを返す。
func candidateFingerprint(candidate Candidate) (string, error) {
	if !supportedKeyTypes[candidate.KeyType] {
		return "", ErrUnsupportedKeyType
	}
	if !base64Pattern.MatchString(candidate.Key) {
		return "", ErrInvalidKey
	}
	return Fingerprint(candidate.Key)
}

// appendAttempts は、追加が別の書き込みと重なったときに読み直す回数の上限である。
//
// 追加は読む・重複を確かめる・書くの三段で、Workspace の全ペインを並行に開くと、
// 未知のホストを同時に書く接続どうしが重なる。追加は完全一致の重複を書かないので、
// 読み直して繰り返しても利用者の同意の意味は変わらない。数回で足りないのは誰かが
// 書き続けている場合で、そのときは衝突として返す。
const appendAttempts = 5

// appendEntry は path の known_hosts に鍵を 1 行追加する。同じ行がすでにあれば何もしない。
func (s *Service) appendEntry(path string, candidate Candidate) (storage.Result, error) {
	newLine := HostField(candidate.Host, candidate.Port) + " " + candidate.KeyType + " " + candidate.Key
	var result storage.Result
	var err error
	for attempt := 0; attempt < appendAttempts; attempt++ {
		result, err = s.appendOnce(path, newLine)
		if !IsExternalChange(err) {
			return result, err
		}
	}
	return result, err
}

func (s *Service) appendOnce(path, newLine string) (storage.Result, error) {
	contents, err := readInWorkspace(s.Workspace, path)
	if err != nil {
		return storage.Result{}, err
	}
	for _, line := range ParseFile(contents).Lines {
		if strings.TrimSpace(line.Raw) == newLine {
			// 完全な重複。書くものはない。
			return storage.Result{}, nil
		}
	}
	updated := string(contents)
	if updated != "" && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += newLine + "\n"
	return s.commit(OperationAdd, replacement(path, contents, []byte(updated)))
}

// replacement は、previous と読んだ path を updated に置き換える変更である。
// 読んだあとに別の書き込みがあれば、Precondition が外れて衝突になる。
func replacement(path string, previous, updated []byte) storage.Change {
	return storage.Change{
		Path:         path,
		Contents:     updated,
		Precondition: storage.Precondition{Exists: previous != nil, Digest: storage.Digest(previous)},
	}
}

func (s *Service) commit(operation string, change storage.Change) (storage.Result, error) {
	if err := s.Workspace.EnsureDirectory(filepath.Dir(change.Path)); err != nil {
		return storage.Result{}, err
	}
	return s.Manager.Commit(storage.Request{Operation: operation, Changes: []storage.Change{change}})
}
