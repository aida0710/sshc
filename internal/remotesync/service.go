package remotesync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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

// KeyRecoveryPath records only remote generation metadata for an interrupted
// key rotation. Key material remains solely in the encrypted secret service.
const KeyRecoveryPath = "sshc/sync-key-recovery.json"

const stateSchemaVersion = 1

const (
	keyRecoverySchemaVersion  = 1
	keyRecoveryPrepared       = "prepared"
	keyRecoveryRemoteAdvanced = "remote_advanced"
)

type keyRecoveryJournal struct {
	SchemaVersion       int    `json:"schemaVersion"`
	Phase               string `json:"phase"`
	Target              string `json:"target"`
	ObjectKey           string `json:"objectKey"`
	OldETag             string `json:"oldETag"`
	NewETag             string `json:"newETag,omitempty"`
	OldCiphertextSHA256 string `json:"oldCiphertextSHA256"`
	NewCiphertextSHA256 string `json:"newCiphertextSHA256"`
}

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

// SnippetsPath is both the legacy plaintext path and the logical path inside a
// sync snapshot. On local disk its bytes are sealed with the device's master
// key; only the outer sync envelope carries the validated plaintext document.
const SnippetsPath = "sshc/snippets.json"

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
	// ErrRemoteDeleted reports that a preview cannot be committed because the
	// live object whose generation would be acknowledged no longer exists.
	ErrRemoteDeleted = errors.New("the acknowledged remote snapshot was deleted")
	// ErrPreviewStale reports that an apply request is not bound to the exact
	// ETag and manifest revision which the user previewed.
	ErrPreviewStale = errors.New("the synchronization preview is stale")
	// ErrRecoveryRequired stops synchronization after an interrupted key
	// rotation whose remote/local outcome cannot be proven without user input.
	ErrRecoveryRequired           = errors.New("an interrupted synchronization key rotation requires recovery")
	ErrRecoveryTargetChange       = errors.New("the synchronization target cannot change during key recovery")
	ErrHistoryKeyLossConfirmation = errors.New("key replacement requires confirmation that older history will become unreadable")
	// ErrNothingToPush reports a manual push whose file set is already the
	// current local base. Re-encrypting identical data would only create a
	// duplicate history object.
	ErrNothingToPush = errors.New("the workspace has no changes to push")
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
	// ErrVaultCodec は、現行のvault交換形式を扱う関数が接続されていない構成を報告する。
	ErrVaultCodec = errors.New("the sync vault codec is not configured")
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

// ParseDirection は現行契約の三つの名前だけを受け付ける。
func ParseDirection(name string) (Direction, bool) {
	switch Direction(name) {
	case DirectionBoth:
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
	SchemaVersion int `json:"schemaVersion"`
	// ETag は、このマシンが最後に push または pull したスナップショットを識別する。
	// 次の条件付き書き込みが比較される世代である。
	ETag string `json:"etag"`
	// Key は、その ETag が属するオブジェクト。世代はひとつのオブジェクトについての
	// 事実なので、設定を別のオブジェクトへ向けること、新しいパスや、オブジェクトに
	// 正直な名前を与えた改名、は、保存された世代を無意味にする。これがないと、次の
	// push は存在しないオブジェクトの世代を要求し、「別のマシンが push した」として
	// 拒否され、そこで勧められた pull は、pull すべきものを何ひとつ見つけられな
	// かった。
	Key string `json:"key"`
	// Target は Endpoint、Bucket、Region、Key から作る同期先の識別子。同じ Key を
	// 使う別バケットへ設定を変えたとき、以前の ETag と Base を流用しないために持つ。
	// 資格情報と direction は同期先そのものではないため含めない。
	Target string `json:"target,omitempty"`
	// Base は、そのスナップショットのマニフェスト。あとの pull に「別のマシンで削除
	// された」と「前回の同期以降ここで作られた」の違いを教えるのが、これで
	// ある。
	Base *Manifest `json:"base"`
	// Origin は、このインストールの不透明な ID。一度だけ生成され、マシンに関する何から
	// も導出されない。
	Origin        string         `json:"origin"`
	LastOperation *SyncOperation `json:"lastOperation"`
}

// KeyProvider returns the current synchronization key while operationMu is
// held. Callers must not snapshot a key before waiting for another stateful
// operation, because a completed rotation changes both the key and live ETag.
type KeyProvider func() (string, error)

// KeyReplacementProvider reads the old key and prepares its exact local CAS
// while operationMu is held. This keeps a concurrent Reconfigure's persisted
// settings and in-memory remote binding in one generation.
type KeyReplacementProvider func() (oldKey string, commit func() error, err error)

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

func targetID(config Config) string {
	return Digest([]byte(strings.Join([]string{
		config.Endpoint, config.Bucket, config.Region, ObjectKeyFor(config),
	}, "\x00")))
}

func stateMatchesTarget(current state, config Config) bool {
	return current.Target != "" && current.Target == targetID(config) && current.Key == ObjectKeyFor(config)
}

// Service は、一度にひとつの push か pull を行う。
type Service struct {
	workspace    *storage.Workspace
	transactions *storage.Manager
	now          func() string
	newOrigin    func() (string, error)
	historySeq   uint64

	integrations IntegrationHooks

	// operationMu serializes every stateful sync operation, including a complete
	// automatic receive/send cycle. binding has a separate, short-lived lock so
	// status reads and configuration do not wait for network I/O.
	operationMu sync.Mutex
	// historyMu prevents several callers from multiplying the bounded but
	// expensive history downloads and Argon2 work. History never holds
	// operationMu while doing remote I/O or decryption.
	historyMu      sync.Mutex
	mu             sync.Mutex
	binding        remoteBinding
	bindingVersion uint64
}

// IntegrationHooks binds remote synchronization to the encrypted local
// documents and mutation barriers owned by other packages. It is supplied once
// at construction and is immutable after the Service is published.
type IntegrationHooks struct {
	OpenVault          func() ([]byte, error)
	SealVault          func(document []byte) ([]byte, error)
	EmptyVaultDocument func() ([]byte, error)
	VaultAdopted       func() error
	OpenSnippets       func() ([]byte, error)
	SealSnippets       func(document []byte) ([]byte, error)
	SecretMutation     func(func() error) error
	StableSnapshot     func(func() error) error
}

func (hooks IntegrationHooks) validate() error {
	required := []struct {
		name    string
		present bool
	}{
		{"OpenVault", hooks.OpenVault != nil},
		{"SealVault", hooks.SealVault != nil},
		{"EmptyVaultDocument", hooks.EmptyVaultDocument != nil},
		{"VaultAdopted", hooks.VaultAdopted != nil},
		{"OpenSnippets", hooks.OpenSnippets != nil},
		{"SealSnippets", hooks.SealSnippets != nil},
		{"SecretMutation", hooks.SecretMutation != nil},
		{"StableSnapshot", hooks.StableSnapshot != nil},
	}
	for _, dependency := range required {
		if !dependency.present {
			return fmt.Errorf("remotesync integration %s is required", dependency.name)
		}
	}
	return nil
}

// NewService は、未設定のサービスを返す。
func NewService(workspace *storage.Workspace, transactions *storage.Manager,
	now func() string, newOrigin func() (string, error)) *Service {
	return &Service{
		workspace: workspace, transactions: transactions, now: now, newOrigin: newOrigin,
	}
}

// NewIntegratedService constructs the production service. Unlike NewService,
// which is the standalone core used by focused package tests, this constructor
// rejects incomplete cross-package wiring before the engine starts.
func NewIntegratedService(workspace *storage.Workspace, transactions *storage.Manager,
	now func() string, newOrigin func() (string, error), integrations IntegrationHooks) (*Service, error) {
	if err := integrations.validate(); err != nil {
		return nil, err
	}
	service := NewService(workspace, transactions, now, newOrigin)
	service.integrations = integrations
	return service, nil
}

// Configure は、この実行のバケットと資格情報を設定する。
//
// 資格情報はメモリ上に保持され、ワークスペースへ書かれることは決してない。自分の
// バケットへの鍵を運ぶスナップショットは、ブートストラップの便宜と引き換えに
// 爆発半径をはるかに大きくする。
func (s *Service) Configure(config Config, credentials objectstore.Credentials, client *objectstore.Client) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	config = normalizeConfig(config)
	if err := s.validateRecoveryTarget(config); err != nil {
		return err
	}
	s.configure(config, credentials, client)
	return nil
}

