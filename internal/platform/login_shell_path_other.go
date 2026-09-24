//go:build windows || android || ios

package platform

import "context"

// WithLoginShellPathは、Unixのログインシェルを使わないOSでは起動元の環境を保つ。
func WithLoginShellPath(_ context.Context, environment []string) ([]string, error) {
	return environment, nil
}
