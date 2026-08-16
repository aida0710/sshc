//go:build !windows

package config

// テストのフィクスチャが使う、このファイルシステムの絶対パスの起点。
const (
	testHome    = "/Users/tester"
	testAltHome = "/home/u"
	testOutside = "/etc/ssh"
)
