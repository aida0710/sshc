# 組み立てた Web UI の置き場所

`npm run build`（web/）が、組み立てた画面をこの下の `app/` に書き出す。Go のバイナリは
`internal/ui/embed.go` の `//go:embed all:dist` でこのディレクトリごと埋め込み、`app/` を配る。

`app/` はリポジトリに入れない（.gitignore）。このファイルは、画面を組み立てる前でも
`go:embed` の対象が空にならず、`go build` と `go test` が通るように置いてある。画面を組み立てずに
作ったバイナリは、`/` を開くと画面が入っていないことを知らせる。

- 手元: `make build` が画面を組み立ててからバイナリを作る。画面だけなら `npm run build --prefix web`。
- CI とリリース: 画面を1回だけ組み立て、各ジョブが同じものを受け取って埋め込む（.github/workflows）。
- Homebrew: formula が Node.js で画面を組み立ててからビルドする（packaging/homebrew/sshc.rb）。
