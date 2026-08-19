//go:build windows

package application

// テストのフィクスチャが使う、このファイルシステムの絶対パスの起点。
const testHome = `C:\Users\Tester`

// testOutside は、ワークスペースの外にある絶対パスのファイル。外にあることを
// 確かめるフィクスチャは、このファイルシステムで絶対と認められる綴りでなければ
// 意味を持たない——絶対でなければ、ルートの下へ継ぎ足されて内側の話にすり替わる。
const testOutside = `D:\shared\ssh_config`
