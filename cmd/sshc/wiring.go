package main

import (
	"sshc/internal/platform"
)

// platformParts は、このプラットフォームの部品一式である。
//
// 組み立てを GOOS ごとのファイルへ分けてあるのは、その OS の部品のパッケージ
// （internal/platform/macos、linux、windows のどれか）だけをバイナリに入れるため
// である。macos と linux のパッケージはそれぞれの OS でしかビルドされないので、
// 実行時に runtime.GOOS で分岐する書き方はそもそも取れない。
type platformParts struct {
	Toolchain platform.Toolchain
	KeyAgent  platform.KeyAgent
}
