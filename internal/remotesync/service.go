package remotesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/objectstore"
	"sshc/internal/storage"
)

// StatePath は、このマシンが最後に何を同期したかを記録する場所。ワークスペース
// ルートからの相対である。他のすべてのファイルと同じくトランザクションマネージャを
// 通して書かれるので、同期の記録が書きかけで残ることはない。
const StatePath = "sshc/sync-state.json"

// archiveSuffix は、そのバイト列が何であるかを示す。暗号化された envelope の中の
// tar.gz である。ライブのオブジェクトも、日付付きのコピーも、これを持つ。
const archiveSuffix = "tar.gz.enc"

// ObjectName は、設定したパスの下に置く最新スナップショットの名前。
// 内容が暗号化された tar.gz であることを接尾辞で示す。
const ObjectName = "workspace." + archiveSuffix

// VaultPath は、保管庫がディスク上で置かれている場所。
const VaultPath = "sshc/secrets"

// TravelPath は、復号済みの vault 文書をスナップショット内で識別する名前。
// VaultPath と分けることで、受信時にローカルの鍵による再暗号化を必須にする。
const TravelPath = "sshc/secrets.json"

// SnapshotPrefix は、ライブのオブジェクトの隣に、push ごとの日付付きコピーを保持する。
//
// 固定キーへの条件付き書き込みで同時更新を検出する。日付付きコピーは手動復旧用で、
// 保持期間はバケットのライフサイクルルールで管理する。
const SnapshotPrefix = "snapshots/"

// ForcePushTarget is the fixed action-token target used for replacing the live
// remote snapshot. The token evidence also binds endpoint, bucket, object key,
// and ETag, so changing any of them invalidates the confirmation.
const ForcePushTarget = "remote-workspace"

// datedLayout は、スナップショットが作られた瞬間にちなんでコピーを名付ける。
// 人がS3の一覧から時刻を読める形は維持し、一意性はoriginとprocess内sequenceを
// 加えたsnapshotKeyForが担う。
const datedLayout = "2006-01-02-150405"

// joinKey は、設定されたパスの下にパスセグメントをひとつ置く。パスは既定で空で
// あり、それはバケットのルートを意味する。
func joinKey(path, name string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return name
	}
	return trimmed + "/" + name
}

// ObjectKeyFor は、この設定においてライブのスナップショットが置かれる場所。
func ObjectKeyFor(config Config) string {
	return joinKey(config.Path, ObjectName)
}

// SnapshotKeyFor は、createdAt に作られたスナップショットの日付付きコピーの場所。
func SnapshotKeyFor(config Config, createdAt string) (string, error) {
	moment, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return "", err
	}
	return joinKey(config.Path, SnapshotPrefix+moment.UTC().Format(datedLayout)+"."+archiveSuffix), nil
}

func snapshotKeyFor(config Config, createdAt, origin string, sealed []byte, sequence uint64) (string, error) {
	moment, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return "", err
	}
	originID := Digest([]byte(origin))[:16]
	// The ciphertext contains a fresh random salt and nonce. Its digest keeps
	// history unique across process restarts even when the stable origin and the
	// second-resolution timestamp are identical. sequence also avoids relying
	// on randomness alone for multiple pushes in one process.
	snapshotID := Digest(sealed)[:16]
	name := moment.UTC().Format(datedLayout) + "-" + originID + "-" + snapshotID + "-" + fmt.Sprintf("%06d", sequence)
	return joinKey(config.Path, SnapshotPrefix+name+"."+archiveSuffix), nil
}

var (
	// ErrNotConfigured は、バケットが設定されていないことを報告する。
	ErrNotConfigured = errors.New("remote sync is not configured")
	// ErrRemoteMoved は、このマシンが最後に見て以降スナップショットが変わったことを
	// 報告する。compare-and-swap の失敗であり、それこそが自動 push を安全にしている
	// 性質である。何も上書きされない。
	ErrRemoteMoved = errors.New("another machine has pushed since this one last synced")
	// ErrNoSnapshot は、空のバケットを報告する。
	ErrNoSnapshot = errors.New("the bucket holds no snapshot yet")
	// ErrConflicts は、判断なしには pull を適用できないことを報告する。
	ErrConflicts = errors.New("this pull needs a decision on at least one file")
	// ErrPushRefused は、受信専用に設定されたマシンでの push を報告する。
	ErrPushRefused = errors.New("this machine is set to receive only")
	// ErrApplyRefused は、送信専用に設定されたマシンでの apply を報告する。
	ErrApplyRefused = errors.New("this machine is set to send only")
	// ErrForcePushTarget reports a force-push confirmation for anything other
	// than the single live workspace object.
	ErrForcePushTarget = errors.New("the force-push target is not valid")
)

// Direction は、このマシンがどちら向きにデータを動かしてよいかを表す。
//
// 支配するのは二つの書き込みだけであり、それ以外は何も支配しない。プレビューは
// バケットを読むだけで何も書かないので、どちらの一方向設定でも利用できる。
// スナップショットを適用してはいけないノートパソコンでも、どれだけ遅れているかは
// 知ることができる。それが、安全設定と目隠しの違いである。
type Direction string

