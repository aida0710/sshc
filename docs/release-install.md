## 入れる

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
chmod +x sshc && ./sshc --version
```

### Windows

Smart App Control が有効な機械では、署名の無い実行ファイルは**警告ではなく拒否**
されます。SmartScreen は「実行しない」→「詳細情報」→「実行」で越えられます。
`docs/manual-test-matrix.md` に、署名しても単純には解決しない理由を書いています。

### Linux

AppImage には実行権が要ります（`chmod +x`）。arm64 は tar.gz です。
