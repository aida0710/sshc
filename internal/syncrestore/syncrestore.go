// Package syncrestore は、Vault に保存した同期の設定から remotesync を構成し直す。
//
// 画面の同期 API と自動同期の巡回の両方がこれを呼ぶ。ConfigureIfUnconfigured は先に
// 呼ばれた側の組み立てを採用するので、組み立て方が 2 か所にあると、どちらが先に来たかで
// 結果が変わってしまう。
package syncrestore

import (
	"net/http"

	"sshc/internal/remotesync"
	"sshc/internal/secret"
)

// FromVault は、Vault のロックが解除されていて同期がまだ構成されていないときだけ、保存済みの設定から
// client を組んで service を構成する。
//
// ロック中、未保存、解釈できない向きのときは何もしない。呼び出し側は、同期の状態の Locked や
// Configured で利用者に伝える。明示的に構成し直した設定は上書きしない。objectStoreHTTP が
// nil なら既定の HTTP client を使う（テストはここへ bucket の代わりを渡す）。
func FromVault(service *remotesync.Service, vault *secret.Service, objectStoreHTTP *http.Client) {
	if service == nil || vault == nil || !vault.Unlocked() || service.Configured() {
		return
	}
	settings, err := vault.SyncSettings()
	if err != nil || settings.Bucket == "" {
		return
	}
	direction, ok := remotesync.ParseDirection(settings.Direction)
	if !ok {
		return
	}
	config := remotesync.Config{
		Endpoint: settings.Endpoint, Bucket: settings.Bucket, Path: settings.Path,
		Region: settings.Region, Direction: direction,
	}
	credentials := remotesync.Credentials{
		AccessKeyID: settings.AccessKeyID, SecretAccessKey: settings.SecretAccessKey,
	}
	client := remotesync.NewClient(config, credentials)
	client.HTTP = objectStoreHTTP
	_, _ = service.ConfigureIfUnconfigured(config, credentials, client)
}