// ConfigureIfUnconfigured restores a persisted binding without overwriting a
// binding explicitly configured while the persisted settings were being read.
// The check and publication share operationMu with Reconfigure.
func (s *Service) ConfigureIfUnconfigured(config Config, credentials objectstore.Credentials, client *objectstore.Client) (bool, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.configured() {
		return false, nil
	}
	config = normalizeConfig(config)
	if err := s.validateRecoveryTarget(config); err != nil {
		return false, err
	}
	s.configure(config, credentials, client)
	return true, nil
}

// Reconfigure persists credentials and swaps the in-memory binding inside the
// same operation boundary. No key rotation can observe new secret settings with
// the previous remote client, or the reverse.
func (s *Service) Reconfigure(config Config, credentials objectstore.Credentials, client *objectstore.Client, persist func() error) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	config = normalizeConfig(config)
	if persist == nil {
		return errors.New("remote sync settings persistence is not configured")
	}
	if err := s.validateRecoveryTarget(config); err != nil {
		return err
	}
	if err := persist(); err != nil {
		return err
	}
	s.configure(config, credentials, client)
	return nil
}

// configure applies one complete remote binding while operationMu is held.
// Configure must be wholly before or wholly after every stateful operation so
// a successful settings response never leaves an older operation running.
func (s *Service) configure(config Config, credentials objectstore.Credentials, client *objectstore.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 末尾のスラッシュがリクエストへ届いたことはない、クライアントはパス全体を
	// 置き換えるが、スナップショットの行き先を表示するすべての画面には
	// "https://host//bucket" として届いていた。設定を保存する場所だけでなくここで
	// 切り詰めることで、これができる前に保存されたものもきれいになる。
	config = normalizeConfig(config)
	s.binding = remoteBinding{config: config, creds: credentials, client: client}
	s.bindingVersion++
}

func normalizeConfig(config Config) Config {
	config.Endpoint = strings.TrimRight(config.Endpoint, "/")
	config.Path = strings.Trim(config.Path, "/")
	return config
}

func (s *Service) validateRecoveryTarget(config Config) error {
	journal, exists, err := s.readKeyRecovery()
	if err != nil {
		return err
	}
	if exists && (journal.Target != targetID(config) || journal.ObjectKey != ObjectKeyFor(config)) {
		return ErrRecoveryTargetChange
	}
	return nil
}

// configuredBinding は、同期処理の開始時点の接続一式をひとつの値として返す。
// Configure はこのロックの前か後のどちらかにしか現れず、一回の操作の途中で
// config と client の世代が混ざることはない。
func (s *Service) configuredBinding() (remoteBinding, error) {
	binding, _, err := s.configuredBindingVersion()
	return binding, err
}

func (s *Service) configuredBindingVersion() (remoteBinding, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding := s.binding
	if _, ok := ParseDirection(string(binding.config.Direction)); !ok || binding.client == nil ||
		binding.config.Bucket == "" || binding.creds.AccessKeyID == "" {
		return remoteBinding{}, 0, ErrNotConfigured
	}
	return binding, s.bindingVersion, nil
}

// Configured は、バケットと資格情報が設定されているかを報告する。
func (s *Service) Configured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configuredLocked()
}

func (s *Service) configured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configuredLocked()
}

func (s *Service) configuredLocked() bool {
	_, validDirection := ParseDirection(string(s.binding.config.Direction))
	return validDirection && s.binding.client != nil && s.binding.config.Bucket != "" && s.binding.creds.AccessKeyID != ""
}

// Direction は、このマシンがどちら向きにデータを動かしてよいかを報告する。
func (s *Service) Direction() Direction {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 未設定のserviceには保存済み契約がない。status formの初期選択だけは通常の双方向を返す。
	if s.binding.config.Direction == "" {
		return DirectionBoth
	}
	return s.binding.config.Direction
}

