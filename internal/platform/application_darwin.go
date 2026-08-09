//go:build darwin

package platform

import "path/filepath"

// validApplicationPath は、開く先が macOS のアプリケーションバンドルかを答える。
//
// macOS で端末を開くのは Launch Services であり、それが受け取るのはバンドルで
// ある。保存の時点でこれを要求するのは、起動の時点まで持ち越せば、設定した人が
// 間違いに気づくのが「開こうとしたとき」になるからだ。
func validApplicationPath(path string) bool {
	return filepath.Ext(path) == ".app"
}
