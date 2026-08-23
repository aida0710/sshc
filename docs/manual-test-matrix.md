# 対応状況と検証記録

表には実行結果だけを記録します。ビルド成功と実行成功は区別してください。
`unverified` は「動作しない」ではなく「未検証」を意味します。

最終更新: 2026-08-23

> 2026-08-23 追記: デスクトップパッケージ（.dmg / AppImage / NSIS）は廃止しました。
> 現在は CLI のみを配布し、`sshc engine` が UI を提供します。以下のパッケージに関する
> 結果は廃止前の検証記録として残します。現在の対応状況を示すものではありません。
>
> パッケージの smoke test を削除した直後は、リリース工程に実行検査がありませんでした。
> 現在は `scripts/ci/cli-smoke.{sh,ps1}` を使用し、各 OS のリリースジョブが生成した
> バイナリを検査します。詳細は「現在のリリース検査」を参照してください。
> 記録の中に出てくる `scripts/{macos,linux,windows}/package-smoke.*` は、
> デスクトップパッケージとともに削除されています。

## 現在のリリース検査

各 OS のリリースジョブは、アップロード対象のバイナリを実行します。
`verify-artifact-name` はファイル名だけを検査するため、バイナリの内容は保証しません。
`nativebuild/machine.go` 自身が「`sshc-linux-arm64` という名前の amd64 バイナリは
その検査を通る」と説明しています。

| 確かめるもの | なぜ go test で出ないか |
| --- | --- |
| `sshc version` がタグの版を名乗る | `-X` が外れてもビルドは通る。`dev` のまま配られる |
| engine が動作していないときに起動方法を表示する | エラー文ではなく実際の利用手順を検査するため |
| `sshc engine` が handoff を生成する | listener と状態ディレクトリを実際に使用するため |
| `sshc status` が実行中の engine を報告する | CLI と engine 間の通信が必要なため |
| アクセス URL が UI を返す | `go:embed` の内容が空でもビルドは成功するため |

実行できるのはホストと同じアーキテクチャのバイナリだけです。別アーキテクチャの成果物は、
ファイル名とバイナリ形式のみを検査します。そのため、実行確認済みと記録できるのはホスト側の成果物だけです。

| OS | アーキ | リリース時に実行 |
| --- | --- | --- |
| macOS | arm64 | する（macos-14 ランナー） |
| macOS | amd64 | しない（名前と中身のアーキのみ） |
| Linux | amd64 | する（ubuntu-24.04 ランナー） |
| Linux | arm64 | しない |
| Windows | amd64 | する（windows-2025 ランナー） |
| Windows | arm64 | しない |

## 廃止前の対応表（記録）

| OS | アーキ | CLI / headless | デスクトップパッケージ | ネイティブ smoke | 備考 |
| --- | --- | --- | --- | --- | --- |
| macOS | arm64 | 確認済 | 確認済 | 通過 | CI と開発機 |
| macOS | amd64 | 確認済 | 確認済 | 翻訳越し | 実機なし。arm64 の Mac で Rosetta 2 経由 |
| Linux | amd64 | 確認済 | AppImage | リリースで実行 | 下記 |
| Linux | arm64 | 確認済 | tar.gz | 通過 | AppImage は起動しない。下記 |
| Windows | amd64 | 確認済 | 確認済 | 通過 | 実機（下記） |
| Windows | arm64 | 未検証 | ビルドのみ | 未実施 | 実機なし。対応表に載せない |
| Android | arm64 / amd64 | 該当なし | 署名済 APK | 該当なし | CLI を積まない |

「ビルドのみ」は、パッケージを生成したがインストールおよび実行は未検証であることを示します。

## 根拠

### CI（全 OS）

- 実行: https://github.com/aida0710/sshc/actions/runs/32083969991
- コミット: `7131497`
- 内容: `gofmt` / `go vet` / `go build` / `go test` / `go test -race` を
  macOS・Linux・Windows で。加えて Web の単体、Linux での e2e、
  三 OS の Desktop Node、Android の APK。

`integration` パッケージは `go test ./...` に含まれるため、所有権と Vault の
プロセス検査は 3 OS すべてで実行されています。

### macOS（開発機 arm64、2026-08-18）

`scripts/macos/package-smoke.sh` を両方の .dmg に対して実行しました。

| 確かめたもの | arm64 | x64 |
| --- | --- | --- |
| パッケージ名と配置 | 通過 | 通過 |
| 同梱 CLI のアーキ（`lipo -archs`） | `arm64` | `x86_64` |
| CLI が走り、headless を案内する | 通過 | 通過（Rosetta 2） |
| engine が上がり handoff を出す | 通過 | 通過 |
| handoff 0600 / state 0700 | 通過 | 通過 |
| 裸の `sshc` が headless を横取りしない | 通過 | 通過 |

