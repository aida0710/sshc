package remotesync

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"sshc/internal/objectstore"
	"sshc/internal/storage"
)

const (
	keyRecoverySchemaVersion  = 1
	keyRecoveryPrepared       = "prepared"
	keyRecoveryRemoteAdvanced = "remote_advanced"
)

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
// remote snapshot. The token evidence also binds the configured binding
// generation, target identity, and ETag, so changing any of them invalidates
// the confirmation.
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
	// この push が親スナップショットに対して記録した変更。ワークスペース相対パスで、
	// 内容は運ばない。
	Added    []string `json:"added,omitempty"`
	Modified []string `json:"modified,omitempty"`
	Removed  []string `json:"removed,omitempty"`
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

// KeyProvider returns the current synchronization key while operationMu is
// held. Callers must not snapshot a key before waiting for another stateful
// operation, because a completed rotation changes both the key and live ETag.
type KeyProvider func() (string, error)

// KeyReplacementProvider reads the old key and prepares its exact local CAS
// while operationMu is held. This keeps a concurrent CompleteSetup's persisted
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