// neverTravels は、ワークスペースに存在しても同期対象から除外するファイル。
// オブジェクトストア資格情報や端末固有の状態をスナップショットへ含めない。
var neverTravels = []string{
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
		body, err := s.workspace.FileSystem().ReadFile(absolute)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return Manifest{}, nil, err
		}
		mode := "0600"
		if info, err := s.workspace.FileSystem().Lstat(absolute); err == nil && info.Mode().Perm() == 0o700 {
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
// messageが空の場合は、自動同期と同じローカル差分の要約を使う。
func (s *Service) Push(ctx context.Context, passphrase, message string) (PushResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.push(ctx, passphrase, "", message)
}

func (s *Service) PushUsing(ctx context.Context, key KeyProvider, message string) (PushResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	// Preserve the public Push contract for a vault that has no remote target:
	// configuration is the first prerequisite, before a synchronization key.
	if _, err := s.configuredBinding(); err != nil {
		return PushResult{}, err
	}
	passphrase, err := currentOperationKey(key)
	if err != nil {
		return PushResult{}, err
	}
	return s.push(ctx, passphrase, "", message)
}

// ForcePush replaces the exact remote ETag which the user confirmed. It never
// performs an unconditional write; a remote change after confirmation is
// reported as ErrRemoteMoved.
func (s *Service) ForcePush(ctx context.Context, passphrase, expectedETag, message string) (PushResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if expectedETag == "" {
		return PushResult{}, ErrForcePushTarget
	}
	return s.push(ctx, passphrase, expectedETag, message)
}

func (s *Service) ForcePushUsing(ctx context.Context, key KeyProvider, expectedETag, message string) (PushResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if expectedETag == "" {
		return PushResult{}, ErrForcePushTarget
	}
	passphrase, err := currentOperationKey(key)
	if err != nil {
		return PushResult{}, err
	}
	return s.push(ctx, passphrase, expectedETag, message)
}

func currentOperationKey(provider KeyProvider) (string, error) {
	if provider == nil {
		return "", errors.New("synchronization key provider is not configured")
	}
	key, err := provider()
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", ErrWeakPassphrase
	}
	return key, nil
}

// ReplaceKey changes the key of an acknowledged live object with compare-and-swap,
// then commits the local key. If the local commit fails, the remote ciphertext is
// restored with another CAS so neither side silently advances on its own.
//
// A key on a machine which has never acknowledged this target is local setup, not
// remote rotation: it may be the key needed to open an existing remote snapshot.
func (s *Service) ReplaceKey(ctx context.Context, oldKey, newKey string, confirmHistoryLoss bool, commit func() error) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.replaceKey(ctx, oldKey, newKey, confirmHistoryLoss, commit)
}

func (s *Service) ReplaceKeyUsing(ctx context.Context, newKey string, confirmHistoryLoss bool, provider KeyReplacementProvider) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if provider == nil {
		return errors.New("sync key replacement provider is not configured")
	}
	oldKey, commit, err := provider()
	if err != nil {
		return err
	}
	return s.replaceKey(ctx, oldKey, newKey, confirmHistoryLoss, commit)
}

func (s *Service) replaceKey(ctx context.Context, oldKey, newKey string, confirmHistoryLoss bool, commit func() error) error {
	if commit == nil {
		return errors.New("sync key commit is not configured")
	}
	if !s.Configured() {
		return commit()
	}
	binding, err := s.configuredBinding()
	if err != nil {
		return err
	}
	if handled, err := s.resolveKeyRecovery(ctx, binding, newKey, commit); handled || err != nil {
		return err
	}
	if oldKey == "" || oldKey == newKey {
		return commit()
	}
	if !confirmHistoryLoss {
		return ErrHistoryKeyLossConfirmation
	}
	current, err := s.readState()
	if err != nil {
		return err
	}
	if !stateMatchesTarget(current, binding.config) || current.ETag == "" {
		return commit()
	}
	objectKey := ObjectKeyFor(binding.config)
	object, err := binding.client.Get(ctx, objectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return ErrRemoteMoved
		}
		return err
	}
	if object.ETag != current.ETag {
		return ErrRemoteMoved
	}
	archive, _, err := envelope.OpenWithin(object.Body, oldKey, envelope.AcceptedFromRemote)
	if err != nil {
		return err
	}
	// Rotation is deliberately not a legacy reader. Refuse to bless ciphertext
	// whose snapshot schema this binary would not otherwise accept.
	if _, _, err := Read(archive); err != nil {
		return err
	}
	key, err := envelope.Derive(newKey)
	if err != nil {
		return err
	}
	resealed, err := key.Seal(archive)
	if err != nil {
		return err
	}
	recovery := keyRecoveryJournal{
		SchemaVersion: keyRecoverySchemaVersion, Phase: keyRecoveryPrepared,
		Target: targetID(binding.config), ObjectKey: objectKey, OldETag: object.ETag,
		OldCiphertextSHA256: Digest(object.Body),
		NewCiphertextSHA256: Digest(resealed),
	}
	if err := s.writeKeyRecovery(recovery); err != nil {
		return err
	}
	newETag, err := binding.client.Put(ctx, objectKey, resealed, object.ETag, "")
	if err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			if cleanupErr := s.removeKeyRecovery(); cleanupErr != nil {
				return errors.Join(err, ErrRecoveryRequired, fmt.Errorf("clear sync key recovery journal: %w", cleanupErr))
			}
			return ErrRemoteMoved
		}
		// A timeout, disconnect, or 5xx can arrive after the store committed the
		// conditional PUT. Keep the prepared evidence until a fresh GET proves
		// whether the live body is exactly the old or new ciphertext.
		return errors.Join(err, ErrRecoveryRequired)
	}
	recovery.Phase = keyRecoveryRemoteAdvanced
	recovery.NewETag = newETag
	rollback := func(cause error) error {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		rollbackETag, rollbackErr := binding.client.Put(rollbackCtx, objectKey, object.Body, newETag, "")
		if rollbackErr != nil {
			if errors.Is(rollbackErr, objectstore.ErrPreconditionFailed) {
				rollbackErr = ErrRemoteMoved
			}
			return errors.Join(cause, ErrRecoveryRequired, fmt.Errorf("restore remote sync key: %w", rollbackErr))
		}
		restored := current
		restored.ETag = rollbackETag
		if stateErr := s.writeState(restored); stateErr != nil {
			return errors.Join(cause, ErrRecoveryRequired, fmt.Errorf("record restored remote sync key: %w", stateErr))
		}
		if removeErr := s.removeKeyRecovery(); removeErr != nil {
			return errors.Join(cause, ErrRecoveryRequired, fmt.Errorf("clear sync key recovery journal: %w", removeErr))
		}
		return cause
	}
	if err := s.writeKeyRecovery(recovery); err != nil {
		return rollback(errors.Join(ErrRecoveryRequired, fmt.Errorf("record advanced remote sync key: %w", err)))
	}
	advanced := current
	advanced.ETag = newETag
	if err := s.writeState(advanced); err != nil {
		return rollback(err)
	}
	if err := commit(); err != nil {
		return rollback(err)
	}
	if err := s.removeKeyRecovery(); err != nil {
		return errors.Join(ErrRecoveryRequired, fmt.Errorf("clear sync key recovery journal: %w", err))
	}
	return nil
}

