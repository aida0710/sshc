# 対応表と、その根拠

**升は、走らせた記録だけで埋める。** ビルドが通ったことは、動いたことではない。
`unverified` は「動かない」ではなく「**確かめていない**」であり、その二つを
同じ言葉で書かない。

最終更新: 2026-08-18

## 対応表

| OS | アーキ | CLI / headless | デスクトップ束 | ネイティブ smoke | 備考 |
| --- | --- | --- | --- | --- | --- |
| macOS | arm64 | 確認済 | 確認済 | **通過** | CI と開発機 |
| macOS | amd64 | 確認済 | 確認済 | 翻訳越し | 実機なし。arm64 の Mac で Rosetta 2 経由 |
| Linux | amd64 | 確認済 | ビルドのみ | 未実施 | CI。arm64 の機械では実行できない |
| Linux | arm64 | 確認済 | **起動しない** | 通過（要 zlib1g-dev） | 下記の runtime の問題 |
| Windows | amd64 | 確認済 | 確認済 | **通過** | 実機（下記） |
| Windows | arm64 | 未検証 | ビルドのみ | 未実施 | **実機なし。対応表に載せない** |
| Android | arm64 / amd64 | — | — | — | CLI を積まない。APK のビルドのみ |

「ビルドのみ」は、**束は出来るが、入れて動かしていない**という意味である。

## 根拠

### CI（全 OS）

- 実行: https://github.com/aida0710/sshc/actions/runs/32083969991
- コミット: `7131497`
- 内容: `gofmt` / `go vet` / `go build` / `go test` / `go test -race` を
  macOS・Linux・Windows で。加えて Web の単体、Linux での e2e、
  三 OS の Desktop Node、Android の APK。

`integration` パッケージは `go test ./...` に含まれるので、**所有権と Vault の
プロセス検査は三 OS すべてで走っている。**

### macOS（開発機 arm64、2026-08-18）

`scripts/macos/package-smoke.sh` を両方の .dmg に対して実行した。

| 確かめたもの | arm64 | x64 |
| --- | --- | --- |
| 束の名前と配置 | 通過 | 通過 |
| 同梱 CLI のアーキ（`lipo -archs`） | `arm64` | `x86_64` |
| CLI が走り、headless を案内する | 通過 | 通過（Rosetta 2） |
| engine が上がり handoff を出す | 通過 | 通過 |
| handoff 0600 / state 0700 | 通過 | 通過 |
| 裸の `sshc` が headless を横取りしない | 通過 | 通過 |

**x64 は翻訳越しである。** arm64 の Mac は Rosetta 2 で x86_64 を走らせるので、
動いたことは本当だが、**x64 の Mac で動く証明にはならない**。スクリプト自身が
結果にそう書く。

### Linux（コンテナ、aarch64、2026-08-18）

`scripts/linux/package-smoke.sh` を、その場でビルドした arm64 の AppImage に
対して実行した。配置、同梱 CLI のアーキ、CLI の実行、engine の起動、
handoff 0600 / state 0700、裸の `sshc` の拒否——すべて通過。

**ただし、そのままでは起動しない。**

electron-builder が arm64 向けに同梱する AppImage の runtime は、**版の付か
ない `libz.so`** を要求する。これは `zlib1g-dev` が提供する開発用のリンクで、
普通の機械には入っていない。素の Debian では:

```
error while loading shared libraries: libz.so: cannot open shared object file
```

**利用者はこの AppImage を起動できない。** 上のスモークは `zlib1g-dev` を
入れた環境で通したものである。

x64 の runtime は `libz.so.1`（誰の機械にもある方）を参照しており、この問題を
持たない。**arm64 固有である。** Linux のデスクトップ束を配るときに、
runtime を差し替えるか arm64 を外すかを決める必要がある。

x64 の実行は arm64 のこの機械では確かめられない（翻訳の手段が無い）。

### Windows amd64（実機）

DESKTOP-VJBBNNS / Windows 11 / 2026-08-18。すべて `sshc run` 越しに実行した。

| 確かめたもの | 結果 |
| --- | --- |
| `go test ./...` | 34 パッケージ緑 |
| `go test -race ./...` | 34 パッケージ緑、競合なし |
| `npm run e2e --prefix web` | 92 通過 / 2 skip |
| `npm run dist:win` | x64 と arm64 のインストーラを生成 |
| `scripts/windows/package-smoke.ps1 -Architecture x64` | **passed** |
| 束の中の CLI のアーキ | x64 = `0x8664`、arm64 = `0xAA64` |

**ConPTY を端から端まで見た。** 本物の PowerShell が指定の作業ディレクトリで
起動し、コマンドを実行し、その出力が HTTP を通って xterm.js に届いた。
`0x03` を ConPTY の入力へ書けば走っている子が止まることも、`cmd.exe` と
PowerShell の両方に対して確認した。

smoke が通した内容: 設置場所、CLI の同梱、利用者 PATH に 1 件だけ、
入れ直しで増えないこと、近い名前（`…\cli-tools`）が install でも uninstall でも
巻き添えにならないこと、HKCU の起動登録が外殻を指し自分のものだけ消えること、
state と handoff が DACL でこの利用者に閉じていること、headless が上がり
裸の `sshc` がそれを横取りしないこと、既存の PATH 5 件が残ること。

### Windows arm64

**実機が無い。** 言えるのは束の中身までで、`VerifyBinaryArchitecture` が
ビルドのたびに PE の machine を読み、行き先と食い違えば止める。**そこで動く
ことは言えない。** 対応表には載せない。

### 署名

**していない。** Smart App Control が有効な Windows 11 では、署名の無い
実行ファイルは**警告ではなく拒否**される。実機で確認した（`go run` の
中間物まで拒まれ、ビルド自体ができない）。この機械は所有者の判断で SAC を
無効にして開発機にした。

署名すれば通る、という単純な話でもない。SAC が見ているのは署名の有無ではなく
積み上がった評判であり、OV はゼロから始まり、EV の即時通過は 2024 年に
撤廃されている。**買う判断は、Windows をリリース対象に入れるときのものである。**

## 手で確かめるもの

自動化していない項目は `manual-acceptance.md` にある。ここには**結果だけ**を
書く。