const (
	// DirectionBoth は既定。このマシンは push もでき、apply もできる。
	DirectionBoth Direction = "both"
	// DirectionPush は、そのマシンが源であるとき、設定が保存する価値のあるものである
	// ワークステーション、のためのもの。スナップショットを適用しないので、別のマシン
	// が push したものがこのディスク上のものを上書きすることはない。
	DirectionPush Direction = "push"
	// DirectionPull は、そのマシンが写しであるとき、共有のマシンや一時的なマシン 、
	// のためのもの。バケットへ書き込まないので、ここで行ったことが他のマシンへ届く
	// ことはない。
	DirectionPull Direction = "pull"
)

// ParseDirection は三つの名前を受け付け、空文字列は both として扱う。direction を
// 一度も聞いたことのない呼び出し側は、従来どおりに振る舞う。
func ParseDirection(name string) (Direction, bool) {
	switch Direction(name) {
	case "", DirectionBoth:
		return DirectionBoth, true
	case DirectionPush:
		return DirectionPush, true
	case DirectionPull:
		return DirectionPull, true
	default:
		return DirectionBoth, false
	}
}

// Config は、ユーザーが一度だけ与えるもの。
type Config struct {
	Endpoint string
	Bucket   string
	Region   string
	// Path は、すべてのオブジェクトが置かれる接頭辞。空ならバケットのルート。
	// バケットはたいていすでにこのアプリケーションにちなんで名付けられているので、
	// その中で同じ名前を繰り返すフォルダは、何もない階層をひとつ増やすだけである。
	Path      string
	Direction Direction
}

// SnapshotSummary distinguishes the source files users manage from one sealed
// object stored remotely. SourceBytes excludes archive headers and the
// manifest; SnapshotBytes is the exact encrypted HTTP body size.
type SnapshotSummary struct {
	CreatedAt     string `json:"createdAt"`
	FileCount     int    `json:"fileCount"`
	SourceBytes   int64  `json:"sourceBytes"`
	SnapshotBytes int64  `json:"snapshotBytes"`
}

type PushResult struct {
	Summary       SnapshotSummary `json:"summary"`
	ObjectCount   int             `json:"objectCount"`
	UploadedBytes int64           `json:"uploadedBytes"`
	CompletedAt   string          `json:"completedAt"`
}

type OperationKind string

const (
	OperationPush  OperationKind = "push"
	OperationApply OperationKind = "apply"
)

// SyncOperation is the last successful write operation. Fields which do not
// apply to its Kind stay zero and are omitted from the local state document.
type SyncOperation struct {
	Kind            OperationKind   `json:"kind"`
	Summary         SnapshotSummary `json:"summary"`
	ObjectCount     int             `json:"objectCount,omitempty"`
	UploadedBytes   int64           `json:"uploadedBytes,omitempty"`
	DownloadedBytes int64           `json:"downloadedBytes,omitempty"`
	Written         int             `json:"written,omitempty"`
	Removed         int             `json:"removed,omitempty"`
	CompletedAt     string          `json:"completedAt"`
}

type SyncStateView struct {
	Synced        bool
	At            string
	Origin        string
	Files         int
	LastOperation *SyncOperation
}

// state は、最後に成功した同期についての、このマシンの記録。
type state struct {
	// ETag は、このマシンが最後に push または pull したスナップショットを識別する。
	// 次の条件付き書き込みが比較される世代である。
	ETag string `json:"etag"`
	// Key は、その ETag が属するオブジェクト。世代はひとつのオブジェクトについての
	// 事実なので、設定を別のオブジェクトへ向けること、新しいパスや、オブジェクトに
	// 正直な名前を与えた改名、は、保存された世代を無意味にする。これがないと、次の
	// push は存在しないオブジェクトの世代を要求し、「別のマシンが push した」として
	// 拒否され、そこで勧められた pull は、pull すべきものを何ひとつ見つけられな
	// かった。
	Key string `json:"key,omitempty"`
	// Base は、そのスナップショットのマニフェスト。あとの pull に「別のマシンで削除
	// された」と「前回の同期以降ここで作られた」の違いを教えるのが、これで
	// ある。
	Base *Manifest `json:"base,omitempty"`
	// Origin は、このインストールの不透明な ID。一度だけ生成され、マシンに関する何から
	// も導出されない。
	Origin string `json:"origin"`
	// LastOperation was added after the original state format. Omitting it is
	// valid and keeps old installations readable without a migration.
	LastOperation *SyncOperation `json:"lastOperation,omitempty"`
}

// FileSource は、スナップショットに含めるべきワークスペース相対のパスを列挙する。
//
// これを注入するのは、「どのファイルが設定の一部なのか」が Include グラフの返す
// 問いであり、このパッケージからはそれが見えないからだ。結果を渡す形にすることで
// 依存の向きが正しく保たれる。ここにあるものは、設定サービスを何ひとつ import
// しない。
type FileSource func() ([]string, error)

