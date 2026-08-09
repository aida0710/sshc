//go:build linux

package platform

// validApplicationPath は、開く先の形について Linux で言えることを答える。
//
// Linux の端末は実行ファイルそのものであり、バンドルという約束はない。だから
// 拡張子について言えることは何もなく、形の検査は共有側の「絶対パスであること」
// と「Clean と一致すること」で尽きている。そこにあるか、実行できるかは起動する
// 側が見る。ここでディスクを見に行かないのは、アンインストールしただけで設定が
// 保存できなくなるのを避けるためで、これは macOS 側と同じ判断である。
func validApplicationPath(string) bool {
	return true
}
