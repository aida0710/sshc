//go:build !windows

package platform_test

// テストのフィクスチャが使う、このファイルシステムの絶対パスの起点。
const (
	testHome              = "/home/tester"
	testAbsolute          = "/srv/deploy"
	testAbsoluteUncleaned = "/srv//deploy/../deploy/"
)
