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

var (
	// ErrInsecureEndpoint は、平文で通信する行き先を断る。
	ErrInsecureEndpoint = objectstore.ErrInsecureEndpoint
	// ErrRefused は、相手が受け付けなかったことを報告する。
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
	// ErrUnsupportedEnvelopeVersion は、この版で復号できない形式を報告する。
	ErrUnsupportedEnvelopeVersion = envelope.ErrUnsupportedVersion
	// ErrWorkspaceBusy は、別の処理が同じワークスペースを更新中であることを報告する。
	ErrWorkspaceBusy = storage.ErrWorkspaceBusy
)

// IsLocalChange reports that files changed between the pull preview and its
// transactional apply. HTTP callers need the distinction, but not the storage type.
func IsLocalChange(err error) bool {
	var conflict *storage.ConflictError
	return errors.As(err, &conflict)
}

// NewClient は、この設定で通信する相手を組む。
//
// 組み立てるのはここである。HTTP 層が自分で組んでいた頃、到達確認と保存後の
// 設定とで別々に組まれており、片方だけが endpoint の末尾スラッシュを落としていた。
func NewClient(config Config, credentials Credentials) *Client {
	return &Client{
		Endpoint: config.Endpoint, Bucket: config.Bucket,
		Region: config.Region, Creds: credentials,
	}
}

// ValidateKey は、暗号化処理の前に鍵の強度を検証する。
func ValidateKey(key string) error {
	derived, err := envelope.Derive(key)
	if err != nil {
		return err
	}
	derived.Destroy()
	return nil
}
