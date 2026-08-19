package remotesync

import (
	"sshc/internal/envelope"
	"sshc/internal/objectstore"
)

// 同期の相手と封の語彙は、このパッケージが持つ。
//
// **HTTP 層が internal/objectstore と internal/envelope を名指ししないためである。**
// あれらは永続化とネットワークの原始操作であり、その名前がそのまま HTTP の契約に
// なっていると、保存の都合で付けた名前を変えるだけで外向きの応答が変わる。ここに
// 別名を置くことで、外から見える語彙の持ち主は同期サービスひとつになる。
//
// 別名であって包み直しではない。errors.Is はどちらの綴りでも通る——**同じ値である
// ことが要点で、翻訳の層をもう一枚増やしたいのではない。**
type (
	// Client は、スナップショットを置きに行く相手である。
	Client = objectstore.Client
	// Credentials は、その相手に名乗る資格情報である。**ワークスペースへは書かれない。**
	Credentials = objectstore.Credentials
)

var (
	// ErrInsecureEndpoint は、平文で話す行き先を断る。
	ErrInsecureEndpoint = objectstore.ErrInsecureEndpoint
	// ErrRefused は、相手が受け付けなかったことを報告する。
	ErrRefused = objectstore.ErrRefused
	// ErrWrongPassphrase は、封を開けられなかったことを報告する。
	ErrWrongPassphrase = envelope.ErrWrongPassphrase
	// ErrWeakPassphrase は、封に使うには短すぎる鍵を断る。
	ErrWeakPassphrase = envelope.ErrWeakPassphrase
	// ErrCostRefused は、開けるのに掛かりすぎるスナップショットを断る。
	ErrCostRefused = envelope.ErrCostRefused
	// ErrUnsupportedEnvelopeVersion は、この版が読めない封を報告する。
	ErrUnsupportedEnvelopeVersion = envelope.ErrUnsupportedVersion
)

// NewClient は、この設定で話す相手を組む。
//
// **組み立てるのはここである。** HTTP 層が自分で組んでいた頃、到達確認と保存後の
// 設定とで別々に組まれており、片方だけが endpoint の末尾スラッシュを落としていた。
func NewClient(config Config, credentials Credentials) *Client {
	return &Client{
		Endpoint: config.Endpoint, Bucket: config.Bucket,
		Region: config.Region, Creds: credentials,
	}
}

// ValidateKey は、この鍵で封ができるかを先に確かめる。
//
// **封をする段になって初めて分かるのでは遅い。** 弱い鍵は、書庫を作ってから
// 断られるより、受け取った時点で断る方がよい。
func ValidateKey(key string) error {
	_, err := envelope.Derive(key)
	return err
}
