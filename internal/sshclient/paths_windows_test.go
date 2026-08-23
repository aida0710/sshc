//go:build windows

package sshclient_test

// テストのフィクスチャが使う、このファイルシステムの絶対パスの起点。
const testHome = `C:\Users\Tester`

// ホームの外にある鍵。この表記が絶対でなければ検査が消える。解決器の結果に
// 現れる絶対パスはそのまま使われ、絶対でないものだけがホームへ継ぎ足される。
// 外の鍵が継ぎ足される側へ落ちれば、区別しているつもりの二つが同じ経路を通る。
const testOutsideKey = `D:\shared\keys\second`
