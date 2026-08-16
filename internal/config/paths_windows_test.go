//go:build windows

package config

// テストのフィクスチャが使う、このファイルシステムの絶対パスの起点。
const (
	testHome    = `C:\Users\Tester`
	testAltHome = `C:\home\u`
	testOutside = `C:\ProgramData\ssh`
)
