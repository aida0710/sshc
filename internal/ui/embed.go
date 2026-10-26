package ui

import (
	"embed"
	"io/fs"
)

// dist は、web のビルドが組み立てた画面を app/ に書き出す場所である（web/vite.config.ts）。
// app/ はリポジトリに入れず、dist/README.md だけを追跡している。画面を組み立てる前でも
// 埋め込む対象が空にならず、コンパイルが通る。そのとき FS の中に index.html は無く、
// httpserver が画面の入っていないことを知らせる。
//
//go:embed all:dist
var assets embed.FS

func FS() (fs.FS, error) {
	return fs.Sub(assets, "dist/app")
}
