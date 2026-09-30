// Package secrettest は、ほかのパッケージのテストが解錠中の vault へ前準備の値を
// 書くための関数を置く。
//
// どれも本番の書き込みと同じ変更（secret.PasswordMutation など）と同じ書き方
// （CommitAtomic）を通す。テストのためだけの書き口を secret.Service に持たせると、
// 設定ファイルと vault の片方だけを変える経路が本番の公開 API に残るからである。
package secrettest

import (
	"sshc/internal/secret"
	"sshc/internal/storage"
)

// storeDedicatedPasswordOperation は、前準備の書き込みを履歴で見分けるための Operation。
const storeDedicatedPasswordOperation = "test.dedicated-password"

// DedicatedPassword は、alias 専用のパスワードと、それを渡してよい認証先である。
type DedicatedPassword struct {
	Alias    string
	Password string
	// Binding は、alias を解決した認証先の digest（64 桁の 16 進数）である。
	Binding string
}

// StoreDedicatedPassword は、接続の保存と同じ変更（PasswordMutationDedicated）で、
// 専用のパスワードと認証先を vault に書く。
//
// transactions には secrets を組んだときの storage.Manager を渡す。別の Manager では、
// 世代バックアップの封じ方（Seal）が secrets の配線と食い違う。
func StoreDedicatedPassword(secrets *secret.Service, transactions *storage.Manager, password DedicatedPassword) error {
	_, err := secrets.WithPasswordMutation(secret.PasswordMutation{
		Kind:     secret.PasswordMutationDedicated,
		Alias:    password.Alias,
		Password: password.Password,
		Binding:  password.Binding,
	}, func(change storage.Change) (storage.Result, error) {
		return transactions.CommitAtomic(storage.Request{
			Operation: storeDedicatedPasswordOperation,
			Changes:   []storage.Change{change},
		})
	})
	return err
}