// remoteBinding は、ひとつの設定保存で切り替わるリモート接続一式。
//
// client は Endpoint、Bucket、Region、資格情報を自分の中にも保持する。したがって、
// client と config を別々に読むと、再設定と同期が重なったときに、古いバケットへ
// 新しいパスで書くような、どの保存設定にも存在しなかった組合せを作れてしまう。
type remoteBinding struct {
	config Config
	creds  objectstore.Credentials
	client *objectstore.Client
}

// Service は、一度にひとつの push か pull を行う。
type Service struct {
	workspace    *storage.Workspace
	transactions *storage.Manager
	files        FileSource
	now          func() string
	newOrigin    func() (string, error)
	historySeq   uint64

	// OpenVault と SealVault は、vault 文書の復号と受信側での再暗号化を抽象化する。
	// secret パッケージへの依存を避けるため、関数として注入する。
	// どちらも nil の場合は、互換性のため暗号化済みファイルをそのまま同期する。
	OpenVault func() ([]byte, error)
	SealVault func(document []byte) ([]byte, error)
	// VaultAdopted は、vault の置換後にメモリ上の状態を再読込する通知。
	VaultAdopted func() error

	// operationMu serializes every stateful sync operation, including a complete
	// automatic receive/send cycle. binding has a separate, short-lived lock so
	// status reads and configuration do not wait for network I/O.
	operationMu sync.Mutex
	mu          sync.Mutex
	binding     remoteBinding
}

// NewService は、未設定のサービスを返す。
func NewService(workspace *storage.Workspace, transactions *storage.Manager, files FileSource,
	now func() string, newOrigin func() (string, error)) *Service {
	return &Service{
		workspace: workspace, transactions: transactions, files: files,
		now: now, newOrigin: newOrigin,
	}
}

// Configure は、この実行のバケットと資格情報を設定する。
//
// 資格情報はメモリ上に保持され、ワークスペースへ書かれることは決してない。自分の
// バケットへの鍵を運ぶスナップショットは、ブートストラップの便宜と引き換えに
// 爆発半径をはるかに大きくする。
func (s *Service) Configure(config Config, credentials objectstore.Credentials, client *objectstore.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 末尾のスラッシュがリクエストへ届いたことはない、クライアントはパス全体を
	// 置き換えるが、スナップショットの行き先を表示するすべての画面には
	// "https://host//bucket" として届いていた。設定を保存する場所だけでなくここで
	// 切り詰めることで、これができる前に保存されたものもきれいになる。
	config.Endpoint = strings.TrimRight(config.Endpoint, "/")
	config.Path = strings.Trim(config.Path, "/")
	s.binding = remoteBinding{config: config, creds: credentials, client: client}
}

// configuredBinding は、同期処理の開始時点の接続一式をひとつの値として返す。
// Configure はこのロックの前か後のどちらかにしか現れず、一回の操作の途中で
// config と client の世代が混ざることはない。
func (s *Service) configuredBinding() (remoteBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding := s.binding
	if binding.client == nil || binding.config.Bucket == "" || binding.creds.AccessKeyID == "" {
		return remoteBinding{}, ErrNotConfigured
	}
	return binding, nil
}

// Configured は、バケットと資格情報が設定されているかを報告する。
func (s *Service) Configured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.binding.client != nil && s.binding.config.Bucket != "" && s.binding.creds.AccessKeyID != ""
}

// Direction は、このマシンがどちら向きにデータを動かしてよいかを報告する。
func (s *Service) Direction() Direction {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.binding.config.Direction == "" {
		return DirectionBoth
	}
	return s.binding.config.Direction
}

// neverTravels は、Include グラフに現れても同期対象から除外するファイル。
// オブジェクトストア資格情報や端末固有の状態をスナップショットへ含めない。
var neverTravels = []string{
	// オブジェクトストアの資格情報。暗号化されてはいるが、自分のバケットへの鍵を運ぶ
	// スナップショットは、スナップショットをひとつ入手した者が以後のすべてを取得
	// できることを意味する。
	SettingsPathRelative,
	// ハンドオフ。あるマシンのある実行のための URL と秘密であり、他のどこでも
	// 何の意味も持たない。
	"sshc/cli",
	// このマシン自身の帳簿。別のマシンのジャーナルやバックアップは、ここでは
	// 一度も起きていない書き込みを記述している。
	"sshc/journal",
	"sshc/backups",
	"sshc/history",
	"sshc/trash",
	// 接続履歴はこの端末での操作状態であり、別の端末へ移さない。
	"sshc/recent-connections.json",
	// pane layout is device-local and contains no process/session state that can
	// be meaningfully restored on another engine.
	"sshc/workspaces.json",
	StatePath,
}

// SettingsPathRelative は、暗号化されたオブジェクトストア設定の相対パス。
// secret パッケージとの循環依存を避けるため、ここでも定義する。
const SettingsPathRelative = "sshc/sync-settings"

func excluded(relative string) bool {
	for _, name := range neverTravels {
		if relative == name || strings.HasPrefix(relative, name+"/") {
			return true
		}
	}
	return false
}

