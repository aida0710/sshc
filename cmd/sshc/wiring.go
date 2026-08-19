package main

import (
	"sshc/internal/platform"
	"sshc/internal/secret"
)

// platformParts は、このプラットフォームの部品一式である。
//
// 組み立てを GOOS ごとのファイルへ分けてあるのは、macOS のバイナリに Linux の
// コードが、Linux のバイナリに AppleScript の定数が入らないようにするためである。
// 実行時に runtime.GOOS で分岐すれば両方が入る。何が出荷物に入るかは、この
// アプリケーションが気にしてきたことである。
type platformParts struct {
	Toolchain platform.Toolchain
	KeyAgent  platform.KeyAgent
	// Biometric は、この OS の錠前である。**持たない OS では nil のままにする。**
	// 画面はそれを「この端末では使えない」として出す——守れないものを守れる
	// ふりをしない、がこの機能の設計の第一行である。
	Biometric secret.Guardian
}
