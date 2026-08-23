# インストールとアップグレード

sshc は、macOS、Linux、Windows 向けの CLI バイナリと Android APK を配布しています。デスクトップアプリ、macOS の app bundle、AppImage、Windows インストーラは配布していません。

## Homebrew（macOS / Linux）

```sh
brew install aida0710/tap/sshc
```

Homebrew formula はソースからビルドするため、Go toolchain も Homebrew によってインストールされます。

## インストールスクリプト（macOS / Linux）

```sh
curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/main/install.sh | sh
```

スクリプトは次の項目を確認してからバイナリを配置します。

- OS と CPU アーキテクチャに対応するバイナリが公開されていること
- ダウンロードしたファイルの SHA-256 が `checksums.txt` と一致すること
- インストール先のディレクトリへ書き込めること
- 既存のインストール先がシンボリックリンクではないこと

ダウンロード、チェックサム、書き込み権限の確認に失敗した場合、既存の実行ファイルを変更せず終了します。通常は `~/.local/bin` にインストールし、root で実行した場合は `/usr/local/bin` を使用します。

インストール後、配置先が `PATH` に含まれていない場合や、別の `sshc` が先に解決される場合は警告と設定例を表示します。`PATH` は自動変更しません。実行中のエンジンと新しい CLI のバージョンが異なる場合も、置き換え前に警告します。

インストール先は `SSHC_INSTALL_DIR`、バージョンは `SSHC_VERSION` で変更できます。

## Windows

[GitHub Releases](https://github.com/aida0710/sshc/releases) から、x64 では `sshc-windows-amd64.exe`、Arm64 では `sshc-windows-arm64.exe` をダウンロードしてください。ファイル名を `sshc.exe` に変更し、`PATH` に含まれるディレクトリへ配置します。

sshc は Windows インストーラを提供せず、レジストリやシステムの `PATH` を変更しません。

## Android

[GitHub Releases](https://github.com/aida0710/sshc/releases) から `sshc-android-v<version>.apk` をダウンロードしてください。APK はリリース用の鍵で署名されています。

## 0.3.x からの移行

0.4.0 でデスクトップアプリの配布を終了しました。旧アプリが残っていると Homebrew formula がリンクされない場合や、`PATH` 上で古い `sshc` が優先される場合があります。

macOS で旧 cask を使用していた場合は、次の手順で削除します。

```sh
sshc version
brew uninstall --cask sshc
old="$HOME/.local/share/sshc/bin/sshc"
if [ "$(readlink "$HOME/.local/bin/sshc" 2>/dev/null)" = "$old" ]; then
  rm "$HOME/.local/bin/sshc"
fi
rm -f "$old"
rmdir "$HOME/.local/share/sshc/bin" "$HOME/.local/share/sshc" 2>/dev/null || true
brew link --overwrite sshc
```

この手順は、`~/.local/bin/sshc` が旧デスクトップアプリの管理下にあるバイナリを指す場合だけ、そのリンクを削除します。`~/.local/share/sshc` に別のファイルがある場合はディレクトリを削除しません。

移行後も古いバージョンが起動する場合は、次のコマンドで実行ファイルの場所を確認します。

```sh
command -v sshc        # macOS / Linux
where.exe sshc         # Windows
sshc version
```

## 起動

```sh
sshc engine      # フォアグラウンドでエンジンを起動
sshc             # URL を表示し、可能であればブラウザで開く
```

`sshc engine` はデーモン化しません。常駐させる場合は tmux、screen、systemd、launchd などを使用してください。

```sh
tmux new -d -s sshc 'sshc engine'
```

初回起動時は vault を作成してロックを解除します。

```sh
sshc vault create
sshc vault unlock
```

同じ操作は Web UI からも実行できます。CLI は対話端末からのみマスターパスワードを読み取り、コマンドライン引数や環境変数からは受け取りません。

vault は 12 時間操作がない場合に自動的にロックされます。

## バージョン不一致

CLI と実行中のエンジンのバージョンが異なる場合、sshc は接続せず、現在使用している実行ファイルのパスを表示します。エンジンを停止し、更新後の `sshc engine` で起動し直してください。
