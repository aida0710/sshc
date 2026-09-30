package main

import (
	"errors"

	"sshc/internal/sshclient"
)

// errProxyLoginRequired は、ProxyCommand の AWS SSO の認証が切れたことを CLI の英語で言う。
//
// sshclient が付ける日本語の案内（sshclient.ExplainedError）は、sshcエンジンの
// ターミナルの接続ログ用である。CLI の接続では ProxyCommand をこのプロセスが起動元の
// 環境で実行するので、同じマシンの同じ OS ユーザーで login し直せば足りる。
var errProxyLoginRequired = errors.New("AWS SSO login is required. Run aws sso login --profile <profile> " +
	"with the profile your ProxyCommand uses, then connect again. " +
	"Add --use-device-code to sign in from a browser on another machine.")

// describeConnectionFailure は、SSH 接続の失敗を CLI の英語の文で返す。
// 英語の文を別に持つ失敗だけを置き換え、それ以外はそのまま返す。
func describeConnectionFailure(err error) error {
	if errors.Is(err, sshclient.ErrProxyAuthenticationRequired) {
		return errProxyLoginRequired
	}
	return err
}
