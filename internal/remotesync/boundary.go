package remotesync

import (
	"errors"

	"sshc/internal/envelope"
	"sshc/internal/objectstore"
	"sshc/internal/storage"
)

// 同期先と暗号化のエラーは、このパッケージの語彙として公開する。
// HTTP 層へ objectstore と envelope の実装詳細を漏らさないためである。
type (
	// Client は、スナップショットを置きに行く相手である。
	Client = objectstore.Client
	// Credentials は、その相手に名乗る資格情報である。ワークスペースへは書かれない。
	Credentials = objectstore.Credentials
)

// Credentials の入力上限。CLI の対話入力と HTTP の設定画面が同じ値で断る。
const (
	MaxAccessKeyIDLength     = 512
	MaxSecretAccessKeyLength = 512
)

var (
	// ErrInsecureEndpoint は、平文で通信する行き先を断る。
	ErrInsecureEndpoint = objectstore.ErrInsecureEndpoint
	// ErrRefused は、相手が受け付けなかったことのうち、下の4つの個別の拒否に
	// 当たらないものを報告する。4つはこれに一致しない。
	ErrRefused = objectstore.ErrRefused
	// ErrAuthenticationFailed は、object storeが資格情報を認証できなかったことを報告する。
	ErrAuthenticationFailed = objectstore.ErrAuthenticationFailed
	// ErrAccessDenied は、object storeが同期先へのアクセスを許可しなかったことを報告する。
	ErrAccessDenied = objectstore.ErrAccessDenied
	// ErrRateLimited は、object storeが要求頻度を制限したことを報告する。
	ErrRateLimited = objectstore.ErrRateLimited
	// ErrServiceUnavailable は、object store自身が5xxで処理不能を報告したことを示す。
	ErrServiceUnavailable = objectstore.ErrServiceUnavailable
	// ErrObjectTooLarge は、暗号化されたremote objectが受信上限を超えたことを報告する。
	ErrObjectTooLarge = objectstore.ErrObjectTooLarge
	// ErrWrongPassphrase は、復号できなかったことを報告する。
	ErrWrongPassphrase = envelope.ErrWrongPassphrase
	// ErrWeakPassphrase は、暗号化に使うには短すぎる鍵を拒否する。
	ErrWeakPassphrase = envelope.ErrWeakPassphrase
	// ErrCostRefused は、開けるのに掛かりすぎるスナップショットを断る。
	ErrCostRefused = envelope.ErrCostRefused
	// ErrUnsupportedEnvelopeVersion は、このバージョンで復号できない形式を報告する。
	ErrUnsupportedEnvelopeVersion = envelope.ErrUnsupportedVersion
	// ErrWorkspaceBusy は、別の処理が同じワークスペースを更新中であることを報告する。
	ErrWorkspaceBusy = storage.ErrWorkspaceBusy
	// ErrPendingTransaction は、中断した変更が残っていて、復旧するまで書き込めないことを報告する。
	ErrPendingTransaction = storage.ErrPendingTransaction
)

// IsLocalChange reports that files changed between the pull preview and its
// transactional apply. HTTP callers need the distinction, but not the storage type.
func IsLocalChange(err error) bool {
	var conflict *storage.ConflictError
	return errors.As(err, &conflict)
}

// NewClient は、この設定で通信する相手を組む。
//
// 組み立てるのはここだけである。到達確認と保存後の設定が同じ組み立てを通るので、
// endpoint の末尾スラッシュのような正規化が片方だけに掛かることがない。
func NewClient(config Config, credentials Credentials) *Client {
	return &Client{
		Endpoint: config.Endpoint, Bucket: config.Bucket,
		Region: config.Region, Credentials: credentials,
	}
}

// ValidateKey は、暗号化処理の前に鍵の強度を検証する。
func ValidateKey(syncKey string) error {
	derived, err := envelope.Derive(syncKey)
	if err != nil {
		return err
	}
	derived.Destroy()
	return nil
}
