package remotesync

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"rsc.io/qr"

	"sshc/internal/envelope"
	"sshc/internal/strictjson"
)

// 同期の設定の書き出し。設定済みのマシンが同期先とその認証情報を同期鍵で封じた文書を作り、
// 別のマシンは同期鍵を入力してそれを開く。文書はファイルにも QR コードにもなる。
//
// 封じるのは同期鍵である。取り込むマシンはどのみち同期鍵を入力するので、文書を開くために
// 別の秘密を覚える必要が無い。書き出すマシンは同期鍵を Vault に持っているので、何も
// 入力せずに書き出せる。
//
//	{"sshcSyncSettings":1,"sealed":"<base64 の envelope>"}
//
// 外側は、開く前に「これは同期の設定である」と見分けるためだけにあり、認証されない。
// 信じてよいのは、封を開いて得た中身だけである。

const (
	// settingsExportFormat は、外側の文書の形の版。
	settingsExportFormat = 1
	// settingsExportKind は、封の中身が同期の設定であることを示す印。同じ同期鍵は
	// スナップショットも封じるので、スナップショットの封を設定として読まないために置く。
	settingsExportKind = "sshc-sync-settings"
	// MaxSettingsExportLength は、取り込む文書として受け付ける長さの上限。入力の上限
	// いっぱいの設定（endpoint 2048、bucket と path 255、認証情報 512 ずつ）を封じて
	// base64 にしても 6 KiB に届かないので、その倍より少し上に置く。
	MaxSettingsExportLength = 16 << 10
)

// ErrSettingsExportRejected は、取り込む文書が同期の設定の書き出しとして読めないことを
// 報告する。同期鍵が違うときは ErrWrongPassphrase を返し、これとは分ける。
var ErrSettingsExportRejected = errors.New("this is not an exported sshc synchronization settings document")

// SettingsExport は、別のマシンが同じ同期先に参加するために要るもの。
//
// 送受信の向き（Direction）は含めない。マシンごとに決める値で、受信専用のマシンを
// 足すために主のマシンの設定を写す、という使い方があるからである。
type SettingsExport struct {
	Endpoint    string
	Bucket      string
	Path        string
	Region      string
	Credentials Credentials
}

type settingsExportDocument struct {
	Format int    `json:"sshcSyncSettings"`
	Sealed string `json:"sealed"`
}

type settingsExportContent struct {
	Kind            string `json:"kind"`
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
	Path            string `json:"path"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
}

// SealSettingsExport は、settings を syncKey で封じた文書を返す。
func SealSettingsExport(settings SettingsExport, syncKey string) (string, error) {
	plaintext, err := json.Marshal(settingsExportContent{
		Kind:     settingsExportKind,
		Endpoint: settings.Endpoint, Bucket: settings.Bucket, Path: settings.Path, Region: settings.Region,
		AccessKeyID: settings.Credentials.AccessKeyID, SecretAccessKey: settings.Credentials.SecretAccessKey,
	})
	if err != nil {
		return "", err
	}
	defer clear(plaintext)
	derivedKey, err := envelope.Derive(syncKey)
	if err != nil {
		return "", err
	}
	defer derivedKey.Destroy()
	sealed, err := derivedKey.Seal(plaintext)
	if err != nil {
		return "", err
	}
	document, err := json.Marshal(settingsExportDocument{
		Format: settingsExportFormat, Sealed: base64.StdEncoding.EncodeToString(sealed),
	})
	if err != nil {
		return "", err
	}
	return string(document), nil
}

// OpenSettingsExport は、SealSettingsExport が書いた文書を syncKey で開く。
//
// 文書は別のマシンから届く。鍵導出のコストは文書が決めるので、スナップショットと同じ
// OpenRemote の上限で開く。返す設定は検証していない。呼び出し側が ValidateTarget に通す。
func OpenSettingsExport(document, syncKey string) (SettingsExport, error) {
	if len(document) > MaxSettingsExportLength {
		return SettingsExport{}, ErrSettingsExportRejected
	}
	var outer settingsExportDocument
	if err := strictjson.Decode([]byte(strings.TrimSpace(document)), &outer); err != nil {
		return SettingsExport{}, ErrSettingsExportRejected
	}
	if outer.Format != settingsExportFormat {
		return SettingsExport{}, ErrSettingsExportRejected
	}
	sealed, err := base64.StdEncoding.DecodeString(outer.Sealed)
	if err != nil {
		return SettingsExport{}, ErrSettingsExportRejected
	}
	plaintext, derivedKey, err := envelope.OpenRemote(sealed, syncKey)
	if errors.Is(err, envelope.ErrWrongPassphrase) {
		return SettingsExport{}, ErrWrongPassphrase
	}
	if err != nil {
		return SettingsExport{}, ErrSettingsExportRejected
	}
	derivedKey.Destroy()
	defer clear(plaintext)
	var content settingsExportContent
	if err := strictjson.Decode(plaintext, &content); err != nil || content.Kind != settingsExportKind {
		return SettingsExport{}, ErrSettingsExportRejected
	}
	return SettingsExport{
		Endpoint: content.Endpoint, Bucket: content.Bucket, Path: content.Path, Region: content.Region,
		Credentials: Credentials{AccessKeyID: content.AccessKeyID, SecretAccessKey: content.SecretAccessKey},
	}, nil
}

// SettingsExportQRCode は、document を QR コードにしたときのマス目を、上の行から返す。
// 各行は '1'（黒）と '0'（白）の並びで、周りの余白は含まない。QR コードに入り切らない
// 長さなら ok は false で、そのときはファイルで渡す。
//
// 誤り訂正は最も低い L にする。画面に出したものをその場で読むので汚れや欠けが無く、
// マス目が粗いほうがカメラで読みやすい。
func SettingsExportQRCode(document string) (rows []string, ok bool) {
	code, err := qr.Encode(document, qr.L)
	if err != nil {
		return nil, false
	}
	rows = make([]string, code.Size)
	var row strings.Builder
	for y := range code.Size {
		row.Reset()
		for x := range code.Size {
			if code.Black(x, y) {
				row.WriteByte('1')
			} else {
				row.WriteByte('0')
			}
		}
		rows[y] = row.String()
	}
	return rows, true
}
