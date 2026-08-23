# sshc

`~/.ssh/config` を壊さずに読み書きする、SSH の設定と接続のための道具です。

**エンジンひとつと、ブラウザで開く画面ひとつ。** `sshc engine` が前面で走って
HTTP で画面を配り、`sshc` がその入口を刷ります。設定ファイルは OpenSSH のもの
そのままで、このアプリケーションが独自の形式へ移すことはありません。

## 入れる

```sh
brew install aida0710/tap/sshc
```

または[リリース](https://github.com/aida0710/sshc/releases)から自分の OS の
実体を落として、PATH の通った場所へ置きます。

```sh
curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/main/install.sh | sh
```

**配るのは CLI ひとつです。** 署名も公証もインストーラも要りません。詳しくは
[docs/release-install.md](docs/release-install.md)。

**0.3.x から上げる人は、旧アプリを外してください** ——残っていると Homebrew は
新しい実体を張らず、古い方が走り続けます（[上げ方](docs/release-install.md#03x-から上げる)）。

Android は[リリース](https://github.com/aida0710/sshc/releases)の APK です。

## 使う

```sh
sshc engine      # エンジンを前面で起こす。この端末は開けたままにする
sshc             # 別の端末から。入口を刷り、画面があればブラウザで開く
```

**エンジンを生かしておくのは人です。** このアプリケーションは detach しません。

```sh
tmux new -d -s sshc 'sshc engine'
```

初回は保管庫がありません。エンジンがそう言うので作ります。

```sh
sshc vault create    # マスターパスワードは端末からしか受け取りません
sshc vault unlock
```

端末から直接使うこともできます。

```sh
sshc <接続先>              # 保存済みの答えを使って繋ぐ
sshc run <接続先> <コマンド>  # 一度だけ走らせて、出力をそのまま返す
sshc connect              # 一覧から選んで繋ぐ
sshc list                 # Host の別名を並べる
sshc status               # エンジンの状態を表で（--json で機械向け）
```

**裸の `sshc` は、走っているものへの入口を刷るだけで、エンジンは起こしません。**
`sshc engine --replace` で、走っているエンジンを止めて入れ替えられます。

ログイン時起動は OS に任せています。unit もスケジュールタスクも作りません
——`tmux`、`systemd`（ユーザー単位）、`launchd` のどれでも、前面のプロセスとして
扱えます。

## できること

- **設定は OpenSSH のまま。** 無損失のパーサで読み書きするので、コメントも並び順も
  空白も保たれます。編集は 3 者マージで、外から変えられていれば衝突として見せます
- **埋め込みターミナル。** ブラウザの中で SSH を話します（ポート転送・agent 転送・
  未知ホストの確認つき）。回線が切れたら繋ぎ直しに行きます（回数は設定で、0 なら
  繋ぎ直しません）。`ProxyJump` と `ProxyCommand` のどちらも通ります
- **接続のログ。** `ssh -v` と同じものを、コンソールそのものへ書きます。深さは 4 段
- **見た目。** 配色 6 種・同梱の JetBrains Mono・背景画像を、接続ごとにも全体にも
- **鍵の管理。** 生成・パスフレーズの変更・agent への登録・リモートの `authorized_keys` へ登録
- **パスワードの保管庫。** マスターパスワードで封じ、**12 時間**触れられなければ
  自分で閉じます。値はどの API も返しません
- **S3 互換のバックアップ。** 封じたスナップショットを置き、別の端末へ引き取れます

なぜそうしたか、何を引き受けないかは [docs/design.md](docs/design.md) にあります。

## 開発

必要なもの: Go 1.26 / Node 22。

```sh
make build       # web を焼き、bin/sshc を作る
make test        # Go と web の全スイート
make e2e         # 実バイナリに対する Playwright
make integration # 実 sshd を相手にした統合（Docker）
make generate    # api/openapi.yaml から Go と TypeScript を作り直す
```

**`internal/ui/dist` はコミットしてあります。** バイナリがそれを焼き込むので、
web を直したら `make build` を通してからコミットしてください。CI が食い違いを
落とします。

Android の AAR は `make android-bind`（NDK が要ります）。

- [docs/design.md](docs/design.md) — なぜそうしたか
- [docs/manual-acceptance.md](docs/manual-acceptance.md) — 機械で確かめられないもの
- [docs/headless-examples.md](docs/headless-examples.md) — サーバーで走らせる

## ライセンス

MIT
