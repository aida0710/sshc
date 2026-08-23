//go:build windows

package platform_test

// テストのフィクスチャが使う、このファイルシステムの絶対パスの起点。
//
// ドライブパスは、ここでは何ら特別なものではない。`C:\srv\deploy` は
// 保存された設定として `/srv/deploy` と同じ資格を持つ。
const (
	testHome              = `C:\Users\Tester`
	testAbsolute          = `C:\srv\deploy`
	testAbsoluteUncleaned = `C:\srv\\deploy\..\deploy\`
)
