//go:build windows

package main

// newPlatformParts は、まだ何も配線されていないことを述べる。
//
// **ダミーを置かない。** Toolchain も KeyAgent も interface であり、nil は
// このアプリケーションが既に扱いを決めている状態である——nil の KeyAgent は
// 「届くエージェントが無い」と報告し、nil の Toolchain は鍵の一覧が
// ssh-keygen の有無を尋ねない、というだけである。どちらも致命ではない。
//
// 本物の Windows toolchain と named-pipe key agent は、それぞれの task が
// 入れる。**それまでのあいだ、偽物を置いて動いているふりをしない。**
func newPlatformParts() platformParts { return platformParts{} }
