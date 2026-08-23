//go:build windows

package main

// cmd 経由で start を呼ぶ。start は実行可能ファイルではなく cmd の組み込みである。
func browserCommand(url string) (string, []string) {
	return "cmd", []string{"/c", "start", "", url}
}