// Collect は、スナップショットに含めるべきすべてのファイルを読む。
//
// すなわち、FileSource が指定するもの、エントリファイルと、Include グラフが
// ワークスペース内で到達するすべてに加えて、metadata.json、パスワードの vault、
// そしてすべての鍵である。ソースが指定するのに存在しないファイルは、失敗させず
// にスキップする。Include はまだ存在しないファイルを指しうるし、それは診断であって
// 同期を拒む理由ではない。
func (s *Service) Collect() (Manifest, map[string][]byte, error) {
	relatives, err := s.files()
	if err != nil {
		return Manifest{}, nil, err
	}
	keys, err := s.walkKeys()
	if err != nil {
		return Manifest{}, nil, err
	}
	relatives = append(relatives, keys...)
	backgrounds, err := s.walkUnder("sshc/backgrounds")
	if err != nil {
		return Manifest{}, nil, err
	}
	// metadata が参照する背景画像も同期する。Android はサンドボックス外の
	// ファイルを直接選択できないため、同期経由で画像本体を受け取る。
	relatives = append(relatives, backgrounds...)
	relatives = append(relatives, "sshc/metadata.json")
	// OpenVault がある場合は、端末固有の鍵で暗号化された vault ファイルを除外し、
	// 復号済み文書を同期用の鍵で保護する。
	if s.OpenVault == nil {
		relatives = append(relatives, VaultPath)
	}

	seen := map[string]bool{}
	contents := map[string][]byte{}
	var entries []Entry
	for _, relative := range relatives {
		relative = filepath.ToSlash(relative)
		if seen[relative] || checkPath(relative) != nil || excluded(relative) {
			continue
		}
		// Include グラフから渡された場合も、端末固有の鍵で暗号化された vault は除外する。
		if relative == VaultPath && s.OpenVault != nil {
			continue
		}
		seen[relative] = true

		absolute := filepath.Join(s.workspace.Root(), filepath.FromSlash(relative))
		body, err := s.workspace.FileSystem().ReadFile(absolute)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return Manifest{}, nil, err
		}
		mode := "0600"
		if info, err := os.Stat(absolute); err == nil && info.Mode().Perm() == 0o700 {
			mode = "0700"
		}
		contents[relative] = body
		entries = append(entries, Entry{
			Path:   relative,
			SHA256: Digest(body),
			Mode:   mode,
			// 秘密鍵とは、keys/ 配下で .pub の接尾辞を持たないものすべてである。この印が、
			// pull にそれを SkipBackup 付きで適用させる。
			Secret: strings.HasPrefix(relative, "keys/") && !strings.HasSuffix(relative, ".pub"),
		})
	}
	// 保管庫は中身として載る。ディスク上のどのファイルとも対応しないので、
	// ここだけは読むのではなく尋ねる。
	if s.OpenVault != nil {
		document, err := s.OpenVault()
		if err != nil {
			return Manifest{}, nil, err
		}
		if document != nil {
			contents[TravelPath] = document
			entries = append(entries, Entry{
				Path: TravelPath, SHA256: Digest(document), Mode: "0600",
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	current, err := s.readState()
	if err != nil {
		return Manifest{}, nil, err
	}
	return Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     s.now(),
		Origin:        current.Origin,
		Files:         entries,
	}, contents, nil
}

func (s *Service) walkKeys() ([]string, error) {
	return s.walkUnder("keys")
}

// walkUnder は、ワークスペース相対のディレクトリ 1 つの下にある通常ファイルを
// 集める。無ければ何も返さない。同期を拒む理由ではない。
func (s *Service) walkUnder(directory string) ([]string, error) {
	root := filepath.Join(s.workspace.Root(), filepath.FromSlash(directory))
	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(s.workspace.Root(), path)
		if err != nil {
			return err
		}
		found = append(found, filepath.ToSlash(relative))
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return found, nil
}

// Check は、この設定が機能するかを知るために、バケットに問いをひとつ投げる。
//
// これは、打ち間違いと「正しく見えるのに何時間もあとの最初の push で、タイプミスを
// した画面から遠く離れたところで失敗する設定」とのあいだに立つものである。まだ
// スナップショットを持たないバケットは「見つからない」と返すが、それは機能して
// いるバケットである。問いは、このエンドポイント・このバケット名・この資格情報が、
// 結果を返すストアに届くかどうかであって、そこへ何かが push されたかどうかでは
// ない。
func (s *Service) Check(ctx context.Context) error {
	binding, err := s.configuredBinding()
	if err != nil {
		return err
	}
	return Check(ctx, binding.client, ObjectKeyFor(binding.config))
}

// Check は、このサービスが保持していないクライアントに対して同じ問いを投げる。
// 設定を保存する前に試せるようにするためだ。試されていない設定を登録することが、
// 打ち間違いを「正しく見える設定」に変えてしまう。
func Check(ctx context.Context, client *objectstore.Client, key string) error {
	if _, err := client.Head(ctx, key); err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return err
	}
	return nil
}

// Push は、このワークスペースを暗号化して書き込む。リモートが動いていれば拒否する。
//
// 最初の書き込みには If-None-Match: *、以後は最後に確認した ETag を If-Match に
// 指定し、別端末による更新を上書きしない。
func (s *Service) Push(ctx context.Context, passphrase string) (PushResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.push(ctx, passphrase, "")
}

// ForcePush replaces the exact remote ETag which the user confirmed. It never
// performs an unconditional write; a remote change after confirmation is
// reported as ErrRemoteMoved.
func (s *Service) ForcePush(ctx context.Context, passphrase, expectedETag string) (PushResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if expectedETag == "" {
		return PushResult{}, ErrForcePushTarget
	}
	return s.push(ctx, passphrase, expectedETag)
}

func (s *Service) push(ctx context.Context, passphrase, forcedETag string) (PushResult, error) {
	binding, err := s.configuredBinding()
	if err != nil {
		return PushResult{}, err
	}
	if binding.config.Direction == DirectionPull {
		return PushResult{}, ErrPushRefused
	}
	client := binding.client
	current, err := s.readState()
	if err != nil {
		return PushResult{}, err
	}
	if current.Origin == "" {
		if current.Origin, err = s.newOrigin(); err != nil {
			return PushResult{}, err
		}
	}

	manifest, contents, err := s.Collect()
	if err != nil {
		return PushResult{}, err
	}
	manifest.Origin = current.Origin
	objectKey := ObjectKeyFor(binding.config)
	if current.Key != objectKey {
		// 別のremote headに保存されたbaseを、新しいbucketの親として記録しない。
		current.ETag = ""
		current.Base = nil
	}
	parentRevision := ""
	if current.Base != nil {
		parentRevision = current.Base.Revision
		if parentRevision == "" {
			parentRevision, err = RevisionFor(*current.Base)
			if err != nil {
				return PushResult{}, err
			}
		}
	}
	if err := FinalizeManifest(&manifest, parentRevision); err != nil {
		return PushResult{}, err
	}
	archive, err := Build(manifest, contents)
	if err != nil {
		return PushResult{}, err
	}
	key, err := envelope.Derive(passphrase)
	if err != nil {
		return PushResult{}, err
	}
	sealed, err := key.Seal(archive)
	if err != nil {
		return PushResult{}, err
	}
	result := PushResult{Summary: snapshotSummary(manifest, contents, len(sealed))}

	ifMatch, ifNoneMatch := forcedETag, ""
	if ifMatch == "" {
		ifMatch = current.ETag
		if ifMatch == "" {
			ifNoneMatch = "*"
		}
	}
	// 日付付きのコピーが先。それが失敗すれば push は何も中途半端に残さず失敗する。
	// ライブの書き込みがそのあと競争に負けても、残るのはこのマシンがその時刻に確かに
	// 作ったスナップショットであり、それはそのフォルダが保持していると述べている
	// ものそのものである。
	s.historySeq++
	dated, err := snapshotKeyFor(binding.config, manifest.CreatedAt, current.Origin, sealed, s.historySeq)
	if err != nil {
		return result, err
	}
	if _, err := client.Put(ctx, dated, sealed, "", ""); err != nil {
		return result, err
	}
	result.ObjectCount++
	result.UploadedBytes += int64(len(sealed))

	etag, err := client.Put(ctx, objectKey, sealed, ifMatch, ifNoneMatch)
	if err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			return result, ErrRemoteMoved
		}
		return result, err
	}
	result.ObjectCount++
	result.UploadedBytes += int64(len(sealed))
	result.CompletedAt = s.now()
	operation := SyncOperation{
		Kind: OperationPush, Summary: result.Summary,
		ObjectCount: result.ObjectCount, UploadedBytes: result.UploadedBytes,
		CompletedAt: result.CompletedAt,
	}
	if err := s.writeState(state{
		ETag: etag, Key: objectKey, Base: &manifest, Origin: current.Origin,
		LastOperation: &operation,
	}); err != nil {
		return result, err
	}
	return result, nil
}

