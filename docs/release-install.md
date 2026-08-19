## 入れる

### Homebrew（macOS / Linux）

```sh
brew install aida0710/sshc/sshc
```

**ソースからビルドします。** brew が Go を用意して `go build` を回すので、
入るのはあなたの機械で作られた実体です。置かれるのは brew の prefix
（`/opt/homebrew/bin` など）で、**そこは最初から PATH に載っています。**

`brew upgrade` で新しくなり、`brew uninstall` で消えます。

### 端末から使うだけなら（Linux / macOS）

```sh
curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/main/install.sh | sh
```

**入るのは CLI ひとつだけです。** アプリは下の各 OS の項を見てください。

この script は置く前に確かめ、**見たものを全部印字します**——機械に合う実体があるか、
落としたものが公開された checksum と一致するか、置き先が PATH に載っているか、
そこに既に居るものが自分の置いたものか、PATH の手前に別の `sshc` が居ないか、
走っている engine と版が食い違わないか。**一致しないものは置かずに止まります。**

`~/.local/bin` へ入れます（root で走らせたなら `/usr/local/bin`）。**PATH を勝手に
書き換えることはしません** ——載っていなければ、足す 1 行をそのまま綴ります。
置き先は `SSHC_INSTALL_DIR`、版は `SSHC_VERSION` で変えられます。

入ったら:

```sh
sshc version     # sshc 0.1.0 darwin/arm64
```


配布物には**署名も公証もありません**。Apple の Developer ID は年 $99 の登録が要り、
このプロジェクトはまだそれを買っていません。**その結果として何が起きるか**を、
起きる前にここに書きます。

### macOS

Apple Silicon なら `-mac-arm64.dmg`、Intel なら `-mac-x64.dmg` です。

初回に「"sshc.app" に…マルウェアが含まれていないことを検証できませんでした」が
出ます。**右クリック →「開く」は macOS 15 以降では効きません。** 次のどちらかを
一度だけ行ってください。

1. その画面を閉じ、**システム設定 → プライバシーとセキュリティ** を開く。下の方に
   同じ文が出ているので、隣の **「このまま開く」**。
2. または端末から:

   ```sh
   xattr -dr com.apple.quarantine /Applications/sshc.app
   ```

**この画面が出るのはブラウザで落としたときだけです。** 隔離の印を付けるのは
ダウンロードしたアプリであって、macOS ではありません。CLI だけで足りるなら、
`curl` は何も付けないので何も出ません:

```sh
curl -L -o sshc https://github.com/aida0710/sshc/releases/latest/download/sshc-darwin-arm64
chmod +x sshc && ./sshc version
```

**上の install.sh は、これに checksum の照合と置き場所の確認を足したものです。**

### Windows

Smart App Control が有効な機械では、署名の無い実行ファイルは**警告ではなく拒否**
されます。SmartScreen は「実行しない」→「詳細情報」→「実行」で越えられます。
`docs/manual-test-matrix.md` に、署名しても単純には解決しない理由を書いています。

### Linux

AppImage には実行権が要ります（`chmod +x`）。arm64 は tar.gz です。
