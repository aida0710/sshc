//go:build windows

package main

import (
	"os"

	"sshc/internal/keys"
)

// newPlatformParts は、この OS の部品を組み立てる。
//
// **ダミーを置かない。** Toolchain も KeyAgent も interface であり、nil は
// このアプリケーションが既に扱いを決めている状態である——nil の Toolchain は
// 鍵の一覧が ssh-keygen の有無を尋ねない、というだけで、致命ではない。本物の
// Windows toolchain はその task が入れる。**それまでのあいだ、偽物を置いて
// 動いているふりをしない。**
//
// KeyAgent は繋がった。Windows の OpenSSH エージェントは固定の named pipe を
// 待っており、そこへ話すのは keys.NewAgent である。lookup を渡すのは、Unix と
// 同じ signature を保つためだけで、**あちらはそれを読まない。**
func newPlatformParts() platformParts {
	return platformParts{KeyAgent: keys.NewAgent(os.LookupEnv)}
}