// BucketObjectView is non-secret object metadata shown on the sync screen.
type BucketObjectView struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified,omitempty"`
}

// BucketView is a live inspection of the configured bucket. History is sorted
// newest first and contains metadata only; encrypted bodies are not downloaded.
type BucketView struct {
	CheckedAt        string             `json:"checkedAt"`
	Live             *BucketObjectView  `json:"live,omitempty"`
	History          []BucketObjectView `json:"history"`
	HistoryTruncated bool               `json:"historyTruncated"`
	LocalIsLive      bool               `json:"localIsLive"`
}

const maxBucketHistoryObjects = 10000

func (s *Service) BucketStatus(ctx context.Context) (BucketView, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.bucketStatus(ctx)
}

func (s *Service) bucketStatus(ctx context.Context) (BucketView, error) {
	binding, err := s.configuredBinding()
	if err != nil {
		return BucketView{}, err
	}
	objectKey := ObjectKeyFor(binding.config)
	view := BucketView{CheckedAt: s.now(), History: []BucketObjectView{}}
	live, err := binding.client.Stat(ctx, objectKey)
	if err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return BucketView{}, err
	}
	if err == nil {
		view.Live = bucketObjectView(binding.config, live)
		current, stateErr := s.readState()
		if stateErr != nil {
			return BucketView{}, stateErr
		}
		view.LocalIsLive = current.Key == objectKey && current.ETag != "" && current.ETag == live.ETag
	}
	history, truncated, err := binding.client.ListNewest(ctx, joinKey(binding.config.Path, SnapshotPrefix), maxBucketHistoryObjects)
	if err != nil {
		return BucketView{}, err
	}
	view.HistoryTruncated = truncated
	sort.Slice(history, func(i, j int) bool {
		if history[i].LastModified.Equal(history[j].LastModified) {
			return history[i].Key > history[j].Key
		}
		return history[i].LastModified.After(history[j].LastModified)
	})
	for _, item := range history {
		view.History = append(view.History, *bucketObjectView(binding.config, item))
	}
	return view, nil
}

