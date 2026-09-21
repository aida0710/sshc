//go:build windows || android || ios

package platform

import "context"

// ProxyEnvironmentは、Unixのログインシェルを使わないOSでは起動元の環境を保つ。
func ProxyEnvironment(_ context.Context, environment []string) ([]string, error) {
	return environment, nil
}