// ResolveKeyRecovery lets the user re-enter the candidate new key after a
// process crash. The journal stores only ETags; candidate possession is proven
// by decrypting and validating the exact advanced live object.
func (s *Service) ResolveKeyRecovery(ctx context.Context, candidate string, commit func() error) (bool, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if commit == nil {
		return false, errors.New("sync key commit is not configured")
	}
	binding, err := s.configuredBinding()
	if err != nil {
		return false, err
	}
	return s.resolveKeyRecovery(ctx, binding, candidate, commit)
}

func (s *Service) resolveKeyRecovery(ctx context.Context, binding remoteBinding, candidate string, commit func() error) (bool, error) {
	journal, exists, err := s.readKeyRecovery()
	if err != nil || !exists {
		return exists, err
	}
	if journal.Target != targetID(binding.config) || journal.ObjectKey != ObjectKeyFor(binding.config) {
		return true, ErrRecoveryRequired
	}
	etag, err := binding.client.Head(ctx, journal.ObjectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return true, ErrRecoveryRequired
		}
		return true, err
	}
	if etag == journal.OldETag {
		current, err := s.readState()
		if err != nil {
			return true, err
		}
		if !stateMatchesTarget(current, binding.config) || current.Base == nil {
			return true, ErrRecoveryRequired
		}
		current.ETag = etag
		if err := s.writeState(current); err != nil {
			return true, errors.Join(ErrRecoveryRequired, err)
		}
		if err := s.removeKeyRecovery(); err != nil {
			return true, errors.Join(ErrRecoveryRequired, err)
		}
		return false, nil
	}
	object, err := binding.client.Get(ctx, journal.ObjectKey)
	if err != nil {
		return true, err
	}
	if object.ETag != etag {
		return true, ErrRemoteMoved
	}
	bodyDigest := Digest(object.Body)
	if bodyDigest == journal.OldCiphertextSHA256 {
		current, err := s.readState()
		if err != nil {
			return true, err
		}
		if !stateMatchesTarget(current, binding.config) || current.Base == nil {
			return true, ErrRecoveryRequired
		}
		current.ETag = etag
		if err := s.writeState(current); err != nil {
			return true, errors.Join(ErrRecoveryRequired, err)
		}
		if err := s.removeKeyRecovery(); err != nil {
			return true, errors.Join(ErrRecoveryRequired, err)
		}
		// The remote conclusively contains the original ciphertext, so the
		// encrypted vault must remain on the original key. ReplaceKey may now
		// retry the requested rotation as a fresh operation.
		return false, nil
	}
	if bodyDigest != journal.NewCiphertextSHA256 {
		return true, ErrRecoveryRequired
	}
	if journal.Phase == keyRecoveryRemoteAdvanced && (journal.NewETag == "" || etag != journal.NewETag) {
		return true, ErrRecoveryRequired
	}
	archive, _, err := envelope.OpenWithin(object.Body, candidate, envelope.AcceptedFromRemote)
	if err != nil {
		return true, err
	}
	if _, _, err := Read(archive); err != nil {
		return true, err
	}
	current, err := s.readState()
	if err != nil {
		return true, err
	}
	if !stateMatchesTarget(current, binding.config) || current.Base == nil {
		return true, ErrRecoveryRequired
	}
	current.ETag = etag
	if err := s.writeState(current); err != nil {
		return true, errors.Join(ErrRecoveryRequired, err)
	}
	if err := commit(); err != nil {
		return true, errors.Join(ErrRecoveryRequired, err)
	}
	if err := s.removeKeyRecovery(); err != nil {
		return true, errors.Join(ErrRecoveryRequired, err)
	}
	return true, nil
}

func (s *Service) push(ctx context.Context, passphrase, forcedETag, message string) (PushResult, error) {
	binding, err := s.configuredBinding()
	if err != nil {
		return PushResult{}, err
	}
	if err := s.ensureNoKeyRecovery(ctx, binding); err != nil {
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
	sameTarget := stateMatchesTarget(current, binding.config)
	if !sameTarget {
		// 別のremote headに保存されたbaseを、新しいbucketの親として記録しない。
		current.ETag = ""
		current.Base = nil
	}
	if forcedETag == "" && sameTarget && current.Base != nil && !manifestChanged(current.Base, manifest) {
		return PushResult{}, ErrNothingToPush
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
	manifest.Ancestors = manifestAncestors(current.Base)
	if strings.TrimSpace(message) == "" {
		message = draftFor(current.Base, manifest).Message
	}
	manifest.Message = message
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
	// 日付付きの候補が先。それが失敗すればライブは更新しない。ライブの条件付き
	// 書き込みが競争に負けたことを確定できた場合は、この候補だけを削除する。
	s.historySeq++
	dated, err := snapshotKeyFor(binding.config, manifest.CreatedAt, current.Origin, sealed, s.historySeq)
	if err != nil {
		return result, err
	}
	if _, err := client.Put(ctx, dated, sealed, "", "*"); err != nil {
		return result, err
	}
	result.ObjectCount++
	result.UploadedBytes += int64(len(sealed))

	etag, err := client.Put(ctx, objectKey, sealed, ifMatch, ifNoneMatch)
	if err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			if cleanupErr := client.Delete(ctx, dated); cleanupErr != nil {
				return result, errors.Join(ErrRemoteMoved, fmt.Errorf("remove the rejected history candidate: %w", cleanupErr))
			}
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
		ETag: etag, Key: objectKey, Target: targetID(binding.config), Base: &manifest, Origin: current.Origin,
		LastOperation: &operation,
	}); err != nil {
		return result, err
	}
	return result, nil
}

// PushDraft returns the same generated message used by unattended pushes.
func (s *Service) PushDraft() (PushDraft, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	manifest, _, err := s.Collect()
	if err != nil {
		return PushDraft{}, err
	}
	current, err := s.readState()
	if err != nil {
		return PushDraft{}, err
	}
	binding, err := s.configuredBinding()
	if err != nil {
		return PushDraft{}, err
	}
	base := current.Base
	if !stateMatchesTarget(current, binding.config) {
		base = nil
	}
	return draftFor(base, manifest), nil
}