func bucketObjectView(config Config, item objectstore.ObjectInfo) *BucketObjectView {
	key := item.Key
	if prefix := strings.Trim(config.Path, "/"); prefix != "" {
		key = strings.TrimPrefix(key, prefix+"/")
	}
	modified := ""
	if !item.LastModified.IsZero() {
		modified = item.LastModified.UTC().Format(time.RFC3339)
	}
	return &BucketObjectView{Key: key, Size: item.Size, LastModified: modified}
}

type ForcePushConfirmation struct {
	ETag     string
	Evidence string
}

// ForcePushConfirmation binds one action token to the current configured
// destination and live ETag. The handler passes the same ETag to ForcePush, so
// a race after token consumption still fails the conditional PUT.
func (s *Service) ForcePushConfirmation(ctx context.Context, target string) (ForcePushConfirmation, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if target != ForcePushTarget {
		return ForcePushConfirmation{}, ErrForcePushTarget
	}
	binding, err := s.configuredBinding()
	if err != nil {
		return ForcePushConfirmation{}, err
	}
	objectKey := ObjectKeyFor(binding.config)
	live, err := binding.client.Stat(ctx, objectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return ForcePushConfirmation{}, ErrNoSnapshot
		}
		return ForcePushConfirmation{}, err
	}
	evidence := Digest([]byte(strings.Join([]string{
		binding.config.Endpoint, binding.config.Bucket, objectKey, live.ETag,
	}, "\x00")))
	return ForcePushConfirmation{ETag: live.ETag, Evidence: evidence}, nil
}

func snapshotSummary(manifest Manifest, contents map[string][]byte, snapshotBytes int) SnapshotSummary {
	var sourceBytes int64
	for _, body := range contents {
		sourceBytes += int64(len(body))
	}
	return SnapshotSummary{
		CreatedAt: manifest.CreatedAt, FileCount: len(manifest.Files),
		SourceBytes: sourceBytes, SnapshotBytes: int64(snapshotBytes),
	}
}

// PullResult は、適用する前の、pull が行うであろう内容。
type PullResult struct {
	Request         storage.Request
	Conflicts       []Conflict
	Manifest        Manifest
	Summary         SnapshotSummary
	DownloadedBytes int64
	CompletedAt     string
	ETag            string
	Origin          string
	objectKey       string
}

// Pull はスナップショットを取得し、それを適用すると何が変わるかを算出する。
//
// 何も書かない。Apply を別の呼び出しにしてあるのは、書き込みの前に必ず見せる
// プレビューを、このアプリケーションの他の部分と同じくユーザーに見せるためである。
func (s *Service) Pull(ctx context.Context, passphrase string, resolve Resolution) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.pull(ctx, passphrase, resolve, "")
}

// PullHistory previews one immutable dated snapshot. Applying it writes local
// files and records the current live ETag, but keeps the selected manifest as
// Base. The next push therefore creates a new head whose parent is the restored
// revision without unconditionally rewinding the remote object.
func (s *Service) PullHistory(ctx context.Context, passphrase, historyKey string, resolve Resolution) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.pull(ctx, passphrase, resolve, historyKey)
}

func (s *Service) pull(ctx context.Context, passphrase string, resolve Resolution, historyKey string) (PullResult, error) {
	binding, err := s.configuredBinding()
	if err != nil {
		return PullResult{}, err
	}
	objectKey := ObjectKeyFor(binding.config)
	sourceKey := objectKey
	stateETag := ""
	if historyKey != "" {
		sourceKey, err = historyObjectKey(binding.config, historyKey)
		if err != nil {
			return PullResult{}, err
		}
		live, statErr := binding.client.Stat(ctx, objectKey)
		if statErr != nil && !errors.Is(statErr, objectstore.ErrNotFound) {
			return PullResult{}, statErr
		}
		if statErr == nil {
			stateETag = live.ETag
		}
	}
	object, err := binding.client.Get(ctx, sourceKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return PullResult{}, ErrNoSnapshot
		}
		return PullResult{}, err
	}
	if historyKey == "" {
		stateETag = object.ETag
	}
	// この envelope の中のパラメータを選んだのは、それを書いた誰かであって、必ずしも
	// このインストールではない。別のユーザーのスナップショットが、パスフレーズの誤りが判明
	// する前にこのマシンへ 1 ギガバイトと 16 スレッドを費やさせられるようであっては
	// ならない。
	manifest, contents, err := openSnapshotObject(object, passphrase)
	if err != nil {
		return PullResult{}, err
	}

	current, err := s.readState()
	if err != nil {
		return PullResult{}, err
	}
	local, err := s.localDigests(manifest, current.Base)
	if err != nil {
		return PullResult{}, err
	}

	request, conflicts, err := Plan(s.workspace.Root(), current.Base, local, manifest, contents, resolve)
	if exchangeErr := s.exchangeVault(&request); exchangeErr != nil {
		return PullResult{}, exchangeErr
	}
	if err != nil && !errors.Is(err, ErrNothingToApply) {
		return PullResult{}, err
	}
	return PullResult{
		Request: request, Conflicts: conflicts, Manifest: manifest,
		Summary:         snapshotSummary(manifest, contents, len(object.Body)),
		DownloadedBytes: int64(len(object.Body)), CompletedAt: s.now(),
		ETag: stateETag, Origin: manifest.Origin, objectKey: objectKey,
	}, err
}

