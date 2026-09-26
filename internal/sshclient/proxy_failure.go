package sshclient

import (
	"errors"
	"fmt"
	"strings"
)

// ErrProxyAuthenticationRequired means an external proxy needs a fresh login.
// Retrying the SSH handshake cannot renew its credentials.
var ErrProxyAuthenticationRequired = errors.New("proxy authentication requires login")

func proxyFailure(err error, complaints string) error {
	cause := fmt.Errorf("%w: ProxyCommand said: %s", err, complaints)
	lower := strings.ToLower(complaints)
	expired := strings.Contains(lower, "error when retrieving token from sso:") &&
		strings.Contains(lower, "token has expired and refresh failed")
	missing := strings.Contains(lower, "error loading sso token:") &&
		(strings.Contains(lower, "does not exist") || strings.Contains(lower, "expired or is otherwise invalid"))
	if !expired && !missing {
		return cause
	}
	return &ExplainedError{
		Sentence: "AWS SSOの認証が必要です。sshcエンジンが動いているマシンの同じOSユーザーで、" +
			"ProxyCommandと同じプロファイルを指定して aws sso login --profile <プロファイル名> を実行し、再接続してください。" +
			"別のマシンのブラウザで認証する場合は --use-device-code を付けてください。",
		Err: errors.Join(ErrProxyAuthenticationRequired, cause),
	}
}