func (s *Service) liveSnapshotFollows(
	ctx context.Context,
	binding remoteBinding,
	passphrase string,
	base *Manifest,
	incoming Manifest,
	incomingCiphertextDigest string,
) (bool, error) {
	if base == nil {
		return true, nil
	}
	if incoming.Revision == base.Revision || incoming.ParentRevision == base.Revision {
		return true, nil
	}
	if slices.Contains(incoming.Ancestors, base.Revision) {
		return true, nil
	}
	infos, _, err := binding.client.ListNewest(
		ctx, joinKey(binding.config.Path, SnapshotPrefix), maxHistoryGraphRevisions,
	)
	if err != nil {
		return false, err
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].LastModified.Equal(infos[j].LastModified) {
			return infos[i].Key > infos[j].Key
		}
		return infos[i].LastModified.After(infos[j].LastModified)
	})
	manifests := make(map[string]Manifest, len(infos))
	type cachedOpen struct {
		manifest Manifest
		valid    bool
	}
	opened := make(map[string]cachedOpen, len(infos)+1)
	// Push publishes the exact same sealed bytes to the dated and live keys.
	// Seed the cache with the live object which pull already authenticated, so
	// encountering its immutable copy does not spend a second KDF attempt.
	opened[incomingCiphertextDigest] = cachedOpen{manifest: incoming, valid: true}
	openAttempts := 0
	var downloaded int64
	for _, info := range infos {
		if info.Size <= 0 || downloaded+info.Size > maxHistoryGraphBytes {
			return false, nil
		}
		object, err := binding.client.Get(ctx, info.Key)
		if err != nil {
			if errors.Is(err, objectstore.ErrNotFound) {
				// LIST described this immutable history object but it disappeared
				// before GET. Its absence cannot prove ancestry and is a changed
				// remote graph, not a generic connectivity failure.
				return false, ErrRemoteMoved
			}
			return false, err
		}
		if info.ETag == "" || object.ETag != info.ETag {
			return false, ErrRemoteMoved
		}
		downloaded += int64(len(object.Body))
		ciphertextDigest := Digest(object.Body)
		cached, seen := opened[ciphertextDigest]
		if !seen {
			if openAttempts >= maxLiveLineageOpenAttempts {
				return false, nil
			}
			openAttempts++
			manifest, _, openErr := openSnapshotObject(object, passphrase)
			cached = cachedOpen{manifest: manifest, valid: openErr == nil}
			opened[ciphertextDigest] = cached
		}
		if !cached.valid {
			// A missing or unreadable link cannot prove ancestry. Fail closed rather
			// than treating a partial graph as permission to apply the live object.
			continue
		}
		manifests[cached.manifest.Revision] = cached.manifest
		if lineageReaches(manifests, incoming.ParentRevision, base.Revision) {
			return true, nil
		}
	}
	return false, nil
}

func manifestAncestors(base *Manifest) []string {
	if base == nil || !validRevision(base.Revision) {
		return nil
	}
	ancestors := make([]string, 0, min(MaxManifestAncestors, 1+len(base.Ancestors)))
	ancestors = append(ancestors, base.Revision)
	for _, revision := range base.Ancestors {
		if len(ancestors) == MaxManifestAncestors || slices.Contains(ancestors, revision) {
			break
		}
		ancestors = append(ancestors, revision)
	}
	return ancestors
}

func lineageReaches(manifests map[string]Manifest, revision, wanted string) bool {
	seen := map[string]bool{}
	for revision != "" && !seen[revision] {
		if revision == wanted {
			return true
		}
		seen[revision] = true
		ancestor, ok := manifests[revision]
		if !ok {
			return false
		}
		revision = ancestor.ParentRevision
	}
	return false
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
	captured, err := s.captureHistoryRead()
	if err != nil {
		return BucketView{}, err
	}
	return s.bucketStatus(ctx, captured)
}

