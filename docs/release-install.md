## 入れる

**配るのは CLI ひとつです。** アプリの束（.app / .dmg / AppImage / インストーラ）は
もうありません。署名も公証も要らないので、**Gatekeeper の警告を通る一手間もありません**
——隔離の印を付けるのはブラウザ経由のダウンロードであり、`curl` で落とした実行体は
何も言われずに動きます。

画面付きのアプリは Android と iOS だけです。あちらは配布の口がストアしかありません。

### Homebrew（macOS / Linux）

```sh
brew install aida0710/tap/sshc
```

**formula はソースから建てます。** Go の toolchain は Homebrew が用意します。

### そのほか（Linux / macOS）

```sh
curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/main/install.sh | sh
```

この script は置く前に確かめ、**見たものを全部印字します**——機械に合う実体があるか、
落としたものが公開された checksum と一致するか、置き先が PATH に載っているか、
そこに既に居るものが自分の置いたものか、PATH の手前に別の `sshc` が居ないか、
走っている engine と版が食い違わないか。**一致しないものは置かずに止まります。**

`~/.local/bin` へ入れます（root で走らせたなら `/usr/local/bin`）。**PATH を勝手に
書き換えることはしません** ——載っていなければ、足す 1 行をそのまま綴ります。
置き先は `SSHC_INSTALL_DIR`、版は `SSHC_VERSION` で変えられます。

### Windows

リリースから `sshc-windows-<アーキ>.exe` を落とし、`sshc.exe` に改名して PATH の
通った場所へ置きます。**インストーラはありません。** レジストリにも machine の PATH にも
触れません。

## 使う

```sh
sshc engine      # エンジンを前面で起こす。この端末は開けたままにする
sshc             # 別の端末から。入口を刷り、画面があればブラウザで開く
```

**エンジンを生かしておくのは人です。** tmux でも screen でも systemd でも構いません
——このアプリケーションは detach しないので、supervisor 側で扱いを変える必要が
ありません。

```sh
tmux new -d -s sshc 'sshc engine'
```

初回は保管庫がありません。エンジンがそう言うので、`sshc vault create` で作ります。

```sh
sshc vault create    # 端末からしか受け取りません
sshc vault unlock    # 12 時間触れられなければ自分で閉じます
```

## 版が食い違ったとき

`sshc` と、走っているエンジンの版が違うと、繋がる前に断ります。**いま走っているのが
どの実体かを綴りで名指しします** ——古いのがどちらかは、このプロセスには分からない
ためです。エンジンを止めて、新しい方で起こし直してください。
