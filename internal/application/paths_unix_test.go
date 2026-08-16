//go:build !windows

package application

// テストのフィクスチャが使う、このファイルシステムの絶対パスの起点。
const testHome = "/Users/tester"

// testOutside は、ワークスペースの外にある絶対パスのファイル。外にあることを
// 確かめるフィクスチャは、このファイルシステムで絶対と認められる綴りでなければ
// 意味を持たない——絶対でなければ、ルートの下へ継ぎ足されて内側の話にすり替わる。
const testOutside = "/etc/ssh/ssh_config"

// shortTempBase は、100 バイトに収まる path を作れる一時ディレクトリの起点を
// 返す。TMPDIR は macOS では 49 文字あり、EvalSymlinks が /private を足すので、
// そこを起点にすると鍵の名前ひとつで境界を越える。
func shortTempBase() string { return "/tmp" }