func (s *Service) bucketStatus(ctx context.Context, captured historyReadSnapshot) (BucketView, error) {
	binding := captured.binding
	objectKey := ObjectKeyFor(binding.config)
	view := BucketView{CheckedAt: s.now(), History: []BucketObjectView{}}
	live, err := binding.client.Stat(ctx, objectKey)
	if err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return BucketView{}, err
	}
	liveExists := err == nil
	liveETag := ""
	if err == nil {
		liveETag = live.ETag
		view.Live = bucketObjectView(binding.config, live)
		view.LocalIsLive = captured.state.target == targetID(binding.config) && captured.state.etag != "" && captured.state.etag == live.ETag
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
	latest, latestErr := binding.client.Stat(ctx, objectKey)
	if latestErr != nil && !errors.Is(latestErr, objectstore.ErrNotFound) {
		return BucketView{}, latestErr
	}
	if (latestErr == nil) != liveExists || (latestErr == nil && latest.ETag != liveETag) {
		return BucketView{}, ErrRemoteMoved
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bindingVersion != captured.bindingVersion {
		return BucketView{}, ErrRemoteMoved
	}
	current, err := s.readState()
	if err != nil {
		return BucketView{}, err
	}
	if snapshotHistoryState(current) != captured.state {
		return BucketView{}, ErrRemoteMoved
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
//
// 永続化トランザクションは remotesync の実装詳細であり、transport へ公開しない。
// Written と Removed は利用者へ表示できるワークスペース相対パスである。
type PullResult struct {
	Written         []string
	Removed         []string
	Conflicts       []Conflict
	Manifest        Manifest
	Summary         SnapshotSummary
	DownloadedBytes int64
	CompletedAt     string
	ETag            string
	Origin          string
	objectKey       string
	target          string
	sourceKey       string
	sourceETag      string
	bindingVersion  uint64
	localState      historyStateSnapshot
	liveMissing     bool
	request         storage.Request
}

func (s *Service) pullPaths(request storage.Request) (written, removed []string) {
	written = make([]string, 0, len(request.Changes))
	for _, change := range request.Changes {
		written = append(written, s.DisplayPath(change.Path))
	}
	removed = make([]string, 0, len(request.Removals))
	for _, removal := range request.Removals {
		removed = append(removed, s.DisplayPath(removal.Path))
	}
	return written, removed
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

// PullRemoteHead previews the current live head after the user explicitly chose
// to trust it. This is the force-receive path for a legitimate force push which
// cannot be proven to descend from this installation's acknowledged revision.
// The snapshot is still authenticated and the later apply is bound to its exact
// ETag and revision. Send-only installations cannot apply any remote bytes.
func (s *Service) PullRemoteHead(ctx context.Context, passphrase string) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.pullWithRemoteAcceptance(ctx, passphrase, ResolveRemote, "", true)
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
	return s.pullWithRemoteAcceptance(ctx, passphrase, resolve, historyKey, false)
}

func (s *Service) pullWithRemoteAcceptance(
	ctx context.Context,
	passphrase string,
	resolve Resolution,
	historyKey string,
	acceptRemoteHead bool,
) (PullResult, error) {
	if acceptRemoteHead && s.Direction() == DirectionPush {
		return PullResult{}, ErrApplyRefused
	}
	binding, bindingVersion, err := s.configuredBindingVersion()
	if err != nil {
		return PullResult{}, err
	}
	if err := s.ensureNoKeyRecovery(ctx, binding); err != nil {
		return PullResult{}, err
	}
	current, err := s.readState()
	if err != nil {
		return PullResult{}, err
	}
	localState := snapshotHistoryState(current)
	objectKey := ObjectKeyFor(binding.config)
	sourceKey := objectKey
	stateETag := ""
	liveMissing := false
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
		} else {
			liveMissing = true
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
	ignoreRules, err := ignoreRulesFromSnapshot(contents)
	if err != nil {
		return PullResult{}, err
	}

	base := current.Base
	if !stateMatchesTarget(current, binding.config) {
		base = nil
	}
	// A bucket writer who does not know the synchronization key can still replay
	// an older, authentic ciphertext. Prove that a changed live head descends
	// from the locally acknowledged revision before treating it as an ordinary
	// pull. PullHistory remains the explicit rollback path.
	if historyKey == "" && !acceptRemoteHead {
		follows, lineageErr := s.liveSnapshotFollows(ctx, binding, passphrase, base, manifest, Digest(object.Body))
		if lineageErr != nil {
			return PullResult{}, lineageErr
		}
		if !follows {
			return PullResult{}, ErrRemoteMoved
		}
	}
	var local map[string]LocalEntry
	readLocal := func() error {
		var readErr error
		local, readErr = s.localDigests(manifest, base, ignoreRules)
		return readErr
	}
	if s.integrations.StableSnapshot != nil {
		err = s.integrations.StableSnapshot(readLocal)
	} else {
		err = readLocal()
	}
	if err != nil {
		return PullResult{}, err
	}

	request, conflicts, err := PlanEntriesWithIgnore(s.workspace.Root(), base, local, manifest, contents, resolve, ignoreRules.Match)
	if exchangeErr := s.stageVault(&request); exchangeErr != nil {
		return PullResult{}, exchangeErr
	}
	if err != nil && !errors.Is(err, ErrNothingToApply) {
		return PullResult{}, err
	}
	written, removed := s.pullPaths(request)
	return PullResult{
		Written: written, Removed: removed, Conflicts: conflicts, Manifest: manifest,
		Summary:         snapshotSummary(manifest, contents, len(object.Body)),
		DownloadedBytes: int64(len(object.Body)), CompletedAt: s.now(),
		ETag: stateETag, Origin: manifest.Origin, objectKey: objectKey, target: targetID(binding.config),
		sourceKey: sourceKey, sourceETag: object.ETag, bindingVersion: bindingVersion,
		localState: localState, liveMissing: liveMissing, request: request,
	}, err
}

// PullAndApply downloads, verifies and commits one exact preview generation as
// a single service operation. The expected values come from the earlier
// user-visible preview; this method then takes a fresh remote/local snapshot
// and keeps operationMu through the final workspace commit.
func (s *Service) PullAndApply(ctx context.Context, passphrase string, resolve Resolution, historyKey, expectedETag, expectedRevision string) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.pullAndApply(ctx, passphrase, resolve, historyKey, expectedETag, expectedRevision, false)
}

func (s *Service) PullAndApplyUsing(ctx context.Context, key KeyProvider, resolve Resolution, historyKey, expectedETag, expectedRevision string) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	passphrase, err := currentOperationKey(key)
	if err != nil {
		return PullResult{}, err
	}
	return s.pullAndApply(ctx, passphrase, resolve, historyKey, expectedETag, expectedRevision, false)
}

// PullAndApplyRemoteHeadUsing applies only the exact live head returned by an
// earlier PullRemoteHead preview. It exists only as an explicit force-receive
// operation; ordinary pulls retain ancestry protection.
func (s *Service) PullAndApplyRemoteHeadUsing(
	ctx context.Context,
	key KeyProvider,
	expectedETag string,
	expectedRevision string,
) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	passphrase, err := currentOperationKey(key)
	if err != nil {
		return PullResult{}, err
	}
	return s.pullAndApply(ctx, passphrase, ResolveRemote, "", expectedETag, expectedRevision, true)
}

func (s *Service) pullAndApply(ctx context.Context, passphrase string, resolve Resolution, historyKey, expectedETag, expectedRevision string, acceptRemoteHead bool) (PullResult, error) {
	result, err := s.pullWithRemoteAcceptance(ctx, passphrase, resolve, historyKey, acceptRemoteHead)
	if err != nil && !errors.Is(err, ErrNothingToApply) {
		return PullResult{}, err
	}
	if expectedETag == "" || expectedRevision == "" ||
		result.ETag != expectedETag || result.Manifest.Revision != expectedRevision {
		return PullResult{}, ErrPreviewStale
	}
	if err := s.validatePullForApply(ctx, result); err != nil {
		return PullResult{}, err
	}
	if err := s.apply(result); err != nil {
		return PullResult{}, err
	}
	return result, nil
}

// Apply は pull をコミットする。どれかのファイルが衝突しているあいだは拒否する。
// 半分だけ適用すれば、どちらの側とも一致しないワークスペースになるからだ。
func (s *Service) Apply(result PullResult) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if err := s.validatePullForApply(context.Background(), result); err != nil {
		return err
	}
	return s.apply(result)
}

func (s *Service) validatePullForApply(ctx context.Context, result PullResult) error {
	if result.liveMissing || result.ETag == "" {
		return ErrRemoteDeleted
	}
	binding, version, err := s.configuredBindingVersion()
	if err != nil {
		return err
	}
	if version != result.bindingVersion || targetID(binding.config) != result.target ||
		ObjectKeyFor(binding.config) != result.objectKey {
		return ErrRemoteMoved
	}
	current, err := s.readState()
	if err != nil {
		return err
	}
	if snapshotHistoryState(current) != result.localState {
		return ErrRemoteMoved
	}
	remoteETag, err := binding.client.Head(ctx, result.objectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return ErrRemoteDeleted
		}
		return err
	}
	if remoteETag != result.ETag {
		return ErrRemoteMoved
	}
	if result.sourceKey != "" && result.sourceKey != result.objectKey {
		sourceETag, err := binding.client.Head(ctx, result.sourceKey)
		if err != nil {
			if errors.Is(err, objectstore.ErrNotFound) {
				return ErrRemoteMoved
			}
			return err
		}
		if sourceETag != result.sourceETag {
			return ErrRemoteMoved
		}
	}
	return nil
}

func (s *Service) apply(result PullResult) error {
	if s.integrations.SecretMutation != nil {
		return s.integrations.SecretMutation(func() error { return s.applyWithSecretGeneration(result) })
	}
	return s.applyWithSecretGeneration(result)
}

