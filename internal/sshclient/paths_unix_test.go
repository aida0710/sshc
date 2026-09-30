//go:build !windows

package sshclient_test

// テストのフィクスチャが使う、このファイルシステムの絶対パスの起点。
const testHome = "/Users/tester"

// ホームの外にある鍵。この表記が絶対でなければ検査が消える。解決器の結果に
// 現れる絶対パスはそのまま使われ、~ で始まるものだけがホームへ継ぎ足される。
const testOutsideKey = "/etc/keys/second"
