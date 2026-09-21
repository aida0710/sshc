package remotesync_test

import (
	"context"

	"sshc/internal/remotesync"
)

// keyOf は、テストが手に持つ passphrase を製品と同じ KeyProvider の形で渡す。
func keyOf(passphrase string) remotesync.KeyProvider {
	return func() (string, error) { return passphrase, nil }
}

// replacing は ReplaceKeyUsing に、今の鍵と保存の手続きを渡す。
func replacing(currentKey string, commit func() error) remotesync.KeyReplacementProvider {
	return func() (string, func() error, error) { return currentKey, commit, nil }
}

// applyPreview は、製品と同じく preview で見た ETag と revision を添えて適用する。
// preview から変わっていれば ErrPreviewStale で拒まれる。鍵は syncPassphrase。
func applyPreview(service *remotesync.Service, resolve remotesync.Resolution, historyKey string, preview remotesync.PullResult) error {
	return applyPreviewWithKey(service, syncPassphrase, pullChoice{resolve: resolve, historyKey: historyKey}, preview)
}

// pullChoice は preview を取ったときの選択。適用は同じ選択で取り直す。
type pullChoice struct {
	resolve    remotesync.Resolution
	historyKey string
}

func applyPreviewWithKey(service *remotesync.Service, passphrase string, choice pullChoice, preview remotesync.PullResult) error {
	_, err := service.PullAndApplyUsing(context.Background(), keyOf(passphrase), choice.resolve, choice.historyKey, preview.ETag, preview.Manifest.Revision)
	return err
}