func (s *Service) applyWithSecretGeneration(result PullResult) error {
	// direction は Pull ではなくここで検査する。プレビューは何も書かないので、
	// 送信専用のマシンでも、どれだけ遅れているかは知ることができる。
	// これが、別のマシンのバイト列をこのディスクへ置く呼び出しである。
	if s.Direction() == DirectionPush {
		return ErrApplyRefused
	}
	if len(result.Conflicts) > 0 {
		return ErrConflicts
	}
	// PullResult is a reusable preview. Keep its logical plaintext request
	// untouched and seal a private copy only at the final commit boundary.
	result.request.Changes = slices.Clone(result.request.Changes)
	result.request.Removals = slices.Clone(result.request.Removals)
	result.request.Directories = slices.Clone(result.request.Directories)
	if err := s.exchangeVault(&result.request); err != nil {
		return err
	}
	if err := s.exchangeSnippets(&result.request); err != nil {
		return err
	}
	written := len(result.request.Changes)
	removed := len(result.request.Removals)
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
		Written:         written, Removed: removed,
		CompletedAt: s.now(),
	}
	stateChange, err := s.stateChange(state{
		ETag: result.ETag, Key: result.objectKey, Target: result.target, Base: &manifest, Origin: origin,
		LastOperation: &operation,
	})
	if err != nil {
		return err
	}
	if result.request.Operation == "" {
		result.request.Operation = "sync.pull"
	}
	// State is deliberately the last write in the same journal as the workspace
	// files. A crash can therefore be completed from one durable transaction and
	// cannot leave a newly applied workspace paired with an older sync baseline.
	result.request.Changes = append(result.request.Changes, stateChange)
	// 別のマシンからのスナップショットは、このマシンにはないかもしれない
	// ディレクトリ（connections/work/、keys/work/）を指定する。stateの親も含め、
	// すべて同じrequestへ載せる。
	result.request.Directories = append(result.request.Directories,
		changeDirectories(s.workspace.Root(), result.request.Changes)...)
	if _, err := s.transactions.Commit(result.request); err != nil {
		return err
	}
	// 保管庫を置き換えたなら、それを配っている側に読み直させる。
	if s.integrations.VaultAdopted != nil && replacesVault(s.workspace.Root(), result.request) {
		if err := s.integrations.VaultAdopted(); err != nil {
			return err
		}
	}
	return nil
}

// exchangeSnippets maps the logical plaintext snapshot entry onto the same
// local path using this installation's master key. Previous ciphertext is not
// retained as a generation backup: snippets had no history before encryption,
// and keeping a nested old-key envelope would make password rotation unsafe.
func (s *Service) exchangeSnippets(request *storage.Request) error {
	local := filepath.Join(s.workspace.Root(), filepath.FromSlash(SnippetsPath))
	for index := range request.Changes {
		if request.Changes[index].Path != local {
			continue
		}
		if s.integrations.SealSnippets == nil {
			return ErrVaultCodec
		}
		if err := s.requireSnippetPrecondition(local, request.Changes[index].Precondition); err != nil {
			return err
		}
		sealed, err := s.integrations.SealSnippets(request.Changes[index].Contents)
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
			Path: local, Contents: sealed, Precondition: precondition, SkipBackup: true,
		}
	}
	for index := range request.Removals {
		if request.Removals[index].Path != local {
			continue
		}
		if err := s.requireSnippetPrecondition(local, request.Removals[index].Precondition); err != nil {
			return err
		}
		body, err := s.workspace.FileSystem().ReadFile(local)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		request.Removals[index] = storage.Removal{
			Path: local, Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(body)},
		}
	}
	return nil
}

func (s *Service) requireSnippetPrecondition(path string, expected storage.Precondition) error {
	if s.integrations.OpenSnippets == nil {
		return ErrVaultCodec
	}
	document, err := s.integrations.OpenSnippets()
	if err != nil {
		return err
	}
	actual := ""
	if document != nil {
		actual = Digest(document)
	}
	if expected.Exists == (document != nil) && (!expected.Exists || expected.Digest == actual) {
		return nil
	}
	return &storage.ConflictError{Path: path, Expected: expected.Digest, Actual: actual}
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
		body, err := s.workspace.FileSystem().ReadFile(localPath)
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

func (s *Service) statePath() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(StatePath))
}

func (s *Service) keyRecoveryPath() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(KeyRecoveryPath))
}

func (s *Service) readKeyRecovery() (keyRecoveryJournal, bool, error) {
	body, err := s.workspace.FileSystem().ReadFile(s.keyRecoveryPath())
	if errors.Is(err, fs.ErrNotExist) {
		return keyRecoveryJournal{}, false, nil
	}
	if err != nil {
		return keyRecoveryJournal{}, false, err
	}
	var journal keyRecoveryJournal
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil || journal.SchemaVersion != keyRecoverySchemaVersion ||
		journal.Target == "" || journal.ObjectKey == "" || journal.OldETag == "" ||
		len(journal.OldCiphertextSHA256) != 64 || len(journal.NewCiphertextSHA256) != 64 ||
		(journal.Phase != keyRecoveryPrepared && journal.Phase != keyRecoveryRemoteAdvanced) ||
		(journal.Phase == keyRecoveryRemoteAdvanced && journal.NewETag == "") {
		return keyRecoveryJournal{}, true, ErrRecoveryRequired
	}
	return journal, true, nil
}

func (s *Service) writeKeyRecovery(journal keyRecoveryJournal) error {
	body, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	precondition := storage.Precondition{}
	if current, readErr := s.workspace.FileSystem().ReadFile(s.keyRecoveryPath()); readErr == nil {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(current)}
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return readErr
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation: "sync.key-recovery",
		Changes: []storage.Change{{
			Path: s.keyRecoveryPath(), Contents: body, Precondition: precondition, SkipBackup: true,
		}},
	})
	return err
}

func (s *Service) removeKeyRecovery() error {
	body, err := s.workspace.FileSystem().ReadFile(s.keyRecoveryPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation: "sync.key-recovery.clear",
		Removals: []storage.Removal{{
			Path:         s.keyRecoveryPath(),
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(body)},
		}},
	})
	return err
}