// Apply は pull をコミットする。どれかのファイルが衝突しているあいだは拒否する。
// 半分だけ適用すれば、どちらの側とも一致しないワークスペースになるからだ。
func (s *Service) Apply(result PullResult) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.apply(result)
}

func (s *Service) apply(result PullResult) error {
	// direction は Pull ではなくここで検査する。プレビューは何も書かないので、
	// 送信専用のマシンでも、どれだけ遅れているかは知ることができる。
	// これが、別のマシンのバイト列をこのディスクへ置く呼び出しである。
	if s.Direction() == DirectionPush {
		return ErrApplyRefused
	}
	if len(result.Conflicts) > 0 {
		return ErrConflicts
	}
	if len(result.Request.Changes)+len(result.Request.Removals) > 0 {
		// 別のマシンからのスナップショットは、このマシンにはないかもしれない
		// ディレクトリ（connections/work/、keys/work/）を指定する。そして
		// トランザクションマネージャが所有するのはファイルであってディレクトリでは
		// ない。ResolveForWrite は、親のない書き込みを拒否する。そこで、その
		// ディレクトリを同じリクエストに載せる。以前はジャーナルの外で作っており、
		// mkdir とコミットのあいだで落ちれば空のディレクトリが残った。
		result.Request.Directories = append(result.Request.Directories,
			changeDirectories(s.workspace.Root(), result.Request.Changes)...)
		if _, err := s.transactions.Commit(result.Request); err != nil {
			return err
		}
		// 保管庫を置き換えたなら、それを配っている側に読み直させる。読み直さな
		// ければ、運ばれてきたパスワードは次にロック解除するまで存在しないのと同じで
		// ある。
		if s.VaultAdopted != nil && replacesVault(s.workspace.Root(), result.Request) {
			if err := s.VaultAdopted(); err != nil {
				return err
			}
		}
	}
	current, err := s.readState()
	if err != nil {
		return err
	}
	origin := current.Origin
	if origin == "" {
		if origin, err = s.newOrigin(); err != nil {
			return err
		}
	}
	manifest := result.Manifest
	operation := SyncOperation{
		Kind: OperationApply, Summary: result.Summary,
		DownloadedBytes: result.DownloadedBytes,
		Written:         len(result.Request.Changes), Removed: len(result.Request.Removals),
		CompletedAt: s.now(),
	}
	return s.writeState(state{
		ETag: result.ETag, Key: result.objectKey, Base: &manifest, Origin: origin,
		LastOperation: &operation,
	})
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
func (s *Service) localDigests(remote Manifest, base *Manifest) (map[string]string, error) {
	paths := map[string]bool{}
	for _, item := range remote.Files {
		paths[item.Path] = true
	}
	if base != nil {
		for _, item := range base.Files {
			paths[item.Path] = true
		}
	}

	digests := map[string]string{}
	for path := range paths {
		// TravelPath はディスク上に存在しないため、復号済み vault 文書の digest を使う。
		if path == TravelPath && s.OpenVault != nil {
			document, err := s.OpenVault()
			if err != nil {
				return nil, err
			}
			if document != nil {
				digests[path] = Digest(document)
			}
			continue
		}
		body, err := s.workspace.FileSystem().ReadFile(filepath.Join(s.workspace.Root(), filepath.FromSlash(path)))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		digests[path] = Digest(body)
	}
	return digests, nil
}

func (s *Service) statePath() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(StatePath))
}

func (s *Service) readState() (state, error) {
	body, err := s.workspace.FileSystem().ReadFile(s.statePath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return state{}, nil
		}
		return state{}, err
	}
	var parsed state
	if err := json.Unmarshal(body, &parsed); err != nil {
		// 壊れた state ファイルは回復可能である。次の pull はこのマシンを、一度も
		// 同期していないマシンとして扱う。それは保守的な扱いだ、何も削除せず、
		// 推測する代わりに衝突として報告する。
		return state{}, nil
	}
	return parsed, nil
}