x64 版は arm64 Mac 上の Rosetta 2 で実行しました。この結果は x86_64 バイナリの実行成功を示しますが、x64 Mac 実機での動作は保証しません。スクリプトの結果にもその制約を記録しています。

### Linux（コンテナ、aarch64、2026-08-18）

arm64 の AppImage は起動しないことを確認し、配布形式を tar.gz に変更しました。

electron-builder が arm64 向けに同梱する AppImage runtime は、バージョン番号のない
`libz.so` を要求します。これは `zlib1g-dev` が提供する開発用リンクで、通常の環境には
含まれません。標準的な Debian 環境では展開もできません。

```
error while loading shared libraries: libz.so: cannot open shared object file
```

この runtime は electron-builder が配る `appimage-12.0.1` に入っており、
26.15.3（現時点の最新）でも pin は変わっていません。`runtimeFile` の
設定項目もありません。x64 の runtime は `libz.so.1` を参照するため、この問題は発生しません。

そのため arm64 のみ `tar.gz` に変更しました。runtime を同梱しない形式なので、
この依存問題は発生しません。その場でビルドした tar.gz に対して
`scripts/linux/package-smoke.sh` を実行し、配置、同梱 CLI のアーキ、
CLI の実行、engine の起動、handoff 0600 / state 0700、引数なし `sshc` の動作を検査し、
aarch64 環境ですべて通過しました。

`ldd` は 25 個の未解決依存を報告しますが、すべて X11/GTK/NSS/音声関連です。コンテナに
デスクトップ環境がないためで、`libz` は含まれません。AppImage も同じ依存ライブラリを要求します。

x64 の AppImage はこの aarch64 環境では実行できないため、squashfs から内容だけを確認しました。
実行検査は x64 ランナーを使う Linux リリースジョブで行います。

### Windows amd64（実機）

DESKTOP-VJBBNNS / Windows 11 / 2026-08-18。すべて `sshc run` 経由で実行しました。

| 確かめたもの | 結果 |
| --- | --- |
| `go test ./...` | 34 パッケージ通過 |
| `go test -race ./...` | 34 パッケージ通過、競合なし |
| `npm run e2e --prefix web` | 92 通過 / 2 skip |
| `npm run dist:win` | x64 と arm64 のインストーラを生成 |
| `scripts/windows/package-smoke.ps1 -Architecture x64` | passed |
| パッケージ内 CLI のアーキテクチャ | x64 = `0x8664`、arm64 = `0xAA64` |

ConPTY の一連の動作を確認しました。実際の PowerShell が指定した作業ディレクトリで
起動し、コマンドを実行し、その出力が HTTP を通って xterm.js に届いた。
`0x03` を ConPTY の入力へ書けば走っている子が止まることも、`cmd.exe` と
PowerShell の両方に対して確認しました。

smoke test では、設置場所、CLI の同梱、利用者 PATH に 1 件だけ追加されること、
入れ直しで増えないこと、近い名前（`…\cli-tools`）が install でも uninstall でも
影響しないこと、HKCU の起動登録がデスクトップラッパーを参照し、自身の登録だけを削除すること、
state と handoff が DACL でこの利用者に閉じていること、headless が上がり
裸の `sshc` がそれを横取りしないこと、既存の PATH 5 件が残ること。

### Windows arm64

実機がないため、確認できたのはパッケージの内容までです。`VerifyBinaryArchitecture` は
ビルド時に PE の machine を読み、対象アーキテクチャと一致しなければ失敗します。
Windows arm64 実機での動作は未検証のため、対応表には含めません。

### 署名

署名は実施していません。macOS で初回起動する手順は `docs/release-install.md` にあります
（システム設定 → プライバシーとセキュリティ →「このまま開く」。右クリックからの「開く」は
macOS 15 以降では機能しません）。パッケージには ad-hoc 署名を付け、パッケージ内容の変更を検出します。Gatekeeper の通過を目的とした署名ではありません。
 Smart App Control が有効な Windows 11 では、署名の無い
実行ファイルは警告ではなく拒否されます。実機で確認しました（`go run` の
中間物まで拒まれ、ビルド自体ができない）。この機械は所有者の判断で SAC を
無効にして開発機として使用しました。

署名だけで実行許可が保証されるわけではありません。SAC は署名の有無だけでなく reputation も使用します。
OV の reputation はゼロから始まり、EV の即時通過は 2024 年に
撤廃されています。証明書の購入は、Windows をリリース対象に含める際に判断します。

## 手動で確認する項目

自動化していない項目は `manual-acceptance.md` にあります。この文書には結果だけを記録します。