// ensureNoKeyRecovery resolves only the outcome which can be proven without
// either key: a prepared rotation whose old ETag is still live. Every uncertain
// remote advance stops before another sync operation can obscure the evidence.
func (s *Service) ensureNoKeyRecovery(ctx context.Context, binding remoteBinding) error {
	journal, exists, err := s.readKeyRecovery()
	if err != nil || !exists {
		return err
	}
	if journal.Target != targetID(binding.config) || journal.ObjectKey != ObjectKeyFor(binding.config) {
		return ErrRecoveryRequired
	}
	etag, err := binding.client.Head(ctx, journal.ObjectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return ErrRecoveryRequired
		}
		return err
	}
	if etag != journal.OldETag {
		return ErrRecoveryRequired
	}
	// The old ciphertext is still authoritative. A prepared journal therefore
	// proves that the remote CAS never advanced (or was rolled back before any
	// local key commit), so the marker can be safely cleared.
	return s.removeKeyRecovery()
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
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil || parsed.SchemaVersion != stateSchemaVersion ||
		parsed.ETag == "" || parsed.Key == "" || parsed.Base == nil || parsed.LastOperation == nil {
		// 壊れた state ファイルは回復可能である。次の pull はこのマシンを、一度も
		// 同期していないマシンとして扱う。それは保守的な扱いだ、何も削除せず、
		// 推測する代わりに衝突として報告する。
		return state{}, nil
	}
	migratedBase, err := migrateSnapshotManifest(*parsed.Base)
	if err != nil {
		return state{}, nil
	}
	parsed.Base = &migratedBase
	return parsed, nil
}

func (s *Service) writeState(next state) error {
	change, err := s.stateChange(next)
	if err != nil {
		return err
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation:   "sync.state",
		Directories: changeDirectories(s.workspace.Root(), []storage.Change{change}),
		Changes:     []storage.Change{change},
	})
	return err
}

func (s *Service) stateChange(next state) (storage.Change, error) {
	next.SchemaVersion = stateSchemaVersion
	body, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return storage.Change{}, err
	}
	body = append(body, '\n')
	precondition := storage.Precondition{}
	if current, err := s.workspace.FileSystem().ReadFile(s.statePath()); err == nil {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(current)}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return storage.Change{}, err
	}
	return storage.Change{
		Path:         s.statePath(),
		Contents:     body,
		Precondition: precondition,
		// state は秘密を何も指定しないが、このアプリケーション自身のファイルで
		// あり、同期のたびにその世代が増えるのは、バックアップディレクトリの中の
		// 雑音でしかない。
		SkipBackup: true,
	}, nil
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
	binding, err := s.configuredBinding()
	if err != nil || !stateMatchesTarget(current, binding.config) {
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

// stageVault maps the travelling logical path to the local vault path while
// retaining plaintext and its logical precondition in the PullResult. Sealing
// is deliberately deferred until apply holds SecretMutation.
func (s *Service) stageVault(request *storage.Request) error {
	travelled := filepath.Join(s.workspace.Root(), filepath.FromSlash(TravelPath))
	local := filepath.Join(s.workspace.Root(), filepath.FromSlash(VaultPath))
	for index := range request.Changes {
		if request.Changes[index].Path == travelled {
			request.Changes[index].Path = local
		}
	}
	for _, removal := range request.Removals {
		if removal.Path == travelled {
			request.Changes = append(request.Changes, storage.Change{
				Path: local, Contents: nil, Precondition: removal.Precondition,
			})
		}
	}
	request.Removals = slices.DeleteFunc(request.Removals, func(removal storage.Removal) bool { return removal.Path == travelled })
	sort.Slice(request.Changes, func(i, j int) bool { return request.Changes[i].Path < request.Changes[j].Path })
	return nil
}

// exchangeVault validates the logical preview against the current unlocked
// vault, then seals it with the exact master-key generation held by apply.
func (s *Service) exchangeVault(request *storage.Request) error {
	if s.integrations.SealVault == nil {
		return ErrVaultCodec
	}
	local := filepath.Join(s.workspace.Root(), filepath.FromSlash(VaultPath))
	for index := range request.Changes {
		if request.Changes[index].Path != local {
			continue
		}
		if err := s.requireVaultPrecondition(local, request.Changes[index].Precondition); err != nil {
			return err
		}
		var sealed []byte
		var err error
		if len(request.Changes[index].Contents) == 0 {
			if s.integrations.EmptyVaultDocument == nil {
				return ErrVaultCodec
			}
			var empty []byte
			empty, err = s.integrations.EmptyVaultDocument()
			if err == nil {
				sealed, err = s.integrations.SealVault(empty)
			}
		} else {
			sealed, err = s.integrations.SealVault(request.Changes[index].Contents)
		}
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
	return nil
}

func (s *Service) requireVaultPrecondition(path string, expected storage.Precondition) error {
	if s.integrations.OpenVault == nil {
		return ErrVaultCodec
	}
	document, err := s.integrations.OpenVault()
	if err != nil {
		return err
	}
	actual := Digest(document)
	if (!expected.Exists && document == nil) || (expected.Exists && expected.Digest == actual) {
		return nil
	}
	return &storage.ConflictError{Path: path, Expected: expected.Digest, Actual: actual}
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
	binding, err := s.configuredBinding()
	if err != nil {
		return false, err
	}
	if !stateMatchesTarget(current, binding.config) {
		return len(manifest.Files) > 0, nil
	}
	return manifestChanged(current.Base, manifest), nil
}

type remoteGeneration struct {
	moved   bool
	etag    string
	deleted bool
	target  string
}

// inspectRemoteGeneration はHEADでETagだけを確認し、ライブオブジェクトが最後に同期した
// 世代から変わったかを返す。一度確認済みのliveが消えた場合も変更として扱い、空の
// bucketと区別する。呼び出し側はoperationMuを保持する。
func (s *Service) inspectRemoteGeneration(ctx context.Context) (remoteGeneration, error) {
	binding, err := s.configuredBinding()
	if err != nil {
		return remoteGeneration{}, err
	}
	current, err := s.readState()
	if err != nil {
		return remoteGeneration{}, err
	}
	objectKey := ObjectKeyFor(binding.config)
	target := targetID(binding.config)
	etag, err := binding.client.Head(ctx, objectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			if stateMatchesTarget(current, binding.config) && current.ETag != "" {
				return remoteGeneration{moved: true, deleted: true, target: target}, nil
			}
			// まだ誰も置いていない。受け取るものは無い。
			return remoteGeneration{target: target}, nil
		}
		return remoteGeneration{}, err
	}
	// 別のオブジェクトの世代は、このオブジェクトについて何も語らない。
	if !stateMatchesTarget(current, binding.config) {
		return remoteGeneration{moved: true, etag: etag, target: target}, nil
	}
	return remoteGeneration{moved: etag != current.ETag, etag: etag, target: target}, nil
}