func (s *Service) writeState(next state) error {
	body, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	precondition := storage.Precondition{}
	if current, err := s.workspace.FileSystem().ReadFile(s.statePath()); err == nil {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(current)}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation: "sync.state",
		Changes: []storage.Change{{
			Path:         s.statePath(),
			Contents:     body,
			Precondition: precondition,
			// state は秘密を何も指定しないが、このアプリケーション自身のファイルで
			// あり、同期のたびにその世代が増えるのは、バックアップディレクトリの中の
			// 雑音でしかない。
			SkipBackup: true,
		}},
	})
	return err
}

// Target は、この実行が指しているエンドポイントとバケットを、表示のために返す。
// アクセスキーと秘密が何かによって返されることは決してない。
func (s *Service) Target() (endpoint, bucket, path, region string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.binding.config.Endpoint, s.binding.config.Bucket, s.binding.config.Path, s.binding.config.Region
}

// SyncState returns a detached view so callers cannot mutate the state value
// retained by a later response.
func (s *Service) SyncState() SyncStateView {
	current, err := s.readState()
	if err != nil || current.ETag == "" || current.Base == nil {
		return SyncStateView{}
	}
	view := SyncStateView{
		Synced: true, At: current.Base.CreatedAt, Origin: current.Base.Origin,
		Files: len(current.Base.Files),
	}
	if current.LastOperation != nil {
		operation := *current.LastOperation
		view.LastOperation = &operation
	}
	return view
}

// DisplayPath は、ワークスペースの絶対パスを、このアプリケーションの他の部分が
// 表示する相対パスへ戻す。
func (s *Service) DisplayPath(absolute string) string {
	relative, err := filepath.Rel(s.workspace.Root(), absolute)
	if err != nil {
		return filepath.Base(absolute)
	}
	return filepath.ToSlash(relative)
}

// exchangeVault は TravelPath の文書を受信側の鍵で再暗号化し、VaultPath へ置く。
// 送信側に vault がない場合は、受信側の vault を削除しない。
func (s *Service) exchangeVault(request *storage.Request) error {
	if s.SealVault == nil {
		return nil
	}
	travelled := filepath.Join(s.workspace.Root(), filepath.FromSlash(TravelPath))
	local := filepath.Join(s.workspace.Root(), filepath.FromSlash(VaultPath))
	for index := range request.Changes {
		if request.Changes[index].Path != travelled {
			continue
		}
		sealed, err := s.SealVault(request.Changes[index].Contents)
		if err != nil {
			return err
		}
		precondition := storage.Precondition{}
		if body, err := s.workspace.FileSystem().ReadFile(local); err == nil {
			precondition = storage.Precondition{Exists: true, Digest: storage.Digest(body)}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		request.Changes[index] = storage.Change{
			Path: local, Contents: sealed, Precondition: precondition,
		}
	}
	request.Removals = slices.DeleteFunc(request.Removals, func(removal storage.Removal) bool {
		return removal.Path == travelled
	})
	return nil
}

// replacesVault は、このリクエストが保管庫のファイルを書き換えるかを返す。
func replacesVault(root string, request storage.Request) bool {
	vault := filepath.Join(root, filepath.FromSlash(VaultPath))
	for _, change := range request.Changes {
		if change.Path == vault {
			return true
		}
	}
	return false
}

// diverged は、このディスクが最後に同期したものと違うかを返す。
// 自動巡回がoperationMuを保持したまま「押し出すものがあるか」を判断する内部操作
// であり、この判断にHTTPは1本も要らない。
func (s *Service) diverged() (bool, error) {
	manifest, _, err := s.Collect()
	if err != nil {
		return false, err
	}
	current, err := s.readState()
	if err != nil {
		return false, err
	}
	if current.Base == nil {
		// 一度も同期していない。載せるものがあるなら、それは違いである。
		return len(manifest.Files) > 0, nil
	}
	if len(manifest.Files) != len(current.Base.Files) {
		return true, nil
	}
	base := make(map[string]string, len(current.Base.Files))
	for _, item := range current.Base.Files {
		base[item.Path] = item.SHA256
	}
	for _, item := range manifest.Files {
		if base[item.Path] != item.SHA256 {
			return true, nil
		}
	}
	return false, nil
}

// remoteGeneration はHEADでETagだけを確認し、ライブオブジェクトが最後に同期した
// 世代から変わったかと現在のETagを返す。Autoは競合中の世代を覚え、同じ暗号文を
// 毎分ダウンロード・復号しない。呼び出し側はoperationMuを保持する。
func (s *Service) remoteGeneration(ctx context.Context) (bool, string, error) {
	binding, err := s.configuredBinding()
	if err != nil {
		return false, "", err
	}
	objectKey := ObjectKeyFor(binding.config)
	etag, err := binding.client.Head(ctx, objectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			// まだ誰も置いていない。受け取るものは無い。
			return false, "", nil
		}
		return false, "", err
	}
	current, err := s.readState()
	if err != nil {
		return false, "", err
	}
	// 別のオブジェクトの世代は、このオブジェクトについて何も語らない。
	if current.Key != objectKey {
		return true, etag, nil
	}
	return etag != current.ETag, etag, nil
}
