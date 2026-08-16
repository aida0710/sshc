//go:build windows

package application

import "os"

// テストのフィクスチャが使う、このファイルシステムの絶対パスの起点。
const testHome = `C:\Users\Tester`

// testOutside は、ワークスペースの外にある絶対パスのファイル。外にあることを
// 確かめるフィクスチャは、このファイルシステムで絶対と認められる綴りでなければ
// 意味を持たない——絶対でなければ、ルートの下へ継ぎ足されて内側の話にすり替わる。
const testOutside = `D:\shared\ssh_config`

// shortTempBase は、100 バイトに収まる path を作れる一時ディレクトリの起点を
// 返す。Windows に /tmp は無く、確実に書ける短い場所は %TMP% だけである。
func shortTempBase() string { return os.TempDir() }
