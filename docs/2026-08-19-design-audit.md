# sshc 設計監査 — 2026-08-19

13 日・735 コミット・43 本の実装計画で 4 度の方針転換を経たコードベースを、15 エージェントの並列調査で監査した記録。

調査本体は読解と `rg` による静的追跡で行い、**その後 Go 1.26.6 / Node 22.19.0 / npm 11.7.0 を導入して主要な主張を実際に検証した**（結果は §0.5）。
個別の指摘に付いた confidence（confirmed / likely / speculative）は静的追跡時点のもので、実行検証を経ているのは §0.5 に挙げた項目に限る。

## 0. 要約

**前提「設計が破綻している」は、半分当たっている。**

外れている側から先に書く。依存グラフに**循環は 1 件も無い**（36 パッケージ・106 辺を DFS で検証）。
OS 分岐のコピペも**ほぼ無い** — `vault_terminal_*`・`ownership` の watch 本体・`platform/shell_*`・`nativepath/fold_*` はいずれも共有行が 3〜11 行しかなく、統合すればプラットフォーム差が確実に潰れる。
真に参照 0 の識別子はエクスポート 1693 個中 67 個（うち 49 は生成物）、非エクスポート 1112 個中 3 個で、**コード全体の衛生状態は悪くない**。

当たっている側。破綻は「散らかり」ではなく、**3 つの決まった形**で残っている。

1. **削除だけ済んで統合が残った。** 旧実装と新実装が並走し、しかも答えが食い違う。
2. **抽象が先にでき、後から足した呼び出し側がそれを使わなかった。** 「新しい作法が旧作法を置き換えた」のではなく「隣に増えた」。
3. **「機能の有無」が型で表されていない。** `nil` が「注入し忘れ」と「その OS に無い」を区別できず、どちらもコンパイルも実行も静かに成功する。

この 3 番目から、**ユーザーに見える実害が既に 2 件出ている。**

## 0.5 実行による検証

固定版のツールチェーン（Go 1.26.6 / Node 22.19.0 / npm 11.7.0）を導入して実際に走らせた結果。

| 検証 | 結果 |
|---|---|
| `go build ./...` / `go vet ./...` | 通過 |
| `go test ./...` | **失敗 1 件**（下記） |
| `go test -race ./...` | 通過。データ競合なし |
| `npm test --prefix web` | 通過（73 ファイル / 688 テスト） |
| `npm run typecheck --prefix web` | 通過 |
| `npm test --prefix desktop` | 通過（41 テスト） |
| `make verify-generated` | 通過。生成物は契約と一致 |
| クロスビルド linux/{amd64,arm64}、windows/{amd64,arm64}、android/arm64 | 通過 |
| クロスビルド darwin/{amd64,arm64} | `CGO_ENABLED=0` では失敗。`biometric_darwin.go:11` が `import "C"` するため cgo 無効時に `macos.Biometric` が消え `wiring_darwin.go:16` が未定義になる。**macOS 上のネイティブビルド（cgo 既定 on）では問題なく、リリースは OS ごとのネイティブジョブなので実害は無い。** ただし Android と違い darwin のタグ整合を CI で検査する経路は無い |

### 機械検証できた監査の主張

`golang.org/x/tools/cmd/deadcode` を本番バイナリ（`./cmd/sshc`）に対して実行し、dead code の指摘を裏付けた。

- **C1** — `internal/platform/process/command.go` の `NewOutputRunner` / `OutputRunner.RunOutput` / `boundedBuffer.*` が到達不能。**確定**
- **C2** — `cmd/sshc/launch_linux.go:126 launchBackground` が到達不能。**確定**。さらに `GOOS={freebsd,openbsd,netbsd} go build ./cmd/sshc` は `engine.go:171: undefined: newPlatformParts` で失敗し、`launch_unsupported.go` がどの GOOS でもコンパイルできないという主張も**確定**
- **実害2 / C11** — `GOOS=windows` で `internal/platform/windows.NewToolchain` と `Toolchain.KeyGen` が到達不能。**確定**
- **C5** — `windowsregistry.RegisterDesktopExecutable` / `RemoveDesktopExecutable`、`windowsacl.RestrictFile` / `IsRestrictedToCurrentUser` が到達不能。**確定**
- **C4 ほか** — `handoff.readValidated`、`effective.Cumulative`、`effective.ParseValues`、`secret` の `Has` / `Rename` / `AssignedCredential` / `HasKeyPassphrase` / `Vault.RemoveKeyPassphrase` / `Vault.Rename`、`terminal` の `Prune` / `Ring.Cap` / `Ring.Len` が到達不能。**確定**
- 監査が挙げていなかった到達不能: `internal/app/run.go:165 Build`、`internal/httpserver/server.go:245 Server.Routes`、`internal/remotesync/service.go:486 Service.Check`、`internal/secret/service.go:322 SetSleep`、`terminal` の `Session.Kind` / `Alias` / `Title` / `Snapshot`

**実害1** — `.Seal =` の本番代入は `internal/app/run.go:265` の 1 箇所のみ。一方 `storage.NewManager(` の本番呼び出しは `app/run.go:142`、`app/run.go:204`、`app/ssh.go:215` の 3 箇所。**3 つのうち 2 つに封が無いことを確定**。

**実害3** — `effective.Project(` の本番呼び出しは 7 箇所（`passwordeligibility.go:82`、`connectionupdate.go:406,442`、`tui.go:51`、`diagnostics/service.go:113,170,187`）、`effective.Resolve(` は 2 箇所（`application/effective.go:67,281`）。並走を確定。監査は「9 箇所」と書いたが実測は 7 箇所（`jump.go` の 2 件は同パッケージ内呼び出しで `effective.` 修飾が付かない）。結論は変わらない。

**C20 / 生成モデル** — `models.gen.go` の生成型 203 個のうち、**本番コードから到達できないのは 84 個（41%）**。内訳は「テストからのみ参照」8 個、「どこからも参照なし」76 個。監査の「77 型」より多い。

### 実行して初めて見つかったもの

静的解析では出せなかった不具合を 1 件検出した。**§1 の表の 0 番**として追加した。

## 0.7 実施した修正（branch: design-repair）

本書の診断のうち、**利用者の判断を要さないものを実装した。** 各段階で `go test ./...` を通し、最後に全スイートを再実行している。

### 実害の修正

| # | 内容 | 検証 |
|---|---|---|
| 0 | 履歴の並び順を `StartedAt`（ナノ秒）で決め、識別子は同時刻のタイブレークにのみ使う。`readRecords` の「辞書順は時系列順」という前提コメントも実態に直した | 該当テスト **30回中19回失敗 → 0回** |
| 1 | `buildKeyService` が `storage.Manager` を呼び出し元へ返すようにし、鍵 vault のマネージャにも `Seal`/`Unseal` を差す。**パスフレーズ変更が平文の秘密鍵をバックアップに残さなくなった** | `Seal` 代入がワークスペース上の全 Manager を覆う |
| 2 | `cmd/sshc/wiring_windows.go` が `windows.NewToolchain(systemRoot())` を配線。`%SystemRoot%`（fallback `%windir%`）を信頼の起点として渡す | `GOOS=windows deadcode` で `NewToolchain`/`KeyGen` が到達可能へ |
| 3 | 資格判定を `effective.Project` から `effective.Resolve` へ移した。**`Match host X` 配下の `PasswordAuthentication no` と `HostName` が報告に載るようになった。** 既定値を拾わないよう、参照するのは `Values` ではなく `Accepted` | 回帰テスト3本を追加。**修正前に落ち、修正後に通ることを確認済み** |
| 4 | `Probe` に `HostKeyAlgorithms` を渡し、認証テストが実接続と同じ鍵種別を名乗るようにした | — |
| 5 | `sshclient.Session` に `ForceClose` を実装。**リモートのチャンネルより先に輸送を断つ**（順序がこのメソッドの全部で、応答を返さない相手ではチャンネルを閉じる書き込み自体が返らない） | `forceCloser` を満たすことをコンパイルで確認 |
| 6 | `mobile/dependencies.go` の「落としているのは4つ」を実態（6つ）に直し、`Biometric` を明示。加えて **`app.Dependencies` の全項目に「配線する／意図して空にする／既定に落とす」の分類を要求するテスト**を追加した。項目が増えれば Android 側が必ず選択を迫られる | 新テスト通過 |

### S1（削除・振る舞いを変えない）

- **C1** 外部プロセス実行の継ぎ目一式（`platform.OutputRunner`/`Command`/`Output`、`process/command.go`）。`internal/platform/command.go` は `Toolchain` だけを残して `toolchain.go` へ。**あわせて `internal/keys` の3つのテストが「子プロセスを起こしていない」と主張しながら、runner が Service に配線されていないため何も表明していなかったことが判明した** —— その性質は `internal/acceptance` の allowlist 走査が本当に守っているので、空振りの表明は削除した
- **C2** `launchBackground`（4 OS 分・呼び出し0）、`launch_unsupported.go`（どの GOOS でもコンパイルできず保険になっていない）、および到達不能になっていた Electron の `--hidden` 起動モード
- **C4** 参照0の小物（`sshFinder` / `handoff.Random` / `readValidated` / `nativeFileSystem` / `DefaultConnectTimeout` / `remotekey.DefaultTimeout`）。`readValidated` の doc が持っていた Remove の同期規約は、実態（`readValidatedHandleWith`）に合わせて `Read` 側へ移した
- **C5** 到達不能な `_other.go` 3本と、未使用の exported ラッパ `RegisterDesktopExecutable` / `RemoveDesktopExecutable`。**小文字版とテストは残した** —— Windows の登録を Go と NSIS のどちらが持つかは §6.4 の未決事項で、再公開は一行で戻せる
- **C6** `web/src/diagnostics/PasswordPanel.tsx`（556行）。生き残っていた `eligibilityText` は `connections/` へ移し、テストを付けた
- **C9** 削除で参照0になった `password.*` の i18n キー 29 件（en/ja 両方）

### C10（文書の修正）

README の実装と食い違う記述のうち4件を直した。「Go 側でプログラムを起こす場所は2つ」（実際は5つ）、ポート転送が同じ節で「動く」と「まだありません」の併記（実装を確認して後者を削除）、`--hidden` 起動と「Linux にはこの起こし方がまだありません」（Linux にはある）、`/cli/unlock` へ渡す解錠（**その route は削除済みで、`TestLegacyCLIUnlockRouteIsNotRegistered` が 404 を表明している**）。

### 検証（全通過）

`gofmt` / `go vet` / `go test ./...`（33 pkg）/ `go test -race ./...` / `GOOS=android` ビルド / web 677 テスト / `tsc` / desktop 41 テスト / `make verify-generated` / `check:i18n`。
規模は **38 ファイル、344 行追加・1219 行削除**。`deadcode` の検出は linux 26→19、windows 29→23 に減少。

### 手を付けなかったもの

- **利用者の判断が要るもの**: §6 の9件（Android のビルドタグ、`internal/api` の契約の所有者、Electron 外殻、Windows の登録の担い手、`SSHC_ASKPASS_*` 防御の扱い、`internal/application` の位置づけ、`Run`/`Stream` の差、`ui/form` と `surface` の統合、テスト専用パッケージの命名）
- **構造の大きな変更**: C13・C15・C17・C19・C24〜C28。いずれも L 規模で、S2 以降の段階に属する。C12（Capabilities）は実害6の範囲でテストによる強制に留めた

## 0.8 第二段: 判断 9 件と構造の変更

`docs/2026-08-19-design-decisions.md` に 9 件の判断を記録し、7 件を実装した。あわせて
S2〜S5 の構造変更を進めた。**全 32 コミット、102 ファイル、+4035 / -2689 行。**

### 構造の変更

| id | 内容 | 結果 |
|---|---|---|
| C13 | 合成の根を `internal/app` に閉じる。`sshParts` と `CLIConnection` が同じ形の `Dialer` を別々に組んでいたのを畳み、`cmd/sshc` の一覧と TUI を `app.ReadConnections` 経由に寄せた | **TUI の表示と実接続が同じ解決器を通るようになった**（それまで TUI は `Project` 経由で Match を見落としていた） |
| C15 | commit + ConflictError 変換の 4 実装を 1 つに | 4 つのうち 3 つは、衝突したパスが自分の計画した設定でないときも nil の base で報告を組み立てていた |
| C17 | 検証規則を `internal/platform` から `internal/validate` へ。`application` の手書きの双子も統合 | `internal/platform` を import する本番ファイルが 29 → 15 |
| C19 | HTTP 層と永続化層の間にサービス層を挟む | `internal/httpserver` から `storage`・`envelope`・`objectstore` の import が **0 に** |
| C20 | 生成型を契約の正本に。ずれを検出する検査を追加 | 初回実行で 1 件検出（`TerminalSettings` の json タグ欠落。入力専用と判明） |
| C24 | `storage.Manager.commit` を分割 | **387 行 → 184 行。** 3 つの並行スライスの添字対応を `journalPlan` として型に |
| C25 | `EditKind` の 3 重 dispatch を表に。`service.go` を分割 | 1450 行 → 1010 行 + `plan.go` 454 行 |
| C26 | `App.tsx` の 23 prop を 4 つの関心に | 葉のコンポーネントの API は不変（テストの巻き添えを避けた） |
| C27 | `ConnectionsPage` の状態を 3 つの hook に | 画面直下の `useState` が 19 → 6 |
| C28 | `KeysScreen` の状態をワークフローごとの hook に | 42 → 13。**`closeAgentForm` と `closeStoredPassphraseForm` が同じ 3 つを消していた**のを、共有物として明示 |

### 機械検査として固定したもの（C30）

- `sshclient.Dialer` を組み立てる本番ファイルはひとつ
- `~/.ssh` を開くのも、トランザクションマネージャを作るのも `internal/app` だけ
- engine の合成の根は必ず封をする
- HTTP 層は永続化層を import しない
- 生成型と実際に返している型の JSON フィールド名が一致する（32 対）
- `app.Dependencies` の全項目について Android 側が選択を宣言している

### 残したもの（第二段の時点）

以下は「下位互換を無視してよい」という指示を受けて第三段で実施した。§0.9 を参照。

- `ui/form` と `ui/surface` の統合（判断 8）
- `ProxyJump` のパスフレーズ開示（判断 5 の未決部分）
- `KeysScreen` の JSX 分割
- `internal/buildcontract` と `internal/acceptance` の住み分け（判断 9 の未決部分）

### 検証

各段階で `go test ./...` を通し、最後に全スイートを再実行した。`gofmt` / `go vet` /
`go test`（34 pkg）/ `-race` / クロスビルド 5 種 / web 677 テスト / `tsc` / desktop 41 /
`make verify-generated` / `check:i18n` すべて通過。**32 コミットはいずれも単独でビルドが
通る**（bisect 可能）。

## 0.9 第三段: 下位互換を外して

「下位互換性は無視してよい。これを考慮するとなにも解決しない」という指示を受けて、
挙動を変える判断を含む残りを実施した。

### 鍵のパスフレーズを連鎖ぶん渡す

アカウントのパスワードは ProxyJump の連鎖ぶんを渡していたのに、パスフレーズは行き先
1 件だけだった——**行き先には届く接続が、手前で止まって手入力を求めていた。**

さらにその 1 件を選ぶ規則が、ProxyCommand・ProxyJump・Match・XAuthLocation を見つけ
たら何も返さない、というものだった。環境変数で bearer capability を渡していた頃の規則
である。**いまのクライアントはそのどれも実行しない。** 鍵を決めるのは `effective.Resolve`
にし、自前で設定を歩く 2 つ目の実装を消した。応答は `keyPath`+`passphrase` の 2 つから
`passphrases` の map ひとつになった（`/cli/*` は OpenAPI に載らない私的なプロトコル）。

### `Button` へ寄せる

対象は監査が言う 110 箇所ではなく、**生の `<button>` 69 箇所**だった（残りは import 行）。
寄せた理由は見た目ではない — `Button` が `type="button"` を既定にする理由を doc は
「フォーム送信はどこにもなく」と書いていたが、`<form onSubmit>` は 6 箇所ある。
**form の中で submit にならずに済んでいたのは、書く人が毎回付けていたからである。**

### `KeysScreen` の JSX 分割

6 つのフォームを `KeyForms.tsx` へ。**prop の束は hook の戻り値そのもの**なので、
10 個の prop に開き直す必要が無い。1600 → 1229 行。

### `buildcontract` を割る

本番のコード（ネイティブビルド CLI と機種判定）を `internal/nativebuild` へ出し、
`buildcontract` にはリポジトリのビルドファイルを検査するメタテストだけを残した。
`internal/acceptance` へは寄せていない——あちらは**走っているアプリ**を、こちらは
**リポジトリのビルドファイル**を検査する。主題が違う。

移した結果、`TestOnlyTheNamedSubsystemsStartAProgram` と Makefile の契約検査が古い
パスを指して赤くなった。**それがあの検査の仕事である。**

## 0.10 第四段: 契約と型と、走らない規則

§5 の残りと、監査が数え落としていたものである。**計画どおりに終わらなかったものが 2 件あるので、そこを先に書く。**

### C20 は、提案した A でも B でもない道になった

§6 は (A)「生成型を通信の正本にする」か (B)「models.gen.go を捨てる」かを問い、C20 は (A) を推す形で書かれていた。**実測したら、どちらも採れなかった。**

同名 36 対のうち **27 対は Go の型が違い、違いは系統的だった**: 生成側は OpenAPI の省略可能を `*T` で表し、`application` 側は値と `omitempty` で表す。さらに `application` は `DiffOp` / `EditAction` のような名前付きの型を持ち、生成側はそれを `string` にする。**(A) を採るとドメインの側が弱くなる。**

採ったのは「寄せずに、生成しない」である（`exclude-schemas` に 20 スキーマ）。対は 36→18、`models.gen.go` は 1524→1339 行、到達不能な生成型は 84→57 になった。残る 18 は、使われている request/response の中に入れ子で現れるので生成が要る——**そこが今の構成での底である。**

副産物として、適合検査の相手が変わった。生成された双子と比べていたのを **`openapi.yaml` そのものと比べる**形にした（`internal/acceptance/contract_drift_test.go`）。`httpserver` 側の `DisallowUnknownFields` 検査は、生成型ではなく実際に `c.JSON` へ渡している型へ向けた。2 つ合わせて「返る本文 ⊆ application の型 = 契約」になる。

### identityKey は 5 実装ではなく 6 箇所だった

§4 の C29 は「5 実装」と書いた。実際には 6 箇所あり、**監査が数え落としていた 1 箇所が一番危なかった。**

`ConnectionTree.tsx` は同じ関数の中で、Map の構築を**直書きのテンプレート文字列**で行い、参照は名前付きの関数で行っていた。名前が付いていないので `identityKey` を grep しても出てこない。**関数だけを直せば、照合は例外も型エラーも出さずに全部外れる**——症状は全ホストがメモ・色・並び順を失うことである。

残りは `connectionBrowser` の定義、`QuickConnectBrowser` の React key、`resetKey` 4 箇所（identity の綴りに内容を足したもの）。落とす場所を 1 つにし、`Record<keyof HostIdentity, true>` で項目の取りこぼしを型で塞いだ。**この検査は省略可能な項目で効くことを確かめてある**——必要な項目なら fixture が軒並み赤くなって埋もれるが、省略可能なら fixture は通り、赤くなるのはこの検査だけだった。

### 外殻に静的検査が無かった（監査の範囲外だった）

`desktop/` の 1,163 行は素の JS で、**静的には誰にも見られていなかった。** Go は gofmt と vet を、web は tsc を通すのに、エンジンを spawn し所有権のパイプを握り symlink を張る package だけが、走らせるまで何も分からない。TypeScript にした。

移動で静かに壊れるものが 1 つあった。出力が `out/` に入ると `join(__dirname, "build", "icon.png")` は**例外を出さずに無い道を指す**——`createFromPath` は空の画像を返すだけなので、症状は「図が Electron の既定に戻る」であり、ビルドもテストも緑のまま出荷される。場所を知るのを `paths.ts` に集約し、実在を検査する。**electron-builder に実際に束を作らせて中を確認した**（`/out/*.js` の一つ上に `/build/` と `/package.json`、テストと `adhoc.js` は非同梱）。

`build.files` の検査は 4 つを名指しで並べており、その後に増えた 3 つは誰にも数えられていなかった。`main.ts` から import を辿って求める形にした。

型が実際に見つけたもの: `catch (error)` が Error を前提に `error.code` を触っていた／`JSON.parse(status)` の `any` が `sessions` へ素通りしていた（読めない行が来ると**例外も出さずに**メニューバーが undefined を綴る）／`BrowserWindow` が `icon()` を二度呼んでいた／メニューの role が `string` だった。

### 「lint の役目は型検査が担う」は、半分だけ正しかった

CI にそう書いてあり、大筋では正しい。だが tsc が見ないものが二種類ある: **待ち忘れた Promise** と **React の規則**である。しかも `react-hooks/exhaustive-deps` を名指しで抑止するコメントが既に 9 箇所あった——**規則の側は一度も走っていなかった。** 抑止だけがあって規則が無い状態は、規則が無いより悪い。読む人には「検討済み」に見えるからである。

**入れなかったものの方が多い。** `recommendedTypeChecked` は 265 件出すが、174 件は `unbound-method` で**うち 173 件はテストの `expect(obj.method)`**、`set-state-in-effect` の 23 件は注釈で理由が書かれた意図的な形だった。信号より雑音が多い lint はいずれ丸ごと切られるので、25 件に絞って**全部直した**（抑止で消したものは無い）。

## 0.11 実行による検証（第二回）

§0.5 が「未検証」に挙げた 4 つのうち、**e2e はこの段で走らせた。**

| 検査 | 結果 |
|---|---|
| `go build` / `go vet` / `go test ./...` | 通過（35 パッケージ） |
| `go test -race ./...` | 通過 |
| web: vitest | 697 通過 / 73 ファイル |
| web: `tsc -b` + e2e の tsconfig | 通過 |
| web: eslint（src + e2e + 各 config） | **0 件** |
| desktop: `tsc --noEmit` | 通過（strict、`noUncheckedIndexedAccess` 込み） |
| desktop: `node --test` | 51 通過 |
| `make verify-generated` | 通過 |
| クロスビルド（windows / android） | 通過 |
| **e2e: chromium (90) + narrow (5)** | **95 通過 / 1 失敗** |
| electron-builder による束の作成（linux dir） | 通過。asar の中身を目視で確認 |

e2e の 1 失敗は `desktop.spec.ts`（実際に Electron を起こす 1 本）で、理由は **X server が無いこと**である（`Missing X server or $DISPLAY`）。CI は `xvfb-run` 経由で走らせている。この環境では xvfb の導入に root が要るため走らせられなかった——**コードの問題ではないが、確かめられてもいない。**

**なお未検証のまま**: 実機での挙動（macOS / Windows / Android 実機、dmg / AppImage / NSIS が実際に起動するか）、S3 同期、PTY を伴う対話。TypeScript 化は束の**配置**までは実測したが、**起動**は確かめていない。

---

## 1. 先に直すべき実害

設計論より優先する。いずれも「テストは緑だが本番の挙動が違う」型。

| # | 内容 | 場所 |
|---|---|---|
| **0** | **履歴の並び順が同一ミリ秒で非決定的になる。** トランザクション ID は `YYYYMMDDTHHMMSS.mmm` + `-` + ランダム hex（`transaction.go:937`）。`readRecords` はファイル名の**文字列ソート**で並べ（`journal.go:513`）、`History()` はそれを逆順にして「新しい順」とする（`history.go:27`）。同じミリ秒に 2 件落ちるとタイムスタンプ接頭辞が一致し、**順序がランダム hex 接尾辞で決まる**。`TestUpdateConnectionCommitsEveryPasswordModeWithTheConfig/new_shared` が **30 回中 19 回失敗**（`make test` がほぼ通らない）。利用者には履歴画面で同一ミリ秒の操作が毎回違う順に見える。`journalRecord.StartedAt` はナノ秒精度を持つのでタイブレークに使える | `internal/storage/journal.go:513`, `internal/storage/history.go:27`, `internal/storage/transaction.go:937` |
| 1 | 鍵 vault 用の `storage.Manager` に `Seal` が配線されず、**パスフレーズ変更時に平文の秘密鍵が世代バックアップに残る**。`Seal` の代入は本番 1 箇所のみ | `internal/app/run.go:142`, `internal/app/run.go:265`, `internal/keys/service.go:451` |
| 2 | `internal/platform/windows.Toolchain` は実装もテストも完成しているのに配線されず、**Windows だけ FIDO 鍵（ed25519-sk / ecdsa-sk）の項目が画面に出ない**。`catalogue.go:74` が `Toolchain == nil` で早期 return する | `cmd/sshc/wiring_windows.go:22`, `internal/platform/windows/toolchain.go:35` |
| 3 | `effective.Project` が `Match` ブロックを一切適用しない（`blockApplies` が `BlockMatch` に常に `("", false)` を返す）ため、**`Match host db` 配下の `PasswordAuthentication no` を資格情報の可否判定が見落とす** | `internal/effective/provenance.go:252`, `internal/application/passwordeligibility.go:120` |
| 4 | `Probe` だけ `HostKeyAlgorithms` を渡さず、認証テストが**実接続の通るホストを `host_key_changed` と誤報しうる** | `internal/sshclient/probe.go:106`, `internal/sshclient/client.go:152` |
| 5 | `ForceClose` がプロセス内 SSH セッションに効かない（doc は効くと書いている） | `internal/terminal/terminal.go:144` |
| 6 | `mobile/dependencies.go` が struct literal のため `Biometric` と `ShutdownTimeout` が黙って欠落（コメントは「落としているのは 4 つ」と書くが実際は 6 つ） | `mobile/dependencies.go:30` |

2 は 3 と同じ根（`nil` による機能欠落）から出ている。1 と 2 は S2 で構造的に再発不能にする。

## 2. 根本原因

### 1. 「並走させてから寄せる」方式の計画で、削除フェーズだけが実行され統合フェーズが実行されなかった。逆に「削除対象を一覧で書いた」計画（embedded-terminal の D8、single-app の Task 7/8）は完全に畳まれている。

- effective.Project が製品 9 箇所に残ったまま Resolve と並走（provenance.go:106 / resolve.go:95）
- Project が Match を無視するため passwordeligibility.go:120 が Match 配下の PasswordAuthentication no を見落とす
- effective.Cumulative（provenance.go:32）が plan Task 1 の成果物として孤立し、製品参照 0
- application/walk.go:24 と effective/provenance.go:206 に Include ロード順の walker が 2 実装

### 2. 抽象を導入したあと、既存の呼び出し側を移行し切らずに新旧が併存した。「新しい作法が旧作法を置き換えた」のではなく「隣に増えた」。

- commitPlannedRequestWith があるのに Save/commitGroupPlan/RelocateKey が同じ 20 行を複製（4 つとも挙動が違う）
- web の asRecord/asArray/asString が 4 モジュール、toProblem が 4 画面に逐語コピー
- ui/surface.tsx の Button 43 箇所 vs ui/form.tsx のクラス文字列 110 箇所が機能ディレクトリ単位で排他
- IntegrationsApi を Pick で絞るのは新しい 3 箇所だけ、古い 8 テストは as unknown as で型検査を無効化
- closeAll（client.go:179）があるのに session.go の 3 メソッドと probe.go:54 が逆順ループを手書き

### 3. 「機能の不在」を nil interface で表しているため、配線漏れと意図的な欠落が型でもテストでも区別できない。

- cmd/sshc/wiring_windows.go:23 が Toolchain を nil のまま返し、Windows で FIDO 鍵の項目が消える（実装は完成済み）
- storage.Manager.Seal の代入が run.go:265 の 1 件のみで、鍵 vault 用（run.go:142）と CLI 用（ssh.go:215）が平文バックアップを書く
- mobile/dependencies.go が struct literal のため Biometric と ShutdownTimeout が黙って欠落（コメントは「落としているのは 4 つ」と書くが実際は 6 つ）
- nil 判定が 11 箇所（keys 4・secret 6・httpserver 1）に散り、internal/platform 内には 0 箇所

### 4. internal/platform が「OS 抽象の層」ではなく「置き場所に困ったものの袋」になり、逆に本物の OS 差分は 9 パッケージに散っている。

- internal/platform を import する非テスト 29 ファイルのうち 22 は ValidateAlias / LocalAccountName / interface 宣言しか使わない
- 直下 430 行のうち OS 条件付きは 191 行（44%）、残る alias.go/directory.go/sanitise.go/command.go 239 行は regexp と path/filepath だけ
- build tag 付き非テスト 50 ファイルのうち platform 木の下は 19 本のみ（残り 30 本は cmd/sshc 12・handoff 4・keys 4 ほか）
- internal/platform/macos → internal/secret（biometric_darwin.go:18）と internal/secret → internal/platform（vault.go:23）で辺が両方向。Windows Hello を同じ形で置くと本物の import 循環になる

### 5. usecase 層が config 系にしか存在せず、httpserver が domain とインフラを直接握っている。

- httpserver の非テスト 19 ファイル中 internal/application を import するのは 7 のみ、82 ルート中 34 が application を通らない
- httpserver/sync.go:207 が objectstore.Client を自分で組み立て :290 で envelope.Derive を直接呼ぶ
- httpserver 5 ファイルが internal/storage の sentinel error と ConflictError を直接握り、永続化層のエラー名が HTTP 契約になっている
- password.go:125 の changeMasterPassword（secret の rekey + remotesync の再封印）という明白な usecase が HTTP ハンドラの中にある

### 6. 通信形式の所有者が決まらないまま、httpserver 内で 3 方針（生成型を使う / 手書き双子 / application 型を直流し）が併存した。

- internal/api の生成型 203 個のうち 33 個の実質スキーマが Go から到達不能（api.EditRequest/Effective/HostForm/GroupRenameRequest/RecoverResponse の参照はいずれも 0）
- config_handlers.go:26-55 が生成型と同形の構造体を 6 個手書き
- connections.go は逆に api 型から application 型へ 8 関数・約 300 行の手変換を持つ（覆うのは 82 ルート中 2）
- problem 本体が api.Problem と problemPayload の 2 型に割れ、後者は openapi.yaml:1293 の additionalProperties:false に違反する blockers を返す

### 7. 同じ規則を Go と Web が別々に実装し、同期する仕組みが「コメントでの祈り」しかない。

- グループ名の予約語が Go 10 個（grouppath.go:40）に対し Web 6 個（GroupsPanel.tsx:83）。rc / environment / known_hosts2 / authorized_keys2 が Web に無い
- 先頭文字ルールが逆向きにずれ、Go は -foo を受理し Web は拒否
- OpenSSH の引用規則が config/token.go と web/values.ts に二重で、Go だけが ErrUnquotableValue を持ち TS は壊れた引用を生成する
- ValidateAlias が platform（alias.go:39）と application（edit.go:161）に同名で 2 実装、長さ上限は 64/64/255 の 3 種
- known_hosts のホスト欄書式が 3 箇所で組み立てられ、hostkey.go:234 のコメントが「同じでなければならない」と祈っている

### 8. 「変えたこと」を記録した文書（README・plan・コメント）が、変更に追随しない。しかもこのリポジトリはコメント密度が異様に高いため、陳腐化したコメントが通常より強く誤読を誘う。

- README.md:368 の「1 台の macOS ランナーで 4 バイナリ」が同じ README:176-186 の表と release.yml の 4 job 構成と正面から矛盾
- README.md:380-381 が死んだ launchBackground の --hidden 起動と存在しない /cli/unlock を現行仕様として説明
- README.md:327 が「プログラムを起こす場所は 2 つ」と書くが programs_test.go:32-53 の allowlist は 5 件
- internal/terminal/terminal.go:3 の package doc が「ssh のコマンドラインを組むのは internal/platform」と、消えた責務分担を説明
- internal/httpserver/diagnostics.go:198 に削除済みハンドラ TerminalCommand の doc だけが宙に浮いている

## 3. あるべき層構造

今のパッケージを移動・削除・改名だけで到達できる 8 層に整理する。新規パッケージは internal/validate（純粋な入力検証と Digest）と internal/globmatch（OpenSSH glob）の 2 つだけで、それ以外は既存パッケージの再配置。中核の 3 つの規約は (1) internal/platform は「OS ごとに実装が変わる interface と、その build-tag 実装」だけを持つ層に戻し、alias.go / directory.go / sanitise.go / command.go を追い出す。(2) 「機能の有無」は nil interface ではなく合成の根で 1 度だけ確定する Capabilities 値で運ぶ。(3) internal/httpserver は L2（storage / envelope / objectstore）を直接 import しない。この 3 本は internal/acceptance の依存方向テストで機械的に固定する（既に programs_test.go が全 .go を歩く前例がある）。層の深さが config 系だけ深いのは無損失パーサ＋トランザクション＋三者マージという固有の複雑さに見合っており揃える必要はないが、internal/application が「アプリ全体の usecase」に見える名前と doc を持っている点は直す。

| 層 | パッケージ | 責務 | 依存してよい層 |
|---|---|---|---|
| **L0 OS API** | `internal/platform/nativepath`, `internal/platform/windowsacl`, `internal/platform/windowspipe`, `internal/platform/windowsregistry`, `internal/platform/process` | golang.org/x/sys と syscall を直接叩く。DACL・named pipe・レジストリ・パス文法・プログラム探索。sshc/internal の他をひとつも import しない（現状 process/command.go だけが親 platform を import しているが、その原因である OutputRunner ごと削除する）。 | （なし） |
| **L1 OS 抽象** | `internal/platform`, `internal/platform/linux`, `internal/platform/macos`, `internal/platform/windows` | OS ごとに実装が変わる interface とその build-tag 実装のみ。KeyAgent / Toolchain / Guardian（secret から移設）/ LoginShell 系 / LocalAccountName。alias.go・directory.go・sanitise.go・command.go はここから出す。platform/macos が internal/secret を import する現在唯一の逆流を消し、platform 木が domain を一切知らない状態にする。 | L0 OS API |
| **L2 インフラ** | `internal/storage`, `internal/envelope`, `internal/objectstore`, `internal/enginelock`, `internal/handoff`, `internal/session` | ディスク・ネットワーク・OS ロックの原始操作。トランザクション・ジャーナル・世代バックアップ・封・条件付き PUT・セッション cookie。storage からは Digest（→ L3 validate）と NewResolver（→ L7 合成の根）を出し、storage → internal/config と storage → os/user の辺を消す。 | L1 OS 抽象, L0 OS API |
| **L3 純粋ロジック** | `internal/config`, `internal/effective`, `internal/validate`, `internal/globmatch`, `internal/api` | OS にもディスクにも依存しない値と規則。無損失パーサ・Include グラフ・解決器（Resolve のみ。Project は廃止）・入力検証（ValidateAlias/Hostname/Port、platform と application の 2 実装を統合）・内容ハッシュ・glob マッチ・生成された通信契約型。 | L1 OS 抽象 |
| **L4 ドメインサービス** | `internal/keys`, `internal/knownhosts`, `internal/secret`, `internal/terminal`, `internal/sshclient`, `internal/remotesync`, `internal/remotekey`, `internal/diagnostics`, `internal/selfupdate` | 1 つの主題を所有するサービス。鍵素材・known_hosts・保管庫・端末セッション・SSH 対話・同期・リモート登録・検査。secret から Guardian interface を L1 へ出す。internal/terminal は現在どおり内部依存ゼロの葉に保つ。 | L3 純粋ロジック, L2 インフラ, L1 OS 抽象 |
| **L5 ユースケース** | `internal/application` | 複数の L4 サービスまたぐトランザクションと、ssh_config 編集の読み書き計画。commit + ConflictError 変換の唯一の入口。httpserver から changeMasterPassword（secret の rekey + remotesync の再封印）のようなサービス横断処理を引き上げる。パッケージ doc を「アプリ全体の usecase」ではなく現在の実態に合わせて明記する。 | L4 ドメインサービス, L3 純粋ロジック, L2 インフラ, L1 OS 抽象 |
| **L6 トランスポート** | `internal/httpserver`, `internal/ui` | HTTP ルート 82 本と WebSocket、SPA 配信、認証面 3 つ（ブラウザ / CLI / ストリーム）。internal/storage・internal/envelope・internal/objectstore を import しない（現在 sync.go / connections.go / knownhosts.go / keys.go / config_requests.go の 5 ファイルが違反）。storage の sentinel error は application 側で包み直したものだけを扱う。 | L5 ユースケース, L4 ドメインサービス, L3 純粋ロジック, L1 OS 抽象 |
| **L7 合成の根** | `internal/app` | Workspace / Manager / Resolver / Capabilities を組む唯一の場所。storage.NewWorkspace・NewManager・NewResolver の呼び出しをここだけに閉じ、Seal の差し忘れと Scanner の欠落を構造的に不可能にする。app.Dependencies は struct literal ではなく必須項目を要求する構築関数で作り、mobile と cmd/sshc の乖離を型で検出する。 | L6 トランスポート, L5 ユースケース, L4 ドメインサービス, L3 純粋ロジック, L2 インフラ, L1 OS 抽象 |
| **L8 エントリ** | `cmd/sshc`, `mobile`, `internal/buildcontract` | argv 解析・OS ごとの外殻起動・所有権チャンネル・gomobile バインド・ビルド CLI。internal/config / internal/effective / internal/storage を直接 import しない（現在 cmd/sshc/list.go と tui.go が違反し、TUI の表示と実接続が別エンジンから来ている）。app が公開する読み取り関数だけを使う。 | L7 合成の根, L1 OS 抽象 |

新規パッケージは `internal/validate`（純粋な入力検証と Digest）と `internal/globmatch`（OpenSSH glob）の 2 つだけ。
それ以外は既存パッケージの移動・削除・改名だけで到達できる。

## 4. 変更カタログ

| id | 種別 | 規模 | 危険度 | 内容 |
|---|---|---|---|---|
| C1 | delete | S | low | 外部プロセス実行の継ぎ目一式を削除する（platform.OutputRunner + process/command.go） |
| C2 | delete | S | low | launchBackground 4 実装と launch_unsupported.go、および Electron の --hidden 分岐を削除する |
| C3 | delete | S | low | askpass 廃止の残骸を削除する（secret の定数と、存在しない環境変数を守る資格情報ポリシー） |
| C4 | delete | S | low | 参照 0 の小物 7 件を削除する（sshFinder / handoff.Random / readValidated / nativeFileSystem ほか） |
| C5 | delete | S | low | 到達不能な Windows 系スタブと未使用の exported ラッパを削除する |
| C6 | delete | S | low | web/src/diagnostics/PasswordPanel.tsx を削除し、eligibilityText を connections/ へ移す |
| C7 | delete | M | low | 製品が一度も設定しない注入フィールドとその nil フォールバックを削除する |
| C8 | delete | M | medium | 死んだビルド経路と startup.sh を削除する |
| C9 | delete | S | low | Notice / i18n の未使用コードと、逆に翻訳が無い発行済みコードを揃える |
| C10 | doc | M | low | 陳腐化した README・package doc・コメントを実装に合わせて直す |
| C11 | merge | M | medium | Toolchain を KeyAgent と同じ build-tag factory に畳み、Windows の配線漏れを構造的に消す |
| C12 | introduce-seam | L | medium | 「機能の有無」を nil interface から合成の根の Capabilities 値へ移す |
| C13 | merge | L | high | ~/.ssh を開く合成の根を internal/app 1 箇所に閉じ、Seal と Scanner の差し忘れを構造的に不可能にする |
| C14 | remove-seam | L | high | effective.Project を廃し、Resolve を唯一の解決器にする |
| C15 | merge | L | high | commit + ConflictError 変換を commitPlannedRequestWith 一本に寄せる |
| C16 | merge | M | low | web の API 基盤（実行時ガード・toProblem・jsonHeaders・issueAction）を api/client.ts に集約する |
| C17 | move | L | medium | 入力検証と純粋ロジックを internal/platform / internal/storage から出し、二重実装を統合する |
| C18 | move | M | medium | secret.Guardian を internal/platform へ移し、platform 木から domain への逆流を消す |
| C19 | move | L | medium | httpserver から internal/storage / envelope / objectstore の直接 import を消し、サービス横断の usecase を application へ引き上げる |
| C20 | merge | L | medium | internal/api の契約方針を 1 本に決め、手書き双子と未使用の生成型を解消する |
| C21 | introduce-seam | M | medium | Go と Web に二重実装された規則（グループ名・引用・alias・known_hosts 書式）を機械で同期させる |
| C22 | move | M | medium | cmd/sshc と mobile を app 経由に寄せ、エントリ層からの直接 import と配線の複製を消す |
| C23 | merge | S | low | 偽の OS 分岐（signals / Stop / wiring の KeyAgent / vault アクション表）を畳む |
| C24 | split | L | medium | storage.Manager.commit（387 行、リポジトリ最大の関数）をフェーズごとに分割する |
| C25 | split | L | high | EditRequest を tagged union に割り、EditKind の 3 重 dispatch を解いて service.go を分割する |
| C26 | split | M | medium | App.tsx の 23 prop バッグを HandoffContext と ShellContext に解体する |
| C27 | split | L | medium | ConnectionsPage を useReducer と 3 つのフックに分割する |
| C28 | split | L | medium | KeysScreen を 8 つのワークフローコンポーネントに分割する |
| C29 | merge | M | medium | ConnectionTree を connectionBrowser の index に載せ替え、identityKey の 5 実装を 1 つにする |
| C30 | introduce-seam | M | low | 依存方向・重複禁止を internal/acceptance の機械検査として固定する |

詳細は付録 A。

## 5. 実行順序

### S1: 消す（振る舞いを変えない削除）

対象: C1, C2, C3, C4, C5, C6, C7, C8, C9, C10

参照 0 を確認済みのコードと陳腐化した文書を落とし、以後の作業で「これは生きているのか」を毎回考えなくて済む状態にする。約 1200 行の製品コードと 550 行の web が消える。この段階の終わりに make test と make verify-generated が通り、go build ./... が 4 OS 分（Makefile:27-30）通り、npm test と npm run typecheck が通ることを確認できる。振る舞いは 1 つも変わらないので、通らなければ削除しすぎたということが即座にわかる。

### S2: 配線を正す（実害のある欠落を型で防ぐ形にする）

対象: C11, C12, C13, C18, C22

「nil が配線漏れと意図的な欠落を区別できない」という根本原因を潰し、Windows のハードウェア鍵欠落と鍵 vault の平文バックアップという 2 つの実害を直す。終わりに、(a) Windows ビルドで keys/catalogue.go が variant を返すこと、(b) `rg '\.Seal\s*=' -g '!*_test.go'` が全 Manager を覆うこと、(c) storage.NewWorkspace / NewManager の非テスト呼び出しが internal/app 内だけであること、(d) mobile と cmd/sshc の Dependencies 差分が明示的な代入としてコードに現れることを機械で確認できる。internal/acceptance の依存方向テスト 5 本のうち platform 木に関する 1 本がここで緑になる。

### S3: 権威を 1 つにする（並走している実装の統合）

対象: C14, C15, C16, C17, C19, C21, C23

「同じ問いに答えるものが 2 つある」状態を全部潰す。終わりに、(a) `rg 'effective\.Project\('` の製品ヒットが 0 になり Match 配下の PasswordAuthentication no が資格情報判定に効くこと、(b) `rg 'var conflict \*storage.ConflictError' -g '!*_test.go'` が 1 箇所になること、(c) web の asRecord / toProblem の定義が各 1 つになること、(d) ValidateAlias / Digest / MatchPattern の実装が各 1 つになること、(e) httpserver が internal/storage / envelope / objectstore を import しないことを、いずれも grep と acceptance テストで確認できる。resolver 統合は effective/resolve_differential_test.go と region_order_test.go が安全網になる。

### S4: 契約と情報構造を決める（設計判断を要する統合）

対象: C20, C29

openQuestions の回答を受けて、通信契約の所有者と Connections の情報構造を 1 本に決める。終わりに internal/api の未使用実質スキーマが 33 → 20 以下になり（または models.gen.go ごと消え）、identityKey の実装が 1 つになり、OrphanPanel.tsx の生 NUL バイトが消える。make verify-generated と e2e（chromium + narrow 2 プロジェクト・94 test）が通ることで、契約変更が UI を壊していないことを確認できる。

### S5: 大きいものを割る（巨大関数・巨大コンポーネントの分割）

対象: C24, C25, C26, C27, C28, C30

最長関数 387 行（storage.commit）と最長コンポーネント 1600 行（KeysScreen）を、既に見えている境界で割る。終わりに 300 行を超える関数と 700 行を超える tsx が無くなる。storage は journal 系テスト 4433 行、web は vitest + Playwright 94 test が安全網になるので、振る舞いが変わっていないことを既存のテストだけで確認できる。順序は C26（prop バッグの解体）を必ず C27/C28 より先に置くこと——先に画面を割ると切り出した子が prop を直接受けたくなって衝突する。最後に C30 で全規約を機械検査にし、次のピボットで同じ形が再発しないようにする。

## 6. 判断が要ること

以下は監査では決められない。方針そのものの選択なので、作者の判断が要る。

> **答えは `docs/2026-08-19-design-decisions.md` にある。** 9 件のうち 7 件を決めて
> 実装し、2 件（`ui/form` の統合、`ProxyJump` のパスフレーズ開示）は挙動を変える
> 判断なので手を付けていない。

### 6.1
**Android を linux build tag に相乗りさせ続けるか。** 現状 `rg 'go:build.*android'` は 0 件で、Android 固有の分岐は mobile/dependencies.go の struct literal 1 箇所と internal/platform/shell_unix.go:45 の runtime.GOOS だけ。GOOS=android は linux タグを満たすので Makefile:30 の `GOOS=android go build ./...` は cmd/sshc も通し、wiring_linux.go:14 の linux.NewToolchain()（/usr/bin など）を掴む。現在 APK に cmd/sshc は入らないので実害は無いが、「Android では Toolchain が nil」という不変条件を守っているのは 1 ファイルだけ。選択肢は (a) 現状維持 + C12 の Capabilities で不変条件を 1 箇所に表明する、(b) android build tag を導入して cmd/sshc を Android 対象から外す。(a) を前提に提案を組んだが、判断が要る。

### 6.2
**internal/api の生成モデルを Go 側で維持するか捨てるか。** TypeScript は openapi.yaml から独立に生成されるので、Go 側の 1524 行は httpserver のためだけに存在する。(A) 生成型を通信の正本にする（config_handlers.go の手書き 6 型を置換、未使用実質スキーマが 33→20 前後、契約違反がコンパイルエラーになる）か、(B) models.gen.go を丸ごと捨てて application 型と手書き request 型で通す（1524 行 + contract_test.go 179 行が消えるが、connections/keys/sync/password の 126 型・9 ファイル約 190 参照の書き換えが必要で、「openapi.yaml を変えたら Go が壊れる」という現在唯一の契約保証も失う）。C20 は (A) を推す形で書いたが、(B) を選ぶなら connections.go の 8 変換関数・約 300 行も同時に消せる。

### 6.3
**Electron 外殻を維持するか。** desktop/ は日本語直書き（main.js の「端末で動いている sshc がエンジンです。」、tray.js:29,36-37）、Android は英語 strings.xml のみ、Web は en/ja 1094 キーと、同一製品が 3 つの言語ポリシーを持つ。加えて外殻 2 つが共通化できる 3 点（単一エンジン保証、入口 URL の使い捨て fragment、CLI 配置）を別々に解いている。維持するなら (a) 文言を web/src/i18n から生成するか両外殻の言語ポリシーを揃える、(b) 「入口をもう一度」を engine の 1 エンドポイントに定めて Electron の `sshc open` 再実行と Android の fragment 剥がし（EngineService.java:53-55）を 1 つにする、が要る。

### 6.4
**Windows のデスクトップ実体登録を Go と NSIS のどちらが持つか。** internal/platform/windowsregistry の RegisterDesktopExecutable / RemoveDesktopExecutable は参照 0 で、実際に書くのは desktop/build/installer.nsh:78 の NSIS。C5 では Go 側を削る前提にしたが、逆に Go に寄せる選択もある。同じ問題が Linux の desktop.json（書き手 Electron / 読み手 Go）と macOS の bundle ID（書き手 OS）にもあり、3 OS で 3 つの記録方式・3 人の書き手になっている——これを 1 つの綴りに寄せるかどうかも判断が要る。

### 6.5
**`passwordeligibility.go:296-301` の SSHC_ASKPASS_* 防御を消すか残すか。** この 5 変数を設定する場所はリポジトリに存在しない（askpass は 2026-08-13 に廃止済み）が、:195 から実際に呼ばれて保存済みパスワードを拒否している。挙動を持つ残骸なので、削除するか「歴史的な防御であり現在は発火しない」と明記するかを決める必要がある。同様に `internal/platform/windowsacl` の RestrictFile / IsRestrictedToCurrentUser は製品呼び出し 0 だが、検証用 API として意図的なら doc に明記すべきで、そうでなければ削除できる。

### 6.6
**internal/application の位置づけを「config 専用の usecase」と明記するか、「アプリ全体の usecase」に拡張するか。** 現在 82 ルート中 34 が application を通らず、changeMasterPassword のようなサービス横断トランザクションが HTTP ハンドラの中にある。(a) 現状を追認して internal/configapp などに改名し doc を実態に合わせる（安価）、(b) 全サブシステムに usecase 層を作って深さを揃える（高価だが変更コストが予測可能になる）。C19 は (a) を前提に「サービス横断だけを引き上げる」中間案にしたが、方針の確認が要る。

### 6.7
**`internal/sshclient` の Run と Stream の差（SetEnv / keepalive / タイムアウト）は意図か漏れか。** 実測すると survey の「7 段を写している」は誤りで、共有は Strict 上書き・chain・defer close・NewSession・ExitError 抽出の 5 段（約 25 行）。Run（exec.go:36、remotekey 用）には SetEnv ループも keepalive も無く、Stream（stream.go:48、sshc run 用）にはある。共通化の前にこの差が意図かどうかを決める必要がある。

### 6.8
**ui/form.tsx のクラス文字列語彙（110 箇所）を surface.tsx のコンポーネント（43 箇所）に寄せるか。** 境界は機能ディレクトリ単位で排他（connections/ が Button、それ以外がクラス文字列）で意味のある線ではない。surface.tsx:127-129 は Button が type="button" を既定にする理由を「フォーム送信はどこにもなく」と書くが、その前提は既に偽で `<form onSubmit>` が 6 箇所にある——現在 form 内に type 無しの button が無いのは手で維持されているだけ。寄せるなら 110 箇所の機械置換だが、`<a>` や `<label>` にクラスを当てている箇所があれば残す必要がある（未確認）。

### 6.9
**`internal/sshintegration` と `integration/` の名前をどうするか。** テスト専用パッケージが 3 つ（integration/ = package integration、internal/sshintegration = package sshintegration_test、internal/acceptance = package acceptance_test）あり、うち 2 つの名前が近すぎて Makefile:318-321 が 4 行かけて別物だと説明している。`internal/sshdconformance` のような相手を名指しする名前に改めるか、注釈のまま残すか。同様に internal/buildcontract には自パッケージを一切テストしないリポジトリ契約メタテストが 1428 行あり、internal/acceptance と住処が二重になっている。

## 7. 付録 A — 変更の詳細

### C1 外部プロセス実行の継ぎ目一式を削除する（platform.OutputRunner + process/command.go）

**delete** / 規模 S / 危険度 low

**理由** — 参照0を確認済み。`rg -n 'RunOutput' --type go` の製品コードヒットは interface 宣言（platform/command.go:51）と実装（process/command.go:28）だけで、呼び出しは全件 *_test.go。テスト側の stub も死んでおり、internal/app/run_test.go:186 と internal/httpserver/diagnostics_test.go:32 の stubRunner は一度もインスタンス化されず、internal/keys/service_test.go:38 の recordingRunner は newServiceWithAgent が受け取った runner を ServiceOptions に詰めない（ServiceOptions に Runner フィールドが無い）。README.md:327 と internal/acceptance/programs_test.go:26 が「RunOutput を通る場所はもう無い」と自認している。後継は internal/sshclient/exec.go:17-26,95-114（MaxCapturedOutput が 64<<10 まで同値）。副次効果として process → platform の辺が消え、process が葉になる。

**対象** — `internal/platform/command.go`, `internal/platform/process/command.go`, `internal/platform/process/command_test.go`, `internal/app/run_test.go`, `internal/httpserver/diagnostics_test.go`, `internal/keys/service_test.go`, `internal/acceptance/programs_test.go`

**手順**

1. internal/platform/process/command.go と command_test.go を削除する
2. internal/platform/command.go から MaxCapturedOutput / ErrTimedOut / ErrProgramPathNotAbsolute / Command / Output / OutputRunner（9-52 行）を削除し、Toolchain interface だけを残す
3. internal/app/run_test.go:184-188 と internal/httpserver/diagnostics_test.go:25-33 の stubRunner を削除する
4. internal/keys/service_test.go の newQueryRunner / recordingRunner と newTestService / newServiceWithAgent の runner 引数を落とす（約30箇所の呼び出しが機械的に短くなる）
5. internal/acceptance/programs_test.go:107-111 の除外リストから 2 ファイルを外し、startsAProcess から "RunOutput(ctx" を落とす

### C2 launchBackground 4 実装と launch_unsupported.go、および Electron の --hidden 分岐を削除する

**delete** / 規模 S / 危険度 low

**理由** — 参照0を確認済み。`rg -n 'launchBackground' --type go` の 7 件は 4 定義行と 3 コメント行のみで、呼び出しもテストも 0。現在の経路は cmd/sshc/connectflow.go:119,155 の launcher.Launch(ctx) で、macOS では launch_darwin.go:46 の `open -b <bundleID>`（-g も --hidden も無し）になる。その結果 desktop/main.js:70 の `--hidden` 判定と :361 の窓を作らない分岐が Go 側から到達不能。launch_unsupported.go は対応する wiring_unsupported.go が存在しない（newPlatformParts は wiring_{darwin,linux,windows}.go の 3 定義のみ）ため、その build tag が選ばれる GOOS ではそもそもコンパイルできず、保険になっていない。

**対象** — `cmd/sshc/launch_darwin.go`, `cmd/sshc/launch_linux.go`, `cmd/sshc/launch_windows.go`, `cmd/sshc/launch_unsupported.go`, `desktop/main.js`, `README.md`

**手順**

1. launch_darwin.go:49-57 / launch_linux.go:124-132 / launch_windows.go:52-60 の launchBackground と直上コメントを削除する
2. cmd/sshc/launch_unsupported.go を削除する
3. desktop/main.js:65-70 の hidden 判定と :361-365 の分岐、および消えた Go 経路を説明するコメントを削除する
4. README.md:380 の「open -g -b <bundleID> --args --hidden」「最大 20 秒待ちます」「Linux にはこの起こし方がまだありません」を現行実装（launcher.Launch、connect.go:99-108 の 40×100ms=4秒、launch_linux.go は実装済み）に合わせて書き換える

### C3 askpass 廃止の残骸を削除する（secret の定数と、存在しない環境変数を守る資格情報ポリシー）

**delete** / 規模 S / 危険度 low

**理由** — 参照0を確認済み。`rg -c 'ErrUnknownToken|TokenTTL' --type go` で internal/secret/service.go のヒットは各 2 件（宣言行とその doc）だけで、他ファイルのヒットは effective.ErrUnknownToken と session.ActionTokenTTL という別シンボル。Service 型の doc（service.go:92-96）も「askpass リクエストひとつに対してである」と 2026-08-13 の 2462d9b で消えた経路を説明し続ける。一方 internal/application/passwordeligibility.go:296-301 の SSHC_ASKPASS_* 5 変数の SendEnv 照合は :195 から実際に呼ばれて動いているが、この 5 変数を設定する場所はリポジトリに存在しない——挙動を持つ残骸なので、削除するか「歴史的な防御であり現在は発火しない」と明記するかを選ぶ必要がある（要判断）。

**対象** — `internal/secret/service.go`, `internal/application/passwordeligibility.go`

**手順**

1. internal/secret/service.go:26-28 の ErrUnknownToken と :75-80 の TokenTTL を削除する
2. service.go:92-96 の Service 型 doc から askpass トークンの記述を外す
3. service.go:824-827 の「トークンを全部無効化する」というコードを伴わないコメントを削除する
4. passwordeligibility.go:296-301 の SSHC_ASKPASS_* 5 変数を、in-process SSH では argv にも環境にも載らないことを確認した上でリストから外す（残す場合は :125 のコメントに「現在の実装では発火しない歴史的な防御」と明記する）

### C4 参照 0 の小物 7 件を削除する（sshFinder / handoff.Random / readValidated / nativeFileSystem ほか）

**delete** / 規模 S / 危険度 low

**理由** — 参照0を確認済み。実測: handoff.Random は `rg -c 'handoff.Random'` が 0 件（doc は「Write が引く乱数源」と書くが write() は乱数を使わず os.CreateTemp 任せ）。sshFinder は cmd/sshc/connect.go:111,113 の 2 件（コメントと型宣言のみ）。readValidated は internal/handoff/handoff.go の 2 件（コメントと定義のみ、本体は Read と 1 行も違わず、doc の「Remove はこれを呼び」も removeWith が呼ぶのは readValidatedHandleWith なので嘘）。nativeFileSystem は internal/storage/filesystem.go の 2 件（コメントと宣言のみ、何も束ねていない）。DefaultConnectTimeout は internal/diagnostics/authentication.go の 2 件（外部 ssh に -o ConnectTimeout= を渡していた頃の残骸）。加えて remotekey.DefaultTimeout と platform.Output.Stopped も参照 0。どれも数行だが、いずれも「消えた仕組みの説明を現在形で持つ doc」を伴い、読む側に存在しない機構を信じさせる。

**対象** — `cmd/sshc/connect.go`, `internal/handoff/handoff.go`, `internal/storage/filesystem.go`, `internal/diagnostics/authentication.go`, `internal/remotekey/register.go`

**手順**

1. cmd/sshc/connect.go:111-113 の sshFinder を削除する
2. internal/handoff/handoff.go:316-317 の Random と :168-173 の readValidated を削除し、Remove の同期規約に関する記述を removeWith の実態（readValidatedHandleWith）に合わせて移す
3. internal/storage/filesystem.go:50-52 の nativeFileSystem を削除する
4. internal/diagnostics/authentication.go:21-22 の DefaultConnectTimeout と internal/remotekey/register.go:39 の DefaultTimeout を削除する
5. platform.Output.Stopped は C1 で Output ごと消えるため確認のみ

### C5 到達不能な Windows 系スタブと未使用の exported ラッパを削除する

**delete** / 規模 S / 危険度 low

**理由** — 参照0を確認済み（import グラフで裏取り）。windowsacl を import する 15 ファイル、windowspipe を import する 2 ファイル、windowsregistry を import する 2 ファイルは全部が //go:build windows なので、非 Windows ビルドでは 3 パッケージとも依存グラフに入らず acl_other.go(14行) + conn_other.go(22行) + launcher_other.go(31行) に到達する経路が無い。各 _other.go の doc は「読む側が build tag を持たずに済むため」と書くが、実際の読み手は全員 build tag を持つ。RegisterDesktopExecutable / RemoveDesktopExecutable も `rg -c` が launcher_windows.go の 2 件と launcher_other.go の 1 件だけで、テストは小文字版を叩く（実際のレジストリ書き込みは desktop/build/installer.nsh:78 の NSIS が行う）。RestrictFile と IsRestrictedToCurrentUser は製品呼び出し 0 だが検証用 API として意図的の可能性があるため削除せず doc 明記に留める（要判断）。

**対象** — `internal/platform/windowsacl/acl_other.go`, `internal/platform/windowspipe/conn_other.go`, `internal/platform/windowsregistry/launcher_other.go`, `internal/platform/windowsregistry/launcher_windows.go`, `internal/platform/windowsacl/acl_windows.go`

**手順**

1. 3 つの _other.go を削除する（定数 AgentPipe / LauncherKey / LauncherValue を非 Windows から参照したい場合は tag 無しの constants.go に分ける）
2. launcher_windows.go:96-98 と :117-119 の RegisterDesktopExecutable / RemoveDesktopExecutable を削除し、ReadDesktopExecutable だけを公開に残す
3. acl_windows.go:172 の RestrictFile と :281 の IsRestrictedToCurrentUser の doc に「production は handle ベースの RestrictFileHandle / EnsureDirectory を使う。これらはテストが結果を検証するための入口である」と明記する

### C6 web/src/diagnostics/PasswordPanel.tsx を削除し、eligibilityText を connections/ へ移す

**delete** / 規模 S / 危険度 low

**理由** — 参照0を確認済み。`grep -rn 'PasswordPanel' web/src web/e2e` のヒットは PasswordPanel.test.tsx（226 行・12 ケース）だけで、本番のレンダリング箇所は 0。全 tsx の export コンポーネント 60 個を走査して未レンダリングはこれ 1 つのみ。本番で生きているのは同ファイル :39 の eligibilityText（5 行）で、消費者は web/src/connections/ConnectionBasicForm.tsx:799,802 の 2 箇所だけ。ファイル冒頭 :16-18 のコメント「このパネルはホストエディタの内側でしかレンダリングされない」は既に事実でない。実際のパスワード UI は ConnectionBasicForm.tsx へ統合済み（e2e/password.spec.ts:12 が region "Authentication" を叩く）。features 間の依存 connections → diagnostics もこのためだけに存在する。

**対象** — `web/src/diagnostics/PasswordPanel.tsx`, `web/src/diagnostics/PasswordPanel.test.tsx`, `web/src/connections/ConnectionBasicForm.tsx`, `web/src/connections/authenticationPolicy.ts`

**手順**

1. eligibilityText（PasswordPanel.tsx:39-…）と eligibilityKeys（:20-38）を web/src/connections/authenticationPolicy.ts へ移す
2. ConnectionBasicForm.tsx:18 の import 先を "../diagnostics/PasswordPanel" から "./authenticationPolicy" に変える
3. PasswordPanel.tsx と PasswordPanel.test.tsx を削除する
4. PasswordPanel.test.tsx が表明していた不変条件のうち ConnectionBasicForm に移っていないものがあれば ConnectionBasicForm.test.tsx へ移す

### C7 製品が一度も設定しない注入フィールドとその nil フォールバックを削除する

**delete** / 規模 M / 危険度 low

**理由** — 参照0を確認済み（フィールドへの代入を全リポジトリで grep）。terminal.Spec.Cleanup（registry.go:32）: 代入は registry_test.go:420,600 のみで唯一の Spec 生成者 httpserver/terminal.go:166,180 は設定せず、doc が理由に挙げる FreezeSSHConfig は `rg` 0 件で削除済み。keys.ServiceOptions.StoredPassphrase（service.go:150）: `StoredPassphrase:` の代入が製品・テストとも 0 件で、実際の注入は全て SetStoredPassphrase（app/run.go:256）——同じ依存に入口が 2 つあり宣言的な方が死んでいる。httpserver.KeyHandlers.Sessions（keys.go:70）: keys.go 内のどのハンドラも参照せず、到達は h.Actions 経由に移っている。remotesync.Entry.Secret（snapshot.go:82）: 読み手は fixture ヘルパーのみで、snapshot.go:84 自身が「いまこれを読む側は居ない」と書く一方 service.go:419 は「pull に SkipBackup 付きで適用させる」と逆を書き、plan_test.go:105 は「どの change も SkipBackup を要求してはならない」と正反対を表明。remotekey.Register の configSnapshot 引数（register.go:187）は本体に一度も現れない。mobile.Version()（sshc.go:26）は Go・Java のどちらからも呼ばれず、値を入れる ldflags も存在しない（Makefile:74 の gomobile bind は ldflags を渡さない）。

**対象** — `internal/terminal/registry.go`, `internal/terminal/session.go`, `internal/keys/service.go`, `internal/httpserver/keys.go`, `internal/httpserver/server.go`, `internal/remotesync/snapshot.go`, `internal/remotesync/service.go`, `internal/remotekey/register.go`, `internal/httpserver/remotekey.go`, `mobile/sshc.go`

**手順**

1. terminal.Spec.Cleanup（registry.go:30-32）とその呼び出し（registry.go:167）・伝播（:188）・session.go の finish 内呼び出しを削除する
2. keys.ServiceOptions.StoredPassphrase（service.go:150）と service.go:168 の読み取りを削除し、SetStoredPassphrase 一本にする
3. httpserver.KeyHandlers.Sessions（keys.go:70）と server.go:315・keys_test.go:166,366 の代入を削除する
4. remotesync.Entry.Secret（snapshot.go:82-89）と service.go:421 の代入、snapshot_test.go の TestAPrivateKeyIsMarkedSecret、service.go:419-420 の逆説明コメントを削除する（omitempty なので旧受信側は壊れない）
5. remotekey.Service.Register の configSnapshot 引数（register.go:187）を削除し、httpserver/remotekey.go:132 の呼び出しを合わせる
6. mobile.Version() と version 変数（sshc.go:20-26）を削除する（残すなら android-bind に -ldflags を足す）

### C8 死んだビルド経路と startup.sh を削除する

**delete** / 規模 M / 危険度 medium

**理由** — 参照0を確認済み。README・scripts・desktop・.github を横断して `make build-cli` / `make desktop-version` / `make release-binaries` の呼び出しは 0 件で、唯一の言及は internal/buildcontract/makefile_test.go:36-45 と make_boundary_test.go:199,246 の「Makefile がこの的を持つこと」を固定するテスト——契約テストが dead target を延命させている。release-binaries が呼ぶ nativebuild matrix 経路（runBuildMatrix 28行 + parseReleaseTargets 39行 + SSHC_NATIVE_RELEASE_TARGETS）は単一 macOS ランナー時代の遺物で、RELEASE_TARGETS が darwin に cgo=1 を要求するため Linux/Windows ホストから通らない。desktop-bundle-windows は Windows でだけ必要なのに release.yml:151-155 が「recipe が POSIX 前提」として make を迂回する。startup.sh は `rg 'startup\.sh'` が 0 件で、内容（bare sshc を exec）は desktop.go:52-54 の noWindow 経路で exit 1 する。

**対象** — `Makefile`, `.github/workflows/release.yml`, `internal/buildcontract/nativebuild.go`, `internal/buildcontract/makefile_test.go`, `internal/buildcontract/make_boundary_test.go`, `startup.sh`, `README.md`

**手順**

1. Makefile から build-cli(:81) / desktop-version(:149) / release-binaries(:210-211) / desktop-bundle-windows(:142-144) のうち、使う予定のないものを削除する
2. makefile_test.go:36-45 と make_boundary_test.go:199,246 の期待一覧から同じ的を外す
3. release-binaries を消すなら nativebuild.go の matrix ケース(:157)・runBuildMatrix(:336-363)・parseReleaseTargets(:602-640)・nativeReleaseTargetsEnvironment(:26) を削除する
4. startup.sh を削除する
5. README.md:368 の段落を :176-186 の表および release.yml の 4 job 構成に合わせて書き直す（この段落は「1 台の macOS ランナーで 4 バイナリ」「Windows と Android はリリースに載せない」という旧戦略の文章がそのまま残っている）

### C9 Notice / i18n の未使用コードと、逆に翻訳が無い発行済みコードを揃える

**delete** / 規模 S / 危険度 low

**理由** — 参照0を確認済み（Notice 定数 31 個を全走査）。製品参照 0 は NoticeGroupCycle（notice.go:27、テストも 0）・NoticeGroupIncludePresent（region.go:29）・NoticeMatchBlock（notice.go:22、effective_test.go:79 が「出ないこと」を表明するのみ）の 3 つ。web 側は SavePreview.tsx:21 が group_cycle を i18n に写像しているが、そのコードを出す Go は無い。逆向きの欠落も同時にある: Go は directory_created / directory_removed / group_directory_created を実際に発行する（fileops.go:398,437 / service.go:1061）のに web の noticeKeys にも i18n にもキーが無く、画面に生の機械コードが出る。i18n 側では copy.command が CopyButton.test.tsx からしか参照されず、本番の消費者だった「端末へ貼るためのコマンド配布」は 2026-08-13 の fffa6ac で削除済み。web/src/connections/authenticationPolicy.ts:16 の hasDirectIdentityFile も定義とテスト以外の参照が 0（呼び出し側 2 箇所が .some(...) でインライン展開している）。

**対象** — `internal/application/notice.go`, `internal/application/region.go`, `web/src/connections/SavePreview.tsx`, `web/src/i18n/messages/en.ts`, `web/src/i18n/messages/ja.ts`, `web/src/ui/CopyButton.test.tsx`, `web/src/connections/authenticationPolicy.ts`

**手順**

1. NoticeGroupCycle と NoticeGroupIncludePresent を削除し、SavePreview.tsx:21 の group_cycle 行と en/ja の notice.group_cycle を消す
2. NoticeMatchBlock は effective.ComplexityMatchBlock（provenance.go:38、診断画面で生きている）と役割が重なるので、どちらが正本かを決めて片方に寄せる
3. directory_created / directory_removed / group_directory_created の i18n キーを en/ja に足す（または該当 Notice の発行をやめる）
4. copy.command を en/ja から削除し、CopyButton.test.tsx の label を生きたキーへ差し替える
5. hasDirectIdentityFile を ConnectionBasicForm.tsx:182 と connectionSavedState.ts:96 のインライン展開へ適用するか、関数とテストを削除する
6. Go の Notice 定数一覧と web の noticeKeys の一致を見る検査を足す（i18n の catalogue.test.ts と同種）

### C10 陳腐化した README・package doc・コメントを実装に合わせて直す

**doc** / 規模 M / 危険度 low

**理由** — このリポジトリはコメント密度が異様に高く「判断の理由を長文で残す」文化があるため、方針変更に追随しなかったコメントは通常の陳腐化より強く誤読を誘う。確認済みの食い違い: README.md:327 の「プログラムを起こす場所は 2 つ」に対し programs_test.go:32-53 の allowlist は 5 件（launch_linux / launch_windows / nativebuild が加わった）。README.md:329 と :336 が同じ節でポート転送を「動きます」と「まだありません」と両方書く（実装は forward.go:154 にあり :329 が正しい）。README.md:343 が「forwarding と LocalCommand をコマンドライン優先設定で無効化」と書くが diagnostics/authentication.go:78-80 は「無効化すべき機能の一覧はもう無い」と明言。internal/terminal/terminal.go:3 の package doc が「ssh のコマンドラインを組み立てるのは internal/platform」と消えた分担を説明。internal/httpserver/diagnostics.go:198-201 に削除済み TerminalCommand の doc だけが宙に浮く。internal/application/region.go:222-224 が既に修正済みの欠陥を「まだ直っていない」と説明。internal/knownhosts/service.go:94 が「ssh-keyscan に尋ねる」と書くが外部プログラムは起こさない。README に biometric の記述が 0 件。

**対象** — `README.md`, `internal/terminal/terminal.go`, `internal/httpserver/diagnostics.go`, `internal/application/region.go`, `internal/knownhosts/service.go`, `internal/effective/provenance.go`, `internal/secret/service.go`, `mobile/sshc.go`

**手順**

1. README.md:327（プログラムを起こす場所 2→5）、:336（ポート転送「まだありません」を削除）、:343（コマンドライン優先設定の記述を削除）、:351 と :66-71 の headless の新旧併記、:70-79 の持ち主表（9 種の invocation のうち 6 種しか載っていない）を直す
2. README に biometric unlock の節（macOS で配線済み、他 OS は未実装）と、Windows で Toolchain が nil であること（C11 で直すなら不要）を書く
3. internal/terminal/terminal.go:3-6 の package doc を「Spec.Open を通して terminal.Process を受け取る」という実際の分担に書き換える
4. internal/httpserver/diagnostics.go:198-201 の TerminalCommand doc を削除する
5. internal/application/region.go:216-225 の重複検出の説明を projection.go:216-224 の実装（alias だけを key にする）に合わせる
6. internal/knownhosts/service.go:94 の「ssh-keyscan に尋ねる」を scan.go:35-36 と同じ「外部プログラムは起こさない」に直す
7. internal/effective/provenance.go:59-61 の「権威ある値については ssh -G に委ねる」を削除する（values.go:3 と正面から矛盾している）
8. mobile/sshc.go の 6 箇所の「Kotlin」を Java に直す（android/ に .kt は 0 件）

### C11 Toolchain を KeyAgent と同じ build-tag factory に畳み、Windows の配線漏れを構造的に消す

**merge** / 規模 M / 危険度 medium

**理由** — 確認済みの実害。`rg -n 'NewToolchain'` で windows.NewToolchain の製品参照は 0 件（自パッケージのテスト 5 件のみ）で、cmd/sshc/wiring_windows.go:23 は `platformParts{KeyAgent: keys.NewAgent(os.LookupEnv)}` だけを返し Toolchain は nil。経路は engine.go:187 → app/run.go:147 → keys/catalogue.go:74 の早期 return で、Windows でハードウェア鍵の項目が黙って消える。実装（windows/toolchain.go:35-53）もテスト（`var _ platform.Toolchain = windows.Toolchain{}` を含む 5 ケース）も完成している。原因は「同じ問い（OS の道具の在り処）に 2 つの機構がある」こと——ssh-agent は keys/agent_{unix,windows}.go:16 の build-tag factory で間違えようがない形なのに、ssh-keygen だけが per-OS パッケージ + 合成の根での明示配線という漏れうる形になっている。5 パッケージ・製品 148 行 + テスト 246 行が最終的に catalogue.go:77 の boolean 1 個にしか使われず、返り値のパスもエラー値も捨てられている点も併せて畳む。

**対象** — `cmd/sshc/wiring.go`, `cmd/sshc/wiring_darwin.go`, `cmd/sshc/wiring_linux.go`, `cmd/sshc/wiring_windows.go`, `internal/platform/command.go`, `internal/platform/process/toolchain.go`, `internal/platform/linux/toolchain.go`, `internal/platform/macos/toolchain.go`, `internal/platform/windows/toolchain.go`, `internal/keys/catalogue.go`

**手順**

1. まず最小修正として wiring_windows.go に `Toolchain: windows.NewToolchain(os.Getenv("WINDIR"))` を入れ、:13-17 の「本物の Windows toolchain はその task が入れる」というコメントを消す（信頼の起点は shell_windows.go:45 の systemLookup と同じ経路を使う）
2. 次に internal/platform に build-tag 付きの newPlatformToolchain() を置き、linux/macos/windows の 3 パッケージをその中へ畳む（linux/macos は中身が 3 要素の文字列スライスなので build tag も外せる）
3. platformParts から Toolchain を外し、合成の根では OS 非依存の 1 行で呼ぶ形にする。KeyAgent も同様に wiring.go の defaultKeyAgent() へ寄せ（3 ファイルで同一行）、platformParts に残るのは Biometric だけになる
4. catalogue.go:74-80 が KeyGen() のパスを捨てて boolean としてしか使っていないことを踏まえ、interface を `HasKeyGen() bool` に狭めるか C12 の Capabilities に統合する

### C12 「機能の有無」を nil interface から合成の根の Capabilities 値へ移す

**introduce-seam** / 規模 L / 危険度 medium

**理由** — nil が「注入し忘れ」と「その OS に無い」を区別できないことが C11 の配線漏れの根本原因。確認済み: 機能の不在を意味する nil 判定は 11 箇所（keys/catalogue.go:74、keys/service.go:670/732/776、secret/biometric.go:70/101/142/196、secret/service.go:1092、app/run.go:268、httpserver/update.go:49）に散り、interface を定義している internal/platform 内には 1 箇所も無い。判定の作法も揃っておらず、nil 判定だけのもの（update.go:49）、nil + stat（catalogue.go:74）、nil + 実 dial（service.go:776）、nil + 実 SecItemAdd（biometric.go:70）が混在する。API へ出る形も KeyInventory.AgentAvailable / BiometricState.Available / catalogue の variant 有無の 3 通りで、束ねた capability 文書が無い（api.HealthResponse は status と version だけ）。「その端末に道具があるか」は OS ではなく端末の性質（署名の無い macOS ビルドでは Touch ID が無い、ssh-agent が死んでいれば Linux でも無い）なので、build tag でも nil でも表現できない。

**対象** — `internal/app/run.go`, `internal/platform/keyagent.go`, `internal/keys/catalogue.go`, `internal/keys/service.go`, `internal/secret/biometric.go`, `internal/httpserver/update.go`, `internal/httpserver/handlers.go`, `api/openapi.yaml`, `web/src/shell/UpdateBadge.tsx`

**手順**

1. internal/app に `Capabilities{HardwareKeys, Agent, Biometric, SelfUpdate, CLI bool}` を定義し、build() が 1 度だけ確定させる（Toolchain / KeyAgent / Guardian / Updates の nil 判定と実行時プローブをここに集約する）
2. 各サブシステムは Capabilities の値を受け取り、自分で nil 判定をしない（keys/catalogue.go:74、secret/biometric.go の 4 箇所、httpserver/update.go:49）
3. Capabilities を GET /api/v1/health か既存の状態レスポンスに載せ、UI の出し分けを 1 経路にする（現状 web は capability boolean で正しく出し分けており OS スニッフィングは 0 件なので、これは既存の良い設計の延長）
4. Capabilities の構築関数が全項目を要求する形にし、ゼロ値で作れないようにする——これで新しい OS や機能を足したときの埋め忘れが型で落ちる
5. BiometricState に錠前の表示名（label）を足し、en.ts:144/203 と ja.ts:128/187 の "Touch ID" 固定を解く（Windows Hello を足したとき Windows の画面に「Touch ID」と出るのを防ぐ）

### C13 ~/.ssh を開く合成の根を internal/app 1 箇所に閉じ、Seal と Scanner の差し忘れを構造的に不可能にする

**merge** / 規模 L / 危険度 high

**理由** — 確認済み。storage.NewManager の製品呼び出しは 3 件（app/run.go:142 = 鍵 vault 用、run.go:204 = 設定用、app/ssh.go:215 = CLI 用）に対し `.Seal =` の代入は run.go:265 の 1 件だけ。つまり鍵 vault 用と CLI 用の Manager は Seal が nil のまま世代バックアップを平文で書く。keys.Service.ChangePassphrase は SkipBackup を付けずに秘密鍵を commit する（service.go:451-458、`SkipBackup: true` の製品 5 件は全て secret / remotesync で keys には 0 件）ので、置き換え前の秘密鍵が ~/.ssh/sshc/backups/ に平文で残る。テストが緑なのはヘルパが自分で manager.Seal を差しているためで、README.md:311「世代バックアップは全てマスターパスワードで封をします…秘密鍵であれ vault 自身であれ、暗号文です」は run.go:204 の Manager を通る書き込みについてのみ真。同型の食い違いが Scanner にもあり、run.go:227 は Collect 付きだが ssh.go:217 は `knownhosts.Scanner{}`（Collect が nil）を渡す。storage.NewWorkspace も 3 件、NewResolver も 4 件に散る。

**対象** — `internal/app/run.go`, `internal/app/ssh.go`, `cmd/sshc/list.go`, `internal/keys/service.go`, `internal/application/service.go`, `internal/diagnostics/service.go`, `internal/acceptance`

**手順**

1. まず単独で直せる実害として、鍵 vault 用 Manager（run.go:142）に Seal を差すか、keys.Service.ChangePassphrase に SkipBackup を付けるかを決める（README:287,311 の主張と合わせる。前者が README どおり）
2. internal/app に OpenWorkspace(home, ...) を置き、Workspace / Manager / Resolver / application.Service を組んで返す。差分（鍵 vault は設定バリデータを付けない、run.go:127-137 に理由あり）は引数で表現する
3. app/ssh.go:211-217 の NewCLIConnection と cmd/sshc/list.go:46-52 の readConfigGraph をその関数経由に載せ替える。ssh.go:26-33 のコメント「組み立てる場所はここひとつである」が事実になる
4. ssh.go:126-137 と :219-228 の複製されたワークスペース相対パス判定（後者は `HasPrefix(relative, "..")` だけなので "..foo" のような正当なパスも弾く）を 1 実装に寄せる
5. internal/acceptance に「storage.NewWorkspace / NewManager の非テスト呼び出しは internal/app 内のみ」を固定するテストを足す（programs_test.go と同じ手法）

### C14 effective.Project を廃し、Resolve を唯一の解決器にする

**remove-seam** / 規模 L / 危険度 high

**理由** — 確認済み。`rg -n 'effective\.Project\(|Project\(w\.graph' -g '!*_test.go'` の製品ヒットは 9 件（passwordeligibility.go:82、connectionupdate.go:406/442、diagnostics/service.go:113/170/187、jump.go:170/209、tui.go:51）に対し `effective.Resolve(` は 2 件（application/effective.go:67,281）のみ。docs/superpowers/specs/2026-08-13-config-resolver-authority-design.md:63-72 が決定事項 4 として「2 つある実装を 1 つにまとめる。先に 1 つへ寄せてから昇格させる」と明記したが、Task 8（ssh -G 削除）だけが実行され Task 6（呼び出し側の統合）は ComputeEffective で止まった。実害を確認済み: provenance.go:248-253 の blockApplies は BlockMatch に対し常に ("", false) を返し、:115-127 の enterBlock は Match を complexity に積んで即 return するため、Match 配下のディレクティブは Source に一切現れない。passwordeligibility.go:120 は projection.Value("PasswordAuthentication") しか見ないので、`Match host db` 内の `PasswordAuthentication no` で BlockerPasswordAuthenticationOff が立たない。Resolve は同じ設定で Match を評価する（resolve.go:179-204）ので接続経路と答えが食い違う。統合先の型は既に出所情報を持つ（resolve.go:30-41 の Accepted が Keyword/Values/Path/Line/Condition を持ち、doc が Project の置き換えを目的と明記）。

**対象** — `internal/effective/provenance.go`, `internal/effective/resolve.go`, `internal/effective/jump.go`, `internal/application/passwordeligibility.go`, `internal/application/connectionupdate.go`, `internal/diagnostics/service.go`, `cmd/sshc/tui.go`, `internal/httpserver/diagnostics.go`, `web/src/diagnostics/DiagnosticsPanel.tsx`

**手順**

1. Accepted に Winner bool を足し、影に隠れた側を Shadowed []Accepted として返す——これが唯一失われる UI（DiagnosticsPanel.tsx:247,255 の source.winner による opacity-60 と "superseded" 表示）を保つために必要
2. Source.Kind（exact/wildcard/global）は httpserver/diagnostics.go:123 が api へ載せているが DiagnosticsPanel が読んでいない（source.kind の参照 0 件）ので捨ててよいことを確認する
3. 製品 9 箇所を Resolve へ移す。passwordeligibility.go:82 が最優先（Match 見落としの実害がある）、次に jump.go:170/209（ExpandRoute が Project 経由なので表示された経路と実接続が食い違う）
4. cmd/sshc/tui.go:51 は C22 の cmd/sshc 直 import 廃止と一緒に app 経由へ寄せる
5. provenance.go の Project / walkLoadOrder / blockApplies / Cumulative（製品参照 0）を削除する。MatchPattern は application.MatchHostLine が委譲しているので C17 の globmatch へ移す
6. internal/effective/region_order_test.go:49,96 が旧エンジン Project に対して順序不変条件を表明しているので、Resolve に対する表明へ書き換える
7. 既定ポート "22" が 6 箇所（resolve.go:61、jump.go:12、diagnostics/service.go:116/192、tui.go:64、passwordeligibility.go:359）に散っているのは Project に既定値充填が無いためなので、Resolve の applyDefaults（resolve.go:264-280）に一本化する

### C15 commit + ConflictError 変換を commitPlannedRequestWith 一本に寄せる

**merge** / 規模 L / 危険度 high

**理由** — 確認済み。`rg -n 'var conflict \*storage.ConflictError' -g '!*_test.go'` は application 4 件（service.go:556、grouprename.go:80、connectioncreate.go:379、keymove.go:147）+ httpserver 2 件。抽象は既に存在し、connectioncreate.go:369 の commitPlannedRequestWith を connectioncreate.go:118/358 と connectionupdate.go:146/210/213 の 5 箇所が使っている——使っていないのは Save / commitGroupPlan / RelocateKey の 3 つ。しかも 4 つの挙動が揃っていない: (a) connectioncreate.go:381-383 だけが prepared.base に無いパスを設定ファイルでないとして素通しし、他 3 つは nil のまま三者 diff を組む。(b) grouprename.go:104 は SaveResult に KeyRelocations を載せるが service.go:576 は載せない。(c) keymove.go:133-138 だけがトランザクション外で EnsureDirectory を呼び、requestFor を使わず storage.Request を直に組むので removals / directories が運ばれない——これは storage 層の設計意図（transaction.go:626 が「これがトランザクションの外の EnsureDirectory でない理由のすべてである」と書く）に正面から反する。grouprename.go:62 のコメントは「保存と同じコミット経路に通す」と書くが実際には複製している。

**対象** — `internal/application/service.go`, `internal/application/grouprename.go`, `internal/application/connectioncreate.go`, `internal/application/keymove.go`, `internal/httpserver/keys.go`, `internal/httpserver/knownhosts.go`, `internal/httpserver/connections.go`, `internal/httpserver/config_requests.go`

**手順**

1. requestFor が KeyRelocations と directories を運ぶよう揃え、keymove.go の独自 Request 組み立てとトランザクション外 EnsureDirectory を消す
2. 戻り値を SaveResult に統一し KeyRelocations は常に載せる（json:"-" なので HTTP 応答は変わらない）
3. 「base に無いパスは設定ファイルではない」（connectioncreate.go:381）を全経路の既定にする——4 つの中で最も正しい挙動で、他 3 つは非設定ファイルの衝突に空の三者 diff を作る
4. Save(service.go:532) / commitGroupPlan(grouprename.go:63) / RelocateKey(keymove.go:124) を commitPlannedRequestWith 経由に載せ替える
5. httpserver 側の ConflictError → 409 external_change 写像 4 箇所（keys.go:501、knownhosts.go:73、connections.go:429、config_requests.go）を共通の storageProblem(c, err) 1 本にする

### C16 web の API 基盤（実行時ガード・toProblem・jsonHeaders・issueAction）を api/client.ts に集約する

**merge** / 規模 M / 危険度 low

**理由** — 確認済み。`grep -rn 'function asRecord' web/src` は 4 定義（api/config.ts:35、api/integrations.ts:143、keys/api.ts:99、remotekeys/api.ts:26）で本体 5 行が throw new Error("invalid_response") まで完全一致。asArray / asString も 4 定義、asNumber / asBoolean は 3 定義。`grep -rn 'function toProblem' web/src` は 4 定義（ConnectionsPage.tsx:77、ConfigExplorer.tsx:26、GroupsPanel.tsx:24、HistoryPanel.tsx:9）で 5 行とも一致し、ApiError と failureCode の定義元である api/client.ts:6,23 には無い——層違いでもある。jsonHeaders は 3 定義、issueAction は 2 定義。各ファイルの直前に「生成された型は契約を記述するに過ぎない…型アサーションは実行時には何も証明しない」という同趣旨のコメントが別々の日本語で書かれている。失われるものが何も無い純粋な重複で、機械的・リスクゼロ。

**対象** — `web/src/api/client.ts`, `web/src/api/config.ts`, `web/src/api/integrations.ts`, `web/src/keys/api.ts`, `web/src/remotekeys/api.ts`, `web/src/connections/ConnectionsPage.tsx`, `web/src/explorer/ConfigExplorer.tsx`, `web/src/groups/GroupsPanel.tsx`, `web/src/history/HistoryPanel.tsx`

**手順**

1. api/client.ts に asRecord / asArray / asString / asNumber / asBoolean / asNonnegativeInteger、toProblem、jsonHeaders、issueAction を集約する
2. 4 つの API モジュールと 4 つの画面をそこから import する形に書き換える
3. keys/api.ts と remotekeys/api.ts を web/src/api/ 配下へ移し、API レイヤが 3 ディレクトリに散っている状態を解く
4. keys/api.ts のうち実行時ガードを通していない 5 メソッド（:287 generate、:301 hardwareCommand、:307 changePassphrase、:362 trash、:378 purge）に validate* を足す（同ファイル :97-98 が「型アサーションは実行時には何も証明しない」と宣言しているのに、この 5 つだけ抜けている）
5. config.ts に `export type ConfigApi = typeof configApi;` を足し、5 つの消費者を api?: ConfigApi の prop 注入へ寄せて vi.mock 4 ファイルを消す
6. IntegrationsApi を受ける 8 コンポーネントの prop 型を Pick<IntegrationsApi, ...> に絞り、8 件の `as unknown as IntegrationsApi` を消す（TerminalView / ConnectionChecks / ConnectionAnalysis が既に正しい作法を持っている）

### C17 入力検証と純粋ロジックを internal/platform / internal/storage から出し、二重実装を統合する

**move** / 規模 L / 危険度 medium

**理由** — 確認済み。internal/platform を import する非テスト 29 ファイルのうち、build tag に裏打ちされた OS 関数を呼ぶのは 4 つだけで、13 ファイルは ValidateAlias / Hostname / Port とそのエラー値しか使わない。直下 430 行のうち OS 条件付きは 191 行（44%）で、残る alias.go 71 / command.go 66 / directory.go 42 / keyagent.go 39 / sanitise.go 21 の 239 行は import が errors, net, regexp, strings, path/filepath だけ。その帰結として、IPv6 ホスト名を許すという純粋な入力検証の変更（コミット 1923d79）が OS 抽象パッケージの diff になった。同じ規則の二重実装も確認済み: ValidateAlias が platform/alias.go:39（正規表現 + MaxAliasLength=64、ErrUnsafeAlias）と application/edit.go:161（バイト走査、上限 64 をリテラル直書き、ErrInvalidAlias）にあり、httpserver は同一パッケージ内で両方を呼ぶ。上限も 64/64/255（config_requests.go:21 の maxAliasLength は knownhosts の検索クエリ長にも流用）の 3 種。storage 側も同型で、Digest（transaction.go:205、3 行）が 7 パッケージ 50 箇所以上から呼ばれ、remotesync/digest.go:8 が「トランザクションマネージャを import しないため」と複製理由を書きながら同じ service.go:18 が storage を import している。NewResolver（loader.go:37）は永続化層に ssh_config の Include 意味論と os/user.Current() を持ち込んでいる。

**対象** — `internal/platform/alias.go`, `internal/platform/directory.go`, `internal/platform/sanitise.go`, `internal/application/edit.go`, `internal/storage/transaction.go`, `internal/storage/loader.go`, `internal/remotesync/digest.go`, `internal/effective/provenance.go`, `internal/knownhosts/file.go`, `internal/httpserver/config_requests.go`

**手順**

1. internal/validate を新設し、ValidateAlias / ValidateHostname / ValidatePort とその sentinel error、MaxAliasLength を移す。application/edit.go:161 は削除して委譲に置き換え、config_requests.go:21 の maxAliasLength(255) は実態に合う maxURLParameterLength へ改名する
2. Digest を internal/validate（または独立した内容ハッシュの葉）へ移し、storage.Digest と remotesync.Digest の 2 実装を 1 つに寄せる。plan.go:113 が前者の値を storage.Precondition.Digest に詰めるので、片方だけ変われば precondition が静かに全滅する
3. internal/globmatch を新設し、effective/provenance.go:283 の MatchPattern と knownhosts/file.go:175 の matchHostPattern（21 行完全一致）を統合する。大小文字の扱いは呼び出し側に残すので挙動は完全に保存される
4. directory.go（ResolveUnderHome）と sanitise.go（SanitiseHomePaths）を internal/platform から出す。sanitise.go:16-20 のルートガードは `TrimRight(home, "/")` だけなので Windows の `C:\` を通してしまい、doc が防ぐと書いている「全絶対パスを壊す」がそのまま起きる点も同時に直す
5. storage/loader.go:37 の NewResolver（config.Resolver の組み立て + os/user）を internal/app へ移し、storage → internal/config と storage → os/user の辺を消す（ConfigLoader 自体は storage に残す）
6. application/effective.go:111 の LocalFactsFor と loader.go:43 が別々に user.Current() を呼んでいるので、LocalFacts（Home/User/UID/Hostname）を合成の根で 1 度だけ組む形にする

### C18 secret.Guardian を internal/platform へ移し、platform 木から domain への逆流を消す

**move** / 規模 M / 危険度 medium

**理由** — 確認済み。この 5 パッケージ層の外向き依存はすべて domain → platform だが、internal/platform/macos/biometric_darwin.go:18 だけが "sshc/internal/secret" を import し、secret.Guardian を満たすために secret.ErrNoGuardian / ErrRefused / ErrNoBiometric / ErrEmptySecret を返す。逆に internal/secret/vault.go:23 は "sshc/internal/platform" を import する。他の継ぎ目（Toolchain, KeyAgent, OutputRunner）は interface が platform 側にあるのに Guardian だけ domain 側にあり、配置規約が 1 つだけ破れている。Go のパッケージ単位では循環しないが、internal/platform/shell_windows.go:8 が既に trusted "sshc/internal/platform/windows" を import しているため、docs/superpowers/specs/2026-08-19-biometric-unlock-design.md:186 が「サブ 3」として予告する Windows Hello を macos の前例どおり internal/platform/windows に置いた瞬間、platform → platform/windows → secret → platform で本物の import 循環になりコンパイルが通らなくなる。そのとき修正は secret の公開 API 変更を伴うので高くつく。

**対象** — `internal/secret/biometric.go`, `internal/platform/macos/biometric_darwin.go`, `internal/platform/keyagent.go`, `cmd/sshc/wiring.go`, `cmd/sshc/wiring_darwin.go`, `internal/app/run.go`

**手順**

1. Guardian interface（secret/biometric.go:39-49）と ErrNoGuardian / ErrRefused / ErrNoBiometric / ErrEmptySecret を internal/platform へ移す（KeyAgent / Toolchain と同じ配置）
2. internal/platform/macos/biometric_darwin.go の import を platform に変え、platform/macos → secret の辺を消す
3. internal/secret は platform.Guardian を受け取るだけにする（biometric.go の 4 箇所の nil 判定は C12 の Capabilities に寄せる）
4. この時点で platform 木が sshc/internal 配下のうち platform 木の外を import しないことを internal/acceptance の依存方向テストで固定する

### C19 httpserver から internal/storage / envelope / objectstore の直接 import を消し、サービス横断の usecase を application へ引き上げる

**move** / 規模 L / 危険度 medium

**理由** — 確認済み。`rg -ln 'sshc/internal/(storage|envelope|objectstore)' internal/httpserver -g '!*_test.go'` は sync.go / connections.go / knownhosts.go / keys.go / config_requests.go の 5 ファイル。使っているのは storage の sentinel error（ErrDirectoryNotEmpty / ErrOutsideWorkspace / ErrSymlinkPath ほか）と ConflictError と Digest だけで、永続化層のエラー名がそのまま HTTP 応答コードになっている。最も深いのが sync.go:207 で、トランスポート層が objectstore.Client を自分で組み立て :290 で envelope.Derive を直接呼ぶ。password.go:125 の changeMasterPassword は secret の rekey と remotesync のスナップショット再封印を 1 つの単位として束ねており、定義上の usecase が HTTP ハンドラの中にある——CLI（/cli/vault/*）から同じ順序を再現する保証が型に無く、vault_cli.go は実際に独自の経路を持っている。全体では 82 ルート中 34 が internal/application を通らないファイルに登録されている。

**対象** — `internal/httpserver/sync.go`, `internal/httpserver/password.go`, `internal/httpserver/vault_cli.go`, `internal/httpserver/config_requests.go`, `internal/httpserver/knownhosts.go`, `internal/httpserver/keys.go`, `internal/httpserver/connections.go`, `internal/application`, `internal/remotesync/service.go`

**手順**

1. sync.go:75-84 と :190-211 の objectstore.Client 構築を remotesync.Service 側の Configure / Reach に押し込み、httpserver から internal/objectstore と internal/envelope の import を消す。あわせて sync.go:33 の Reach 関数継ぎ目（最も浅い HEAD 1 本にだけあり、実際にバイトを運ぶ Push/Pull/Rekey には無い）を objectstore の Store interface に置き換える
2. password.go:125-160 の changeMasterPassword を internal/application へ移し、browser と CLI が同じ usecase を呼ぶ形にする
3. storage の sentinel error を application 側で自前のエラーへ包み直す（GraphError / SyntaxError / GroupBlockedError の前例がある）。C15 の storageProblem 集約と同時に行う
4. internal/acceptance に「internal/httpserver は internal/storage / envelope / objectstore を import しない」を固定するテストを足す

### C20 internal/api の契約方針を 1 本に決め、手書き双子と未使用の生成型を解消する

**merge** / 規模 L / 危険度 medium

**理由** — 確認済み。`rg -c 'api\.<型名>' --type go -g '!internal/api/*'` で EditRequest / Effective / HostForm / GroupRenameRequest / RecoverResponse はいずれも 0 件。生成 203 型に対する推移的到達可能性解析で 76 型が到達不能、うち 43 は oapi-codegen の boilerplate、残る 33 が実質スキーマ。config_handlers.go:26-55 は生成型と JSON タグまで同形の構造体を 6 個手書きしている（groupRenameRequest / groupDeleteRequest / historyList / restoreRequest / recoverRequest / recoverResponse）。httpserver は同一パッケージ内で 3 方針を採り、config 系 12 ルートは api 参照 1 件、connections 系 2 ルートは 8 関数・約 300 行の手変換を持つ。加えて problem 本体が api.Problem（15 ファイル約 240 箇所）と problemPayload（config_requests.go:45、20 箇所）の 2 型に割れ、後者は openapi.yaml:1293 の additionalProperties:false に違反する blockers を返す——しかも `rg 'group_blocked' web/src` は 0 件で読む画面が無い。TypeScript は openapi.yaml から独立に生成されるので、契約から外れた endpoint は UI 側だけが壊れる。

**対象** — `internal/httpserver/config_handlers.go`, `internal/httpserver/config_requests.go`, `internal/httpserver/security.go`, `internal/httpserver/connections.go`, `internal/api/models.gen.go`, `api/openapi.yaml`

**手順**

1. 方針を決める（要判断・openQuestions 参照）。生成型を通信の正本にするなら config_handlers.go:26-55 の 6 型を api.* に置換し decodeEdit を api.EditRequest 経由にする——生成物の行数は減らないが未使用型が 33→20 前後になり契約違反がコンパイルエラーになる
2. api.GroupDeleteRequest.Destination は *string（契約上 optional）だが手書き側は非ポインタなので、nil → "" に畳む 1 行を足して挙動を保つ
3. problemPayload を api.Problem に統合し、blockers を openapi.yaml に足すか（読む画面が無いので）出力をやめる
4. 逆方針（application 型を直流し）を選ぶ場合は、connections.go の 8 変換関数・約 300 行を落とせるが、connections/keys/sync/password 系で「openapi.yaml を変えたら Go がコンパイルエラーになる」という現在唯一の契約保証が消えるので、レスポンスをスキーマ検証にかける acceptance テストで代替する必要がある
5. どちらを選んでも、endpoint ごとにどの方針かをコメントで書けるようにし、3 方針併存をやめる

### C21 Go と Web に二重実装された規則（グループ名・引用・alias・known_hosts 書式）を機械で同期させる

**introduce-seam** / 規模 M / 危険度 medium

**理由** — 確認済みの食い違い。グループ名の予約語は Go 10 個（grouppath.go:40-51: sshc, config, known_hosts, known_hosts2, authorized_keys, authorized_keys2, environment, rc, connections, keys）に対し Web 6 個（GroupsPanel.tsx:83）で rc / environment / known_hosts2 / authorized_keys2 が無い。先頭文字ルールも逆向きにずれ、Go の validateGroupSegment(:75-97) は `-foo` を受理し Web の segmentPattern(:79) は拒否する。GroupsPanel.tsx:75-78 のコメントは重複の目的を「invalid_request としか言わない往復通信の前に、どの文字が間違っているかを言えるようにするため」と書くが、rc や environment ではその目的が達成されていない。OpenSSH の引用規則も config/token.go:23-32 と web/values.ts:34-38 に二重で、Go の RenameArgument は `"` を ErrUnquotableValue で拒むのに formatValues にはその検査が無く、`a"b` を渡すと画面上は正常でも保存が落ちる。hostValidation.ts:28-30 のコメントは「Keep the browser check aligned with the Go boundary」と手動同期を宣言している。known_hosts のホスト欄書式は hostkey.go:239 / knownhosts/service.go:165 / sshintegration/client_test.go:104 の 3 箇所で、hostkey.go:234 が「同じでなければならない」と祈りつつ既定ポート判定が食い違う（service.go は int なので Port==0 が `[host]:0` になる）。

**対象** — `internal/application/grouppath.go`, `web/src/groups/GroupsPanel.tsx`, `internal/config/token.go`, `web/src/connections/values.ts`, `web/src/connections/hostValidation.ts`, `internal/knownhosts/service.go`, `internal/sshclient/hostkey.go`, `internal/acceptance`

**手順**

1. 最小の是正として、Web の予約語 6 個を Go の 10 個に揃え、先頭文字ルールをどちらかに寄せる（Go 側を ^[A-Za-z0-9] に狭めるのが安全側）
2. values.ts の formatValues に RenderArgument と同じ拒否条件（" / 改行 / NUL）を足し、書き出し側の非対称を消す
3. known_hosts のホスト欄書式を internal/knownhosts に `HostField(host string, port int) string` として 1 つ置き、sshclient.hostField（sshclient は既に knownhosts を import しているので逆流しない）と service.go:165 のインラインをそれに寄せる
4. 恒久策として、GET /api/v1/config/overview に groupNamePolicy（reserved, maxSegments, maxSegmentBytes, segmentPattern）を載せて Web が規則をリテラルで持つのをやめる。それが重いなら internal/acceptance に「grouppath.go の reservedGroupNames と GroupsPanel.tsx の reserved が一致する」ことを表明するテキスト照合テストを置く（internal/buildcontract が Makefile / workflow YAML の中身を固定する同種のメタテストを既に持っており作法が揃う）
5. hostValidation.ts の IPv6 判定（17 行の自前パーサ、Go は net.ParseIP）は緩く受けてサーバの Problem を表示する形に倒す

### C22 cmd/sshc と mobile を app 経由に寄せ、エントリ層からの直接 import と配線の複製を消す

**move** / 規模 M / 危険度 medium

**理由** — 確認済み。cmd/sshc の非テストファイルは internal/config / internal/effective / internal/storage を直接 import する（list.go = application, config, storage / tui.go = application, effective）。cmd/sshc の out-degree は 17 で mobile（4）の 4 倍以上。実害として、tui.go:51 が effective.Project で表示する hostname/user/port と、app/ssh.go:246 経由の NewTarget が Resolve で決める接続先が別のエンジンから来ている。mobile 側は engine.go:171-193 の 18 フィールドに対し dependencies.go:30-51 が 12 フィールドで、コメント（:17-20）は「落としているものが 4 つ」と宣言するが実際は 6 つ——Biometric（engine.go:189）と ShutdownTimeout（engine.go:192）がコメントに挙がらないまま欠けている。Go の struct literal は欠落をエラーにしないので、app.Dependencies にフィールドが増えるたびにこの乖離が静かに広がる。

**対象** — `cmd/sshc/list.go`, `cmd/sshc/tui.go`, `cmd/sshc/engine.go`, `mobile/dependencies.go`, `internal/app/run.go`, `internal/acceptance`

**手順**

1. internal/app に「alias 一覧を返す」「alias の要約（hostname/user/port）を返す」の 2 つの読み取り関数を公開し、cmd/sshc/list.go:46-52 と tui.go:51-64 をそれに載せ替える。これで cmd/sshc から config / effective / storage の import が消え、解決器も Resolve に揃う（C14 の一部を兼ねる）
2. app.Dependencies を struct literal ではなく必須項目を要求する構築関数（または各フィールドの意図を表明する Options ビルダ）で作り、cmd/sshc と mobile の両方をそこへ通す。落とすものは `deps.Updates = nil` のように明示的な代入にし、コメントではなくコードが「何を落としたか」を持つ形にする
3. Validate() を足して必須（Home / Listen / UI / Logger / Owner / PID）の欠落を起動時に弾く——現在は nil が「機能の不在」と「配線忘れ」の両方を意味する
4. internal/acceptance に「cmd/sshc と mobile は internal/config / effective / storage を import しない」を固定するテストを足す

### C23 偽の OS 分岐（signals / Stop / wiring の KeyAgent / vault アクション表）を畳む

**merge** / 規模 S / 危険度 low

**理由** — 確認済み。`diff <(sed -n '16,20p' cmd/sshc/signals_unix.go) <(sed -n '21,25p' cmd/sshc/signals_windows.go)` が空——本体 5 行が 1 バイトも違わない（syscall.SIGTERM は両 OS に存在する）。ownershipMonitor.Stop() も ownership_unix.go:207-222 と ownership_windows.go:128-143 の diff が receiver 型名 1 行のみで、15 行が逐語一致。wiring_{darwin,linux,windows}.go の KeyAgent は 3 ファイルとも `keys.NewAgent(os.LookupEnv)` の同一行で、wiring_windows.go:19-21 自身が「lookup を渡すのは signature を揃えるためだけで、あちらはそれを読まない」と書いている。vault のアクション 5 語も invocation.go:85 の whitelist・vault.go:61-62 の再検査・vault.go:101-129 の switch の 3 箇所にあり、switch の default(:130) は invocation.go が既に弾くため到達不能。逆に統合してはならないものも実測済み: vault_terminal_*（termios+poll vs ConsoleMode+ReadConsoleInputW、共有はエラー処理の定型 52 行のみ）、ownership の watch 本体（poll vs PeekNamedPipe）、platform/shell_*（実行ビット vs 信頼された既知フォルダ）、nativepath/fold_*（恒等 vs SimpleFold 軌道）は substantive 行の共有が 3〜11 行しかない。

**対象** — `cmd/sshc/signals_unix.go`, `cmd/sshc/signals_windows.go`, `cmd/sshc/ownership.go`, `cmd/sshc/ownership_unix.go`, `cmd/sshc/ownership_windows.go`, `cmd/sshc/wiring.go`, `cmd/sshc/invocation.go`, `cmd/sshc/vault.go`

**手順**

1. signals_unix.go と signals_windows.go を build tag 無しの signals.go 1 本に統合し、Windows 固有の説明（SIGBREAK を書かない理由）はコメントとして同じファイルに残す
2. ownership.go に `stopMonitor(events chan error, stop func(), joined *sync.WaitGroup) error` を置き、2 実装の Stop() をそれに寄せる（OS 固有なのは stopWatcher の中身だけ）
3. wiring.go に `defaultKeyAgent() platform.KeyAgent` を置き、3 ファイルの KeyAgent 行をそれに寄せる（C11 と同時に行うと platformParts は Biometric だけになる）
4. invocation.go に `var vaultActions = [...]string` を 1 つ置き、vault.go:61-65 の再検査と :130-131 の到達不能な default を削除する
5. internal/sshclient の closeAll（client.go:179）を session.go の 3 箇所（:132, :164, :203）と probe.go:54 に適用する（probe.go は []ssh.Conn なのでジェネリクスにするか型を持ち替える）
6. hostkey.go:64 の defaultHostKeyAlgorithms と scan.go:15 の ScanAlgorithms（7 要素が同順で完全一致）を 1 つに寄せる

### C24 storage.Manager.commit（387 行、リポジトリ最大の関数）をフェーズごとに分割する

**split** / 規模 L / 危険度 medium

**理由** — 確認済み。全 Go 関数を測った結果、最長は transaction.go:320 の commit で 387 行（次点は app/run.go:182 の build 236 行、application/grouprename.go:212 の planGroupLayout 218 行）——フロントエンドの最長関数より長い。ジャーナル・世代バックアップ・封印・原子的 rename・中断復旧という、このアプリケーションで最も壊れ方が静かな領域が 1 つの関数に収まっている。内部には既にフェーズの見出しコメントが立っており分割線が見えている: :346（planned が作るディレクトリ）、:361（ディレクトリが先）、:502（ディレクトリの削除は最後、深いものから）、:626（バリデータ受理後にディレクトリを作る）、:642（置き換え前に以前の内容をコピー）、:677（新しいファイルは隣にステージし rename を原子的に）。フェーズ間で共有される可変状態（entries / staged / previous の 3 スライスが添字対応を保つことを :642 のコメントが明記）が暗黙の不変条件になっており、フェーズを跨いだ変更の安全性を読む側が全体を保持しないと判断できない。

**対象** — `internal/storage/transaction.go`

**手順**

1. `type commitPlan struct { entries []journalEntry; staged, previous []string; directories []string }` を導入し、3 スライスの添字対応を型のフィールドとして表現する
2. 見出しコメントの位置で planDirectories → resolveChanges → planDirectoryRemovals → createDirectories → backupPrevious → stageFiles → applyRenames の 7 メソッドに切る
3. commit 本体には各フェーズの順序とロールバックの判断だけを残す（30 行程度）
4. 既存の中断復旧テスト（journal 系 4433 行）が緑のままであることを確認する——このテスト量はこの分割の安全網として十分に厚い

### C25 EditRequest を tagged union に割り、EditKind の 3 重 dispatch を解いて service.go を分割する

**split** / 規模 L / 危険度 high

**理由** — 確認済み。EditKind（service.go:42）は 12 定数、EditRequest（:82-102）は Kind + 13 個の optional フィールドで、どの Kind がどれを使うかは型に現れない。同じ 12 分岐が 3 箇所にある: plan() の switch（:583-600）、httpserver/config_requests.go:144 の validateEditRequest（97 行）、定数表自身。dispatch 先の配置も不整合で planFileEdit(:604, 132行) / planMoveHost(:927, 162行) / planMetadataEdit(:1090, 149行) が service.go に、planFileRename / planFileDelete / planDirectoryCreate / planDirectoryDelete が fileops.go にあり、切り分けの根拠がコメントにも無い。service.go は 1426 行で 100 行超の関数を 4 つ持つ。「どのフィールドがどの Kind で必須か」の知識が Go の型に無く validateEditRequest だけが持っているため、application を別経路（connectioncreate / connectionupdate / grouprename）から呼ぶコードはその検査を通らない。

**対象** — `internal/application/service.go`, `internal/application/fileops.go`, `internal/httpserver/config_requests.go`, `internal/httpserver/config_handlers.go`

**手順**

1. FileEdit / MoveEdit / MetadataEdit / FileOpEdit の 4 構造体で 12 Kind を覆い、EditRequest は UnmarshalJSON で Kind に応じて分岐するデコード層だけにする
2. validateEditRequest の「Kind に対して要るフィールドが揃っているか」の大半が型で消えることを確認し、残る長さ・文字集合の検査だけを残す
3. plan* の配置規則を決めて plan_edit.go / plan_move.go / plan_fileops.go / plan_metadata.go に分ける
4. service.go は Overview / HostDetail / FileContents / Preview / Save / Pending / History / Restore / Recover という読み書きの入口だけに絞る（500 行程度）
5. C20 で api.EditRequest を使う方針を選んだ場合は、生成型と tagged union の対応をここで決める

### C26 App.tsx の 23 prop バッグを HandoffContext と ShellContext に解体する

**split** / 規模 M / 危険度 medium

**理由** — 確認済み。SectionViewProps（App.tsx:844-873）は 23 prop で、SectionView は Connections 用に 12 を使い残りを {...props}（:905）で PaddedSection へ丸投げし、PaddedSection（:958）が 15 を分割代入する——connectionDraft / onConnectionDraftChange / onNavigateForCreation / location / onNavigationBlockerChange / preferredConnectionKey / onPreferredConnectionKeyApplied / onOpenFile の 8 は素通りする。23 のうち 11 は「画面間ハンドオフ」という 1 つの概念（作成ドラフト、生成鍵→Connections、公開鍵→Remote Keys、ファイル位置→Config）を個別の useState + 2 コールバックで表現したもの。5 つ目のハンドオフを足すたびに App の state・型・SectionView・PaddedSection の 4 箇所を触る。**この解体を C27/C28 より先に行うこと**——KeysScreen と ConnectionsPage を先に分割すると、切り出した子が prop を直接受けたくなって衝突する。

**対象** — `web/src/App.tsx`, `web/src/routing/useSectionRoute.ts`, `web/src/connections/ConnectionsPage.tsx`, `web/src/keys/KeysScreen.tsx`

**手順**

1. HandoffContext（{ connectionDraft, generatedPrivateKey, generatedPublicKey, fileTarget } の 4 スロットと hand/take だけ）を web/src/shell/handoff.tsx に作り、11 prop と App.tsx の 4 useState・4 consume コールバックをそこへ移す
2. ナビゲーション系 6 prop（section / onNavigate / location / onNavigateLocation / onNavigationBlockerChange / onInspector）は routing/useSectionRoute.ts が既に持つ概念なので ShellContext に寄せる
3. 残る実 prop を groups / knownAliases / onLock / consoles / onShowConsole / onTerminalSettingsChange の 6 個にする
4. App.tsx:1025-1029 の到達不能なフォールバック分岐（13 セクションが 2+11 で網羅済み）を削除し、`const _exhaustive: never = section` の網羅性アサーションに置き換える
5. App.tsx:123（セクション数 10→13）、:752（九画面→11 画面）、:870-871（インスペクタは Connections だけ→Groups も）のコメントを直す

### C27 ConnectionsPage を useReducer と 3 つのフックに分割する

**split** / 規模 L / 危険度 medium

**理由** — 確認済み。1018 行、useState 16（:125-149）、useEffect 8、関数 22。中核は 1 つの useEffect（:240-316）の中に 12 行の状態リセットが 4 回コピーされていること: redirect(:245-258) / invalid(:263-274) / target===null(:282-294) が完全に同一の 12 行を持ち、4 つ目（:302-311）が setActivePanel / setActiveAdvanced だけ除いた 10 行。同じ束は :538 と :736 でも再現する。「選択が変わったら何をリセットするか」という 1 つの規則が 4〜6 箇所にあり、savedRevision(:141) と invalidLocation(:134) がこの束に入っていないのが意図か漏れかコードから読めない。責務の切れ目も見えている: URL⇄選択の同期(155-220 + 240-316)、overview 読込(222-238)、保存トランザクション(407-514)、選択とパネル切替(515-560)、フィールド編集の投函(545-729)、作成完了(730-744)、端末起動(745-753)、複製・移動・削除(754-826)。

**対象** — `web/src/connections/ConnectionsPage.tsx`

**手順**

1. 選択に紐づく 12 個の state（selection, detail, savedState, editorDirty, refreshState, activePanel, activeAdvanced, missingSelection, preview, problem, managing）を useReducer に畳み、{type:"cleared"} / {type:"selected", target} / {type:"panel"} の 3 アクションにする。4 つのコピーが dispatch 1 行になる
2. URL⇄選択の同期を useConnectionSelection(location, onNavigateLocation) フックへ切り出す
3. 保存トランザクション(407-514)を useConnectionSave(...) へ、複製・移動・削除(754-826)を useConnectionActions(...) へ切り出す
4. :241-243 の「lazy chunk ロード後に effect で /connections → /connections/servers へ replace する」正規化は、sectionRoute.ts:21 と connectionRoute.ts:155 の正本の食い違いが原因なので、ナビのリンク自体を /connections/servers にして effect を消す

### C28 KeysScreen を 8 つのワークフローコンポーネントに分割する

**split** / 規模 L / 危険度 medium

**理由** — 確認済み。1600 行、useState 42（:196-239）。状態は用途で機械的に 8 群へ漏れなく割れる: 一覧と絞り込み 11 / 鍵生成 8 / パスフレーズ変更 4 / 保管庫のパスフレーズ 6 / agent 登録 3 / 移動・再配置 6 / 鍵の表示 2 / ゴミ箱 2 = 42。JSX のトップレベル子も同じ境界で切れており、`grep -n '^      {'` が 17 個の独立ブロックを返す（:684 FolderPane+表 231 行、:930 保存パスフレーズ、:1007 ゴミ箱確認、:1056 公開鍵表示、:1175 agent 登録、:1268 移動フォーム、:1383 パスフレーズ変更、:1434 鍵生成、:1537 ゴミ箱一覧ほか）。純関数部分だけが organizer.ts に切り出され UI が取り残されている。42 個の state が同じスコープにあるため「移動フォームを閉じたときに生成結果も消えるか」といった問いにコードから答えられない。責務の境界は既に見えているので設計判断ではなく作業。

**対象** — `web/src/keys/KeysScreen.tsx`, `web/src/keys/organizer.ts`

**手順**

1. 7 つの独立した JSX ブロックをコンポーネントへ抽出する: GenerateKeyForm(1434-1522 + 1498 の結果 + 1523 のハードウェアコマンド) / ChangePassphraseForm(1383) / StoredPassphrasePanel(930) / AgentRegisterForm(1175) / RelocateKeyForm(1268 + 1313) / RevealKeyDialog(1374 + 1056) / TrashPanel(1007 + 1537)
2. KeysScreen には一覧・絞り込み・一括移動(596-928)と、api と reload() を子へ渡す責務だけを残す（300 行程度）
3. organizer.ts の Folder / MoveTarget（MoveTarget = Exclude<Folder,{kind:"all"}> で「絞り込みを外すことは置き場ではない」を型に落としている）は良い形なので触らない
4. C26 の HandoffContext が先に入っていることを確認する（生成鍵→Connections、公開鍵→Remote Keys の受け渡しが子コンポーネントから直接できる必要がある）

### C29 ConnectionTree を connectionBrowser の index に載せ替え、identityKey の 5 実装を 1 つにする

**merge** / 規模 M / 危険度 medium

**理由** — 確認済み。2026-08-11 の計画が ConnectionTree を削除し ConnectionBrowser を作ると決め、翌 08-12 の計画がそれを差し戻した——その 1 日の反転で両方の産物が残った。ConnectionTree.tsx:56 の nearestParent と connectionBrowser.ts:55 の nearestDeclaredParent は識別子名以外一字も違わず、hostMatches(:46) と matches(:69) は alias/patterns/group/tags の 4 条件が同一、metadata 結合 → order→sourceOrder 二段ソート(:83-107 と :97-120) も同手順。ConnectionTree が connectionBrowser から取っているのは duplicateAliasesOf だけ。identityKey は connectionBrowser.ts:52 / ConnectionTree.tsx:42 / ConnectionTree.tsx:86（同ファイル内で再実装）/ QuickConnectBrowser.tsx:48 / OrphanPanel.tsx:12 の 5 箇所にあり、**OrphanPanel.tsx:12 だけはソースに生の NUL バイトが埋まっている**（rg が binary と判定、xxd で `${path}` の直後に 0x00）——grep / diff / エディタから見えない。さらに Go 側に第 3 の重複があり、HostEntry.Duplicate（projection.go:69,220）を api へ載せているが web は読まず（`rg 'server.duplicate\b'` 0 件）自前の duplicateAliasesOf を使う。しかも意味が違い、Go は「影に隠れた側」だけ、web は「組の全員」を立てる。

**対象** — `web/src/connections/ConnectionTree.tsx`, `web/src/connections/connectionBrowser.ts`, `web/src/connections/OrphanPanel.tsx`, `web/src/overview/QuickConnectBrowser.tsx`, `internal/application/projection.go`

**手順**

1. identityKey を connectionBrowser.ts の唯一の export とし、5 箇所と 4 つの resetKey 合成（HostDetail.tsx:74, AdvancedSettings.tsx:41, ConnectionBasicForm.tsx:88, ManageConnection.tsx:36）をそこへ寄せる。OrphanPanel.tsx:12 の生 NUL バイトはこれで消える
2. ConnectionTree を buildConnectionBrowserIndex の上に載せ直す。**1 点だけ差を保つこと**: connectionBrowser は alias==="" のブロックを捨てる(:105 の filter)が ConnectionTree はそれを `Host <patterns>` として表示する(:44)——管理ツリーはパターンだけのブロックを見せねばならないので index 側に includeUnnamed を足すかフィルタを呼び出し側へ移す
3. GroupNameOrder（深い順・Include 用）と web の treeOrder（親が先・画面用）は**意図的に別の順序なので統合しない**——統合すると生成される Include の優先順位が壊れる
4. Go の HostEntry.Duplicate は web が読まないので api から外すか、意味（影に隠れた側 vs 組の全員）を決めて片方に寄せる

### C30 依存方向・重複禁止を internal/acceptance の機械検査として固定する

**introduce-seam** / 規模 M / 危険度 low

**理由** — 確認済みの前提: 依存グラフに循環は 1 件も無く（36 パッケージ 106 辺を DFS 検証、テストファイルを含めても 0）、internal/effective の非テストは internal/config しか import せず、internal/terminal は out-degree 0 の葉という強い局所性が既にある。この良い状態を維持する仕組みが無いことが問題で、既に platform/macos → secret の逆流と httpserver → storage/envelope/objectstore の飛び越しが入り込んでいる。このリポジトリには internal/acceptance/programs_test.go が全 .go を歩いて exec.Command を数え、internal/buildcontract が Makefile / workflow YAML の中身を固定するという前例があるので、作法として揃う。C11〜C29 で作った規約を検査にしないと、次のピボットで同じ形が再発する。

**対象** — `internal/acceptance`, `internal/buildcontract`

**手順**

1. 依存方向テストを足す: (1) internal/effective は internal/config 以外を import しない (2) internal/platform 木は sshc/internal のうち platform 木の外を import しない (3) internal/httpserver は internal/storage / envelope / objectstore を import しない (4) cmd/sshc と mobile は internal/config / effective / storage を import しない (5) storage.NewWorkspace / NewManager の非テスト呼び出しは internal/app 内のみ
2. Go の Notice 定数一覧と web の noticeKeys の一致を見る検査を足す（C9）
3. grouppath.go の reservedGroupNames と GroupsPanel.tsx の reserved の一致を見る検査を足す（C21）
4. README の主張を検査する範囲を広げる: internal/acceptance/documentation_test.go は現在「消した入口の名前が残っていないか」だけを見ており（語句 10 個程度）、README:329 と :336 のような同一文書内の矛盾も :327 のような数の食い違いも捕まえない。少なくとも programs_test.go の allowlist 件数と README の記述の一致は機械で確かめられる
5. build tag の述語を unix / windows の 2 語に統一する（現在 unix 10 / !windows 9 / !unix 1 に割れ、internal/storage の中でも filesystem_unix.go が !windows、journal_fixture_unix_test.go が unix と食い違う）

## 8. 付録 B — 横断分析の所見

### 層構造 / 依存の向き

rg で全 Go ファイル（非テスト）の import を機械抽出し、36 パッケージ・106 辺の依存グラフを再構成した。**循環は 1 つも無い。テストファイル（外部テストパッケージ含む）を加えても 0 件である。**これは確認済みの事実で、「設計が破綻している」という前提はグラフの形については当たっていない。

しかし「循環が無い」ことと「層になっている」ことは別である。トポロジカル深さを計算すると、破綻は 4 つの形で現れる。

**(1) internal/platform は層ではなく、全高度から届く道具袋である。** 深さは 1（最下層の 1 つ上）だが、これを import する 11 パッケージは深さ 2（storage）から 8（cmd/sshc）まで 6 段にまたがる。非テストの import 元 29 ファイルを実際に使っている記号で分類すると、13 ファイルは ValidateAlias/Hostname/Port とそのエラー値しか使わず、2 ファイルは LocalAccountName だけ、7 ファイルは KeyAgent/Toolchain という interface 宣言だけ、1 ファイルは死蔵の OutputRunner。**build tag に裏打ちされた本物の OS 関数（LoginShell/LoginArgv0/LoginArguments/LocalAccountName）を呼ぶ本番ファイルは 4 つしかない。**内訳も同じで、直下 430 行のうち OS 条件付きは 191 行（44%）、残り 239 行は regexp と path/filepath だけの純粋なロジックである。結果として、IPv6 ホスト名を許すという純粋な入力検証の変更が OS 抽象パッケージの diff になった（1923d79 → internal/platform/alias.go:47-59）。

**(2) OS 差分は internal/platform に閉じていない。** build tag を持つ本番ファイルは 49 本あり、そのうち internal/platform 木の下にあるのは 19 本だけ。残り 30 本は cmd/sshc(12), handoff(4), keys(4), enginelock(2), storage(2), terminal(2), sshclient(2), diagnostics(2) に散っている。syscall / golang.org/x/sys / os/exec / os/user を直接 import する非テストパッケージも 8 つあり、internal/platform はその 1 つに過ぎない。「上位層が GOOS を知らずに済む」という目的は達成されていない。

**(3) platform 木と domain 木が相互依存しており、その帰結として「platform」パッケージが usecase と同じ深さに居る。** internal/platform/macos → internal/secret（biometric_darwin.go:18）と internal/secret → internal/platform（vault.go:23）で辺が両方向に張られている。Go のパッケージ単位では別物なので循環にならないが、深さ計算では internal/platform/macos が 4（= internal/application と同じ）になる。しかも internal/platform → internal/platform/windows（shell_windows.go:8）が既に在るので、spec が「サブ 3」として予告している Windows Hello の Guardian を macos の前例どおり internal/platform/windows に置いた瞬間、platform → platform/windows → secret → platform という**本物の import 循環**になる。

**(4) usecase 層が 8 サブシステム中 1 つにしか存在しない。** internal/application は「設定エンジンとトランザクションマネージャの間の use case」（paths.go:1-4）と自称し、実際 config 系は config → effective → application → httpserver の 4 段を持つ。ところが httpserver の非テスト 19 ファイルのうち internal/application を import するのは 6 つだけで、残り 13 は secret / keys / knownhosts / remotesync / terminal / diagnostics / remotekey / sshclient、さらには storage / envelope / objectstore を直接触る。ルート数で見ると 82 のうち 34 が application を経由しないファイルに登録されている。極端なのが sync.go で、objectstore.Client を自分で組み立て（:207）envelope.Derive を直接呼ぶ（:290）。password.go:125 の changeMasterPassword は secret の rekey と remotesync の再封印を 1 つの usecase として束ねているが、その usecase は HTTP ハンドラの中に居る。

**機能追加コストの実測。**「編集可能な接続の基本設定」（c002045/c580c54/36c5110/d4ae452/2839ca0/1923d79）は 39 ファイル、うち Go は 16。「接続の鍵パスフレーズ」は 40 ファイル、Go は 18。直近の biometric（7dd40d9）は 44 ファイル。API に出る機能はどれも api/openapi.yaml → internal/api/models.gen.go → internal/httpserver/*.go → internal/app/run.go → domain の 5 点を必ず通る。ファイル別 churn では internal/app/run.go が 51 コミット、internal/httpserver/server.go が 50 コミットで突出しており、この 2 ファイルが全機能のボトルネックである。ただし config 系の 4 段構えは機能あたり 16-18 Go ファイルを要求するのに対し、2 段しかない sync や terminal は 10-13 で済んでおり、**層を深くした側が明確に高い**。層の深さが払っているのは「無損失パーサ + トランザクション + 3 者マージ」という config 特有の複雑さで、それ自体は正当だが、同じ深さを他の 7 サブシステムに要求していないため、読み手はどの usecase がどこに居るかをコードから予測できない。

**あるべき層の並び（提案）。**
L0 `internal/platform/{nativepath, windowsacl, windowspipe, windowsregistry, process}` — OS API のみ。
L1 `internal/platform`（現状から alias.go / directory.go / sanitise.go / command.go を抜き、LoginShell・LocalAccountName・KeyAgent・Toolchain・Guardian の 5 interface だけにする）。**Guardian の interface はここへ移し、platform/macos が secret を import する辺を消す。**
L2 `internal/storage`（Digest と loader.go を追い出す）, `internal/envelope`, `internal/objectstore`, `internal/enginelock`, `internal/handoff`。
L3 `internal/config`, `internal/effective`, （新設）`internal/validate` — ValidateAlias/Hostname/Port と Digest の置き場。
L4 domain services: `internal/keys`, `internal/knownhosts`, `internal/secret`, `internal/terminal`, `internal/sshclient`, `internal/remotesync`, `internal/remotekey`, `internal/diagnostics`。
L5 usecase: `internal/application` を config 専用から改名・拡張し、sync の rekey+reseal や vault 変更のようなサービス横断トランザクションをここへ引き上げる。
L6 transport: `internal/httpserver`, `internal/session`, `internal/api`, `internal/ui`。**L2 への直接 import（storage 記号・envelope・objectstore）を禁じる。**
L7 composition: `internal/app`。**Workspace/Manager/Resolver を組む場所をここ 1 か所にし、cmd/sshc/list.go と app/ssh.go の独自組み立てを廃す。**
L8 entry: `cmd/sshc`, `mobile`。**config/effective/application/storage への直接 import を禁じ、app が公開する口だけを使わせる。**

**確度の高い指摘**

- **internal/platform は層ではなく全高度から届く道具袋になっている（29 import 元のうち 22 は OS を必要としていない）**（confirmed）
  - 場所: `internal/platform/alias.go:39`, `internal/platform/alias.go:47`, `internal/platform/directory.go:1`, `internal/platform/sanitise.go:16`, `internal/platform/keyagent.go:34`, `internal/platform/command.go:49`
  - 影響: 「上位層が GOOS を知らない」ための境界のはずが、実際にはドメイン層（secret, knownhosts, remotekey）とトランスポート層（httpserver 7 ファイル）が OS 抽象パッケージを名指しで import している。読み手は internal/platform を見た瞬間に OS 依存を疑うが、22/29 の import はそうではない。逆方向の実害も出ており、IPv6 ホスト名を許すという純粋な入力検証の変更（コミット 1923d79）が OS 抽象パッケージの diff になった。
  - 対策: alias.go / directory.go / sanitise.go を internal/platform から出し、新しい葉パッケージ（例: internal/validate と internal/homepath）に移す。internal/platform には LoginShell 系・LocalAccountName・KeyAgent・Toolchain・（後述の）Guardian という「OS ごとに実装が変わるもの」だけを残す。この 1 手で internal/platform の import 元は 29 → 7 まで落ち、依存グラフ上「platform を import しているパッケージは OS を触る」という読み方が回復する。
- **platform 木と domain 木が相互依存しており、予告済みの Windows Hello 実装で本物の import 循環になる**（confirmed）
  - 場所: `internal/platform/macos/biometric_darwin.go:18`, `internal/secret/vault.go:23`, `internal/secret/biometric.go:39`, `internal/platform/shell_windows.go:8`, `docs/superpowers/specs/2026-08-19-biometric-unlock-design.md:186`
  - 影響: 現時点で Go の循環にはなっていないが、これは macOS 実装がたまたま internal/platform/macos という別パッケージに居るからにすぎない。設計文書 :186 は Windows Hello を「サブ 3」として予告しており、macos の前例どおり internal/platform/windows に Guardian 実装を置けば platform → platform/windows → secret → platform で**コンパイルが通らなくなる**。そのとき初めて設計の誤りに気づくことになり、修正は secret の公開 API 変更を伴うため高くつく。
  - 対策: Guardian interface と ErrNoGuardian / ErrRefused / ErrNoBiometric / ErrEmptySecret を internal/secret から internal/platform へ移す（KeyAgent / Toolchain と同じ配置規約に揃える）。secret 側は platform.Guardian を受け取るだけにする。これで platform/macos → secret の辺が消え、platform 木は domain を一切知らない葉に戻る。
- **usecase 層が 8 サブシステム中 1 つにしか無く、82 ルート中 34 が internal/application を通らない**（confirmed）
  - 場所: `internal/application/paths.go:1`, `internal/httpserver/sync.go:207`, `internal/httpserver/sync.go:290`, `internal/httpserver/password.go:125`, `internal/httpserver/knownhosts.go:1`, `internal/httpserver/terminal.go:1`
  - 影響: 同じ HTTP 境界の中で「usecase 層を通す」経路と「通さない」経路が並存しているため、ある機能の業務ロジックがどこにあるかをコードの配置から予測できない。sync の rekey+reseal のようにサービスを 2 つまたぐ処理が HTTP ハンドラに置かれると、CLI（/cli/vault/*）から同じ順序を再現する保証が型に無く、実際 vault_cli.go は独自の経路を持っている。また application を通らない経路では、application が持つ再パース・再解決バリデータ（validate.go）や履歴・コンフリクト変換の恩恵も受けない。
  - 対策: (a) 短期: sync.go の objectstore.Client 構築（:75-84, :190-211）を remotesync.Service 側の Configure/Reach に押し込み、httpserver から internal/objectstore と internal/envelope の import を消す。(b) changeMasterPassword（password.go:125-160）を internal/application（または新設の usecase パッケージ）へ移す。(c) 中期: internal/application を「config 専用の usecase」から「アプリ全体の usecase」に位置づけ直し、keys/secret/remotesync/knownhosts のサービス横断処理をここへ集約する。少なくとも「httpserver は internal/storage / envelope / objectstore を import しない」という 1 本の規則を acceptance テストで固定する。
- **~/.ssh を開く合成の根が 4 つあり、互いに等価でない（Seal・Scanner・解決器が食い違う）**（confirmed）
  - 場所: `internal/app/run.go:197`, `internal/app/run.go:142`, `internal/app/run.go:227`, `internal/app/run.go:265`, `internal/app/ssh.go:211`, `internal/app/ssh.go:215`
  - 影響: 「~/.ssh を開くのは 1 か所」という不変条件が型でもテストでも守られていない。internal/app/ssh.go:26-33 のコメント自身が「組み立てる場所はここひとつである…二箇所で組み立てると、片方だけが vault を見る日が来る」と警告しているのに、同じファイルの :211-217 が 2 つ目を作っている。実際に食い違いが 3 つ（Seal・Scanner・解決器）出ており、いずれも「テストは通るが本番の挙動が違う」型の破綻である。
  - 対策: storage.NewWorkspace / NewManager / NewResolver を internal/app の 1 関数（例: app.OpenWorkspace(home) が Workspace・Manager・Resolver・application.Service を組んで返す）に閉じ込め、他から呼べないよう cmd/sshc と app/ssh.go の直接呼び出しを廃す。差分（鍵 vault は設定バリデータを付けない等）はその関数の引数で表現する。acceptance に「storage.NewWorkspace / NewManager の非テスト呼び出しは internal/app 内のみ」を固定するテストを足す（既存の programs_test.go と同じ手法が使える）。

### 抽象化の不足 / 重複 / 巨大化

実測の結論から先に書く。**このリポジトリの重複は「OS 分岐のコピペ」ではない。** cmd/sshc と internal/platform の build tag ペアを実際に diff すると、本当に同一なのは signals_*（本体 6 行が完全一致）と ownershipMonitor.Stop()（15 行が逐語一致）と launchBackground（4 実装・呼び出し 0）だけで、vault_terminal_*（termios+poll vs ConsoleMode+UTF-16）、ownership の watch 本体（poll vs PeekNamedPipe）、platform/shell_*（実行ビット vs 信頼された既知フォルダ）、nativepath/fold_*（恒等 vs SimpleFold 軌道）は substantive 行の共有が 3〜11 行しかなく、統合すればプラットフォーム差が確実に潰れる。OS 分岐は健全である。

**実際の重複は 3 種類に集中している。**

(1) **抽象が既に存在するのに、後から足した呼び出し側がそれを使っていない。** `commitPlannedRequestWith`（connectioncreate.go:369）があるのに Save / commitGroupPlan / RelocateKey が同じ 20 行を書き直し、4 つとも挙動が違う。`closeAll`（client.go:179）があるのに 4 箇所が同じ逆順ループを手書き。`decodeBody`（バッファ消去付き）があるのにマスターパスワードを受ける 3 経路が消去しない `decodeJSON` を使う。`Resolution.Accepted`（resolve.go:35）が Source と同じ出所情報を持つのに、`effective.Project` が本番 9 箇所から呼ばれ続けている。**これは「共通化すべき」ではなく「共通化が完了していない」。**

(2) **Go と Web が同じ規則を別々に実装し、既に食い違っている。** グループ名の予約語が Go 10 個・Web 6 個（`rc` / `environment` / `known_hosts2` / `authorized_keys2` が Web に無い）で、先頭文字ルールは逆向きにずれている。OpenSSH の argv_split が config/token.go と values.ts に二重にあり、**render 側だけ非対称**——Go は `"` を含む値を `ErrUnquotableValue` で拒むが TS は壊れた引用を生成する。IPv6 検証は Go の `net.ParseIP` に対し TS が 17 行の自前パーサを持ち、コメント自身が「Go の境界と揃えておくこと」と手動同期を宣言している。alias 検証は 4 実装・長さ上限 3 種。web の API 基盤（asRecord/asArray/asString/asNumber/asBoolean/toProblem/jsonHeaders/issueAction）は 4 モジュールに逐語コピー。

(3) **巨大化は「責務が多い」ではなく「union が型になっていない」ことに起因する。** `EditRequest`（service.go:82）は 12 の EditKind が 14 個の optional フィールドを分け合う平坦な union で、その結果 dispatch が plan() の switch・validateEditRequest の 97 行 switch・EditKind 定数表の 3 箇所に散り、plan* が service.go と fileops.go に恣意的に分かれ、service.go が 1426 行になっている。ConnectionsPage の 1018 行の中核は 12 行の状態リセットが 4 回コピーされた useEffect で、これは useReducer 1 つで消える。KeysScreen の 1600 行は useState 42 個で、8 つの独立したワークフローに機械的に割れる。App.tsx の 23 prop バッグは、その 11 個が「画面間ハンドオフ」という 1 つの概念である。

なお最大の関数はフロントエンドではなく **storage/transaction.go:320 の `commit()`（387 行）** であり、内部の 6 フェーズには既にコメントの見出しが付いている。

**共通化してはいけないもの**も明示しておく: GroupNameOrder（深い順・Include 用）と web の treeOrder（親が先・画面用）は意図的に別の順序であり、統合は生成される Include の優先順位を壊す。ConnectionTree が `alias === ""` のブロックを表示し connectionBrowser がそれを捨てるのも、管理ツリーと接続先ブラウザという用途の違いであり残さねばならない。

**確度の高い指摘**

- **設定解決エンジンが 2 本並走している（Project 9 箇所 / Resolve 2 箇所）。統合先の型は既に出所情報を持っている**（confirmed）
  - 場所: `internal/effective/provenance.go:106`, `internal/effective/resolve.go:95`, `internal/effective/resolve.go:30`, `internal/effective/provenance.go:48`, `internal/application/passwordeligibility.go:82`, `internal/application/connectionupdate.go:406`
  - 影響: 同じ問いに 2 つの答えがあり、既に食い違っている。Project は Match ブロックを一切適用せず complexity に積んで return するだけ（provenance.go:118-127、blockApplies が BlockMatch に常に ("",false) を返す provenance.go:252）なので、`Match host db` 配下の `PasswordAuthentication no` は passwordeligibility.go:120 の projection.Value() に現れない。一方 Resolve は同じ設定で Match を評価する（resolve.go:179-204）。つまり接続経路と資格情報の可否判定が別の答えを出す。また Project には既定値の充填が無いため、diagnostics/service.go:116 と :192、cmd/sshc/tui.go:64 が "22" を各自で埋めている。
  - 対策: Resolve を単一の権威にする。ただし**素朴な削除では 2 つ失われる**ので順序が要る: (1) `Accepted` に `Winner bool` を足し、影に隠れた側も `Shadowed []Accepted` として返す（web/src/diagnostics/DiagnosticsPanel.tsx:247,255 が `source.winner` で opacity-60 と "superseded" 表示を出しており、これが唯一失われる UI である）。(2) `Source.Kind` は捨ててよい——httpserver/diagnostics.go:123 が api へ載せているが DiagnosticsPanel は読んでいない（`source.kind` の参照 0 件）。(3) その上で 9 箇所を Resolve へ移し、provenance.go の Project / walkLoadOrder / blockApplies を削除する。effective.Cumulative（provenance.go:32、本番参照 0）も同時に消える。
- **commit + ConflictError 変換が 4 回書かれ、抽象は既に存在するのに 2 つが使っていない。しかも 4 つとも挙動が違う**（confirmed）
  - 場所: `internal/application/service.go:532`, `internal/application/grouprename.go:63`, `internal/application/connectioncreate.go:369`, `internal/application/keymove.go:124`, `internal/application/service.go:556`, `internal/application/grouprename.go:80`
  - 影響: grouprename.go:62 のコメントは「保存と同じコミット経路に通す」と書いているが、実際には経路を共有せず複製している。4 つの微妙な差はどれも意図的とは読めず、片方だけ直す修正が今後も続く。特に (c) は、鍵の再配置だけがジャーナルの外でディレクトリを作るという、storage 層の設計意図（transaction.go:626 付近のコメントが「これがトランザクションの外の EnsureDirectory でない理由のすべてである」と書く）に正面から反する。
  - 対策: `commitPlannedRequestWith` を唯一の入口にする。(1) prepared に `keyRelocations` と `directories` は既にあるので、requestFor がそれらを運ぶよう揃え、keymove の独自 Request 組み立てを消す。(2) 戻り値を SaveResult に統一し、KeyRelocations は常に載せる（json:"-" なので HTTP 応答は変わらない）。(3) 「base に無いパスは設定ファイルではない」（connectioncreate.go:381）を全経路の既定にする——これが 4 つの中で最も正しい挙動で、他 3 つは非設定ファイルの衝突に対して空の三者 diff を作る。httpserver 側は `storageProblem(c, err) (error, bool)` を security.go か共通ファイルに 1 つ置き、各 xxxProblem の末尾でそれを呼ぶ。
- **web の API クライアント基盤（実行時ガード・Problem 変換・ヘッダ・action token）が 4 モジュールに逐語コピーされている**（confirmed）
  - 場所: `web/src/api/config.ts:35`, `web/src/api/integrations.ts:143`, `web/src/keys/api.ts:99`, `web/src/remotekeys/api.ts:26`, `web/src/api/integrations.ts:352`, `web/src/api/integrations.ts:358`
  - 影響: 失われるものは何も無い純粋な重複である。実行時ガードは「サーバーが契約を破ったら UI が壊れる前に止める」という安全機構で、その定義が 4 つあるということは、たとえば「null を record として通さない」の修正が 1 箇所にしか入らない状態が常に起こりうるということ。toProblem が画面側にあるのも層違いで、ApiError を Problem に正規化するのは client.ts の仕事である。
  - 対策: web/src/api/client.ts に `asRecord/asArray/asString/asNumber/asBoolean/asNonnegativeInteger`、`toProblem`、`jsonHeaders`、`issueAction` を集約し、4 つの API モジュールと 4 つの画面がそこから import する。機械的でリスクゼロ、影響範囲はテストを含めても import 行の書き換えだけ。同時に keys/api.ts と remotekeys/api.ts を web/src/api/ 配下へ移し、API レイヤが 3 ディレクトリに散っている状態も解く。
- **ConnectionTree が connectionBrowser の index 構築を再実装。identityKey は 5 実装あり、うち 1 つはソースに生の NUL バイトを含む**（confirmed）
  - 場所: `web/src/connections/ConnectionTree.tsx:42`, `web/src/connections/ConnectionTree.tsx:56`, `web/src/connections/ConnectionTree.tsx:86`, `web/src/connections/ConnectionTree.tsx:90`, `web/src/connections/connectionBrowser.ts:52`, `web/src/connections/connectionBrowser.ts:55`
  - 影響: docs/superpowers/plans/2026-08-11 が ConnectionTree を削除し ConnectionBrowser を作ると決め、翌 08-12 の計画がそれを差し戻した——その 1 日の反転で両方の産物が残った。connectionBrowser.ts:86 のコメントは「2 つの画面がこの印を出す。規則が 2 か所にあると、片方だけが直る」と重複を自認しているが、**同じ規則の 3 つ目が Go にあり、しかも意味が違う**ことには気づいていない: Go の Duplicate は「影に隠れた側」だけを立て、web の duplicateAliasesOf は「組の全員」を立てる。同じ `duplicate` という語が 2 つの意味で存在し、片方は配線されていない。生 NUL バイトは grep / diff / エディタから見えず、Windows のツールチェーンによっては壊れる。
  - 対策: (1) `identityKey` を connectionBrowser.ts の唯一の export とし、5 箇所と 4 つの resetKey 合成（HostDetail.tsx:74, AdvancedSettings.tsx:41, ConnectionBasicForm.tsx:88, ManageConnection.tsx:36）をそこへ寄せる。OrphanPanel.tsx:12 の生 NUL は同時に消える。(2) ConnectionTree を `buildConnectionBrowserIndex` の上に載せ直す。**ただし 1 点だけ差を保つこと**: connectionBrowser は `alias === ""` のブロックを捨てる（:105 の filter）が ConnectionTree はそれを `Host <patterns>` として表示する(:44)——管理ツリーはパターンだけのブロックを見せねばならないので、index 側に `includeUnnamed` を足すか、フィルタを呼び出し側へ移す。(3) Go の `HostEntry.Duplicate` は web が読まないので、api から外すか、web を server 側の値へ寄せる。後者を選ぶなら Go 側を「組の全員」に変える必要がある（現在の credentialusage.go:41 は「影に隠れた側」の意味で使っているので、そちらは別フィールドに分ける）。
- **グループ名の検証が Go と Web で二重実装され、既に両方向へ食い違っている**（confirmed）
  - 場所: `internal/application/grouppath.go:40`, `internal/application/grouppath.go:59`, `internal/application/grouppath.go:75`, `web/src/groups/GroupsPanel.tsx:79`, `web/src/groups/GroupsPanel.tsx:80`, `web/src/groups/GroupsPanel.tsx:83`
  - 影響: GroupsPanel.tsx:75-78 のコメントは、この重複の目的を「"invalid_request" としか言わない往復通信の前に、パネルがどの文字が間違っているかを言えるようにするため」と明記している。ところが `rc` や `environment` というグループ名はパネルを通過してサーバーに拒否されるので、**その目的が 4 つの名前について達成されていない**——コメントが防ぐと言っている体験がそのまま起きる。逆に `-foo` は画面が先に拒むがサーバーは受け入れる。どちらの規則が正なのかコードから決められない。
  - 対策: Go を正とし、規則を機械で同期させる。最小の形は、GET /api/v1/config/overview のレスポンス（既に metadata を運んでいる）に `groupNamePolicy: { reserved: string[], maxSegments: number, maxSegmentBytes: number, segmentPattern: string }` を足し、Web が正規表現をリテラルで持つのをやめること。それが重いなら、`internal/acceptance` に「grouppath.go の reservedGroupNames と GroupsPanel.tsx の reserved が一致する」ことを表明するテキスト照合テストを置く——このリポジトリは既に internal/buildcontract で Makefile / workflow YAML の中身を固定する同種のメタテストを持っており、作法として揃う。あわせて先頭文字の規則をどちらかに寄せる（Go 側を `^[A-Za-z0-9]` に狭めるのが安全側）。
- **OpenSSH の引用規則（argv_split）が Go と TS に二重実装され、書き出し側だけ非対称になっている**（confirmed）
  - 場所: `internal/config/token.go:23`, `internal/config/token.go:41`, `internal/config/token.go:20`, `web/src/connections/values.ts:5`, `web/src/connections/values.ts:34`, `web/src/groups/GroupsPanel.tsx:5`
  - 影響: エディタが受け付けた値をエンジンが拒む、あるいはその逆が起きる。formatValues は ConnectionsPage / GroupsPanel の高度設定欄で使われており、`ProxyCommand` に `"` を含むコマンドを書いた利用者には、画面上は正常に見えて保存が invalid_request で落ちる経路がある。IPv6 側も同型で、`::ffff:01.2.3.4` は TS の validIPv4(`/^\d{1,3}$/`) を通るが Go の net.ParseIP は先頭ゼロを拒む。
  - 対策: Go を正本とし、Web からは「規則の実装」を消す。(1) formatValues に RenderArgument と同じ拒否条件（`"` / 改行 / NUL）を足すのが最小修正で、これだけで書き出し側の非対称は消える。(2) 恒久策としては、値の分割と再描画をサーバー側の責務にする——既に EditRequest.Fields は `values []string` を運ぶので、テキスト⇄配列の変換だけを POST /api/v1/config/preview の往復で済ませられる。ただし入力のたびに往復するのは重いので、(1) を先に入れ、(2) は「TS 側は緩く受けて、拒否理由はサーバーの Problem を表示する」形に倒すのが現実的。hostValidation.ts も同様に、IPv6 の判定は緩く（`:` を含むなら通す）してサーバーに委ねる方が、17 行のパーサを 2 つ保守するより安全。
- **storage.Manager.commit が 387 行で、リポジトリ最大の関数。内部フェーズには既に見出しコメントが付いている**（confirmed）
  - 場所: `internal/storage/transaction.go:320`, `internal/storage/transaction.go:346`, `internal/storage/transaction.go:361`, `internal/storage/transaction.go:502`, `internal/storage/transaction.go:626`, `internal/storage/transaction.go:642`
  - 影響: ジャーナル・世代バックアップ・封印・原子的 rename・中断復旧という、このアプリケーションで最も壊れ方が静かな領域が 1 つの関数に収まっている。フェーズ間で共有される可変状態（entries / staged / previous の 3 スライスが添字対応を保つ、:642 のコメントがそれを明記）が暗黙の不変条件になっており、フェーズを跨いだ変更が安全かどうかを読む側が全体を保持しないと判断できない。
  - 対策: 見出しコメントの位置がそのまま分割線になる。`type commitPlan struct { entries []journalEntry; staged, previous []string; directories []string }` を導入し、`planDirectories(request) → resolveChanges(plan) → planDirectoryRemovals(plan) → createDirectories(plan) → backupPrevious(plan) → stageFiles(plan) → applyRenames(plan)` の 7 メソッドへ切る。3 スライスの添字対応は commitPlan のフィールドとして表現すれば、暗黙の不変条件が型の中に入る。commit 本体は各フェーズの順序とロールバックの判断だけを残す（30 行程度）。
- **KeysScreen.tsx が 1600 行・useState 42 個で、8 つの独立したワークフローを 1 コンポーネントに抱えている**（confirmed）
  - 場所: `web/src/keys/KeysScreen.tsx:196`, `web/src/keys/KeysScreen.tsx:684`, `web/src/keys/KeysScreen.tsx:930`, `web/src/keys/KeysScreen.tsx:1007`, `web/src/keys/KeysScreen.tsx:1056`, `web/src/keys/KeysScreen.tsx:1175`
  - 影響: 42 個の state が同じスコープにあるため、どの操作がどれを触るかが読めない。実際 `closeAgentForm` のようなクロージャが複数の群にまたがって state をリセットしており、たとえば「移動フォームを閉じたときに生成結果も消えるか」といった問いにコードから答えられない。責務の境界は既に見えているので、これは設計判断ではなく作業である。
  - 対策: 上の 8 群をそのままコンポーネントへ切る。7 つは既にモーダル/フォームとして独立した JSX ブロックなので機械的に抽出できる: `<GenerateKeyForm>`(1434-1522 + 1498 の結果 + 1523 のハードウェアコマンド)、`<ChangePassphraseForm>`(1383)、`<StoredPassphrasePanel>`(930)、`<AgentRegisterForm>`(1175)、`<RelocateKeyForm>`(1268 + 1313 の結果)、`<RevealKeyDialog>`(1374 + 1056)、`<TrashPanel>`(1007 + 1537)。KeysScreen には一覧・絞り込み・一括移動（596-928）と、`api` と `reload()` を子へ渡す責務だけが残り、300 行程度になる。organizer.ts の Folder / MoveTarget は既に良い形（`MoveTarget = Exclude<Folder, {kind:"all"}>` で「絞り込みを外すことは置き場ではない」を型に落としている）なので触らない。

### プラットフォーム分岐の設計（Windows / macOS / Linux / Android）

**一枚の絵：4 OS はどこで、どう分かれているか**

**分岐手段は実測で 7 通りあり、うち 3 通りが「同じ問い」に別々に答えている。**

| 手段 | 実測箇所 | 主な使われ方 |
|---|---|---|
| build tag | 非テスト 50 ファイル（windows 22 / `unix` 10 / `!windows` 9 / linux 3 / darwin 3+暗黙1 / `!unix` 1 / `!darwin&&!linux&&!windows` 1）。**android タグは 0 件**（`rg 'go:build.*android'`）。同名 per-OS 関数ペアは 36 組 | OS 原始操作（PTY・flock・DACL・named pipe・no-echo 入力・シグナル） |
| build tag 付き LOC | windows 3337 / unix 763 / !windows 221 / darwin 193 / linux 165 → **build tag 付き非テストコードの約 72% が Windows** | — |
| runtime.GOOS | **製品コードで 1 箇所だけ**（`internal/platform/shell_unix.go:45`、Android のシェル選択）。他は `internal/buildcontract`（ビルド CLI、出荷物に載らない） | Android 唯一の実行時分岐 |
| interface 差し替え | 6 個（`platform.Toolchain` / `platform.KeyAgent` / `secret.Guardian` / `terminal.Starter` / `desktopLauncher` / `ownershipMonitor`） | 「OS の道具」の抽象 |
| nil = 機能の不在 | 11 箇所（Toolchain 1・KeyAgent 3・Guardian 6・Updates 1）。**internal/platform 内には 0 箇所** | Android / Windows の欠落表現 |
| 実行時プローブ | 3 個（`Agent.Available` が実 dial、`macos.Biometric.available` が実 `SecItemAdd`、`Toolchain.KeyGen` が stat） | 「本当に使えるか」 |
| 別バイナリ | Electron: `process.platform` 6 箇所 + `app.dock !== undefined` の duck-typing 3 箇所（`desktop/main.js:228,354,413`）／Android Java: **OS 分岐ゼロ**、`Build.VERSION.SDK_INT` の版分岐のみ | 外殻 |

**擁護すべき点を先に置く。** Android 固有の振る舞い 6 点はすべて根拠付きで表現されている——CLI 無し（成果物に含めないだけ）、ssh-keygen/ssh-agent 無し（`mobile/dependencies.go:43-44` の nil）、自己更新無し（同 `:50`）、HOME=filesDir（`EngineService.java:72` が渡す）、`/system/bin/sh`（`shell_unix.go:27`）、CGO 必須（`Makefile:61-68`）。`shellFallbacks(goos)` が引数で goos を取る設計（`shell_unix.go:21-22` に理由明記）は優秀で、Android の表は Linux ホスト上でも `shell_unix_test.go:78` が検査できている。Web UI には OS スニッフィングが 1 行も無く、機能の出し分けはサーバが返す capability boolean（`biometric.available` / `agentAvailable` / catalogue）だけで行われている——**これがこのリポジトリで最も正しい分岐点である。**

**破綻は「同じ種類の差異に違う手段が使われている」ことに集中する。**

1. **「OS が提供する道具の在り処」に 2 つの機構。** ssh-agent は domain 内の build-tag factory（`keys/agent_unix.go:16` / `agent_windows.go:16`）、ssh-keygen は per-OS パッケージ + interface + 合成の根での配線（`platform/{linux,macos,windows}/toolchain.go` → `cmd/sshc/wiring_*.go`）。同じ形の問いに 2 つの答え方があり、**後者は Windows で配線が漏れている**（`wiring_windows.go:23` が Toolchain を nil のまま返す）。実装もテストも完成している `windows.NewToolchain` の製品参照は 0 件。
2. **「デスクトップ外殻の居場所」に 3 つの transport・3 つの配置・3 人の書き手。** macOS = bundle ID 定数（`launch_darwin.go:12`、書き手は OS）／Linux = JSON 記述子（書き手は Electron `desktop/launcher.js:52`、読み手は `launch_linux.go:90` にインライン）／Windows = HKCU（書き手は NSIS `installer.nsh:78`、読み手は専用パッケージ `windowsregistry`）。Go 側の `RegisterDesktopExecutable`/`RemoveDesktopExecutable` は NSIS に役目を取られて参照 0 件。
3. **「機能の有無」に 3 つの手段（build tag / nil / 実行時プローブ）が混在し、型が網羅を強制しない。** `newPlatformParts()` は 3 ファイルにあるが `platformParts` の 3 フィールドを全部埋める義務は無く、テストも無い。同じ形の破綻が `mobile/dependencies.go` にもあり、struct literal なので `app.Dependencies` にフィールドが増えても壊れず、**既に Biometric と ShutdownTimeout が黙って欠けている**（コメントは「落としているものは 4 つ」と書くが実際は 6 つ）。

**4 OS 対応の設計として、あるべき分岐点はどこか。**

答えは 3 層に分けられる。

- **層 1（コンパイル時 / build tag）に置くべきもの＝「OS が別の API を持つ原始操作」だけ。** PTY vs ConPTY、flock vs LockFileEx、unix socket vs named pipe、mode bits vs DACL、termios vs ConsoleMode。ここは実測でも正しく閉じている（`terminal.Starter` は `NewStarter()` という同名 factory 1 本、`storage` の OS 差は 6 関数）。逆に **`signals_unix.go:16-20` と `signals_windows.go:21-25` は本文が 1 バイトも違わず、`ownershipMonitor.Stop()` も 2 実装が逐語同一**——ここは分岐点ではない。
- **層 2（実行時 / capability 値）に置くべきもの＝「その端末に道具があるか」。** これは OS ではなく端末の性質であり（署名の無い macOS ビルドでは Touch ID が無い、ssh-agent が死んでいれば Linux でも無い）、**build tag でも nil でも表現できない。** 正しい形は既に `secret.BiometricState{Available, Enabled}` と `agentAvailable` にある。**足りないのはこれを 1 つの capability 文書に束ねること**で、いま `Toolchain`/`KeyAgent`/`Guardian`/`Updates` の 4 つがそれぞれ別の nil 判定 11 箇所に散り、`internal/platform` 側には 1 箇所も無い。`Capabilities{HardwareKeys, Agent, Biometric, SelfUpdate, CLI bool}` のような明示的な値を合成の根で 1 度だけ組み、3 つの wiring と mobile がそれを**必ず埋める**（ゼロ値が「無い」ではなく、構築関数が全項目を要求する）形にすれば、Windows Toolchain の配線漏れは型で落ちる。
- **層 3（外殻 / 別バイナリ）に置くべきもの＝「engine の寿命を誰が持つか」だけ。** ここは Electron と Android で本質的に違う（子プロセス vs 同一プロセス）ので共通化できない。しかし現状は**共通化できるはずの 3 点まで別々に解いている**：単一エンジン保証（Electron は `requestSingleInstanceLock` + Go の flock、Android は Go の flock のみ、しかもロックパスの組み立てが `cmd/sshc/lock.go:23` と `mobile/sshc.go:70` の 2 箇所）、入口 URL の使い捨て fragment（Electron は `sshc open` を再実行、Android は `EngineService.java:53-55` で fragment を剥がす）、文字列（Electron は日本語直書き、Android は英語 strings.xml、Web は en/ja 1094 キー）。**入口の再発行は engine 側 API 1 本に寄せ、外殻はそれを呼ぶだけにできる。**

最後に一つ、方針として明示すべきこと。**GOOS=android は `linux` タグを満たすため、`cmd/sshc` は Android 向けにコンパイルが通り、`wiring_linux.go` の `linux.NewToolchain()`（`/usr/bin` 等）を掴む。** README:58 はこれを自覚しているが、「Android では Toolchain が nil」という不変条件を守っているのは `mobile/dependencies.go` 1 ファイルだけで、build tag も型も守っていない。層 2 を値にすれば、この不変条件も 1 箇所で表明できる。

**確度の高い指摘**

- **Windows の Toolchain が合成の根で配線されておらず、4 OS のうち Windows だけハードウェア鍵の項目が黙って消える**（confirmed）
  - 場所: `cmd/sshc/wiring_windows.go:22`, `cmd/sshc/wiring_windows.go:13`, `internal/platform/windows/toolchain.go:35`, `internal/platform/windows/toolchain.go:43`, `internal/keys/catalogue.go:74`, `cmd/sshc/engine.go:171`
  - 影響: %WINDIR%\System32\OpenSSH\ssh-keygen.exe を持つ Windows でも FIDO 鍵（ed25519-sk / ecdsa-sk）の項目が画面に出ない。README.md:60 は Toolchain が nil になるのを Android だけの性質として説明し、README の Windows 節（:96-153）には ssh-keygen が使えないことが 1 行も書かれていないので、コード・文書・実物の 3 つが食い違っている。しかも nil が正当な状態として設計されているため、コンパイルも実行も静かに成功する——「機能の有無」を型で表していないことの実害がここに出ている。
  - 対策: wiring_windows.go で `Toolchain: windows.NewToolchain(os.Getenv("WINDIR"))`（信頼の起点は shell_windows.go:45 の systemLookup と同じ経路）を渡す。あわせて 3 つの newPlatformParts が platformParts の全フィールドについて意図を表明していることを検査するテスト（cmd/sshc に wiring_test.go は存在しない）を置くか、platformParts をゼロ値で作れない構築関数に変える。
- **「その端末に道具があるか」を表す手段が nil・実行時プローブ・build tag の 3 通りに割れ、判定が 11 箇所に散っている**（confirmed）
  - 場所: `internal/keys/catalogue.go:74`, `internal/keys/service.go:670`, `internal/keys/service.go:732`, `internal/keys/service.go:776`, `internal/secret/biometric.go:70`, `internal/secret/biometric.go:101`
  - 影響: 「ここで何ができるか」がサブシステムごとに違う語彙で表現されるので、4 OS × 5 機能の組み合わせを一望できる場所がコードにもレスポンスにも無い。新しい OS（あるいは新しい機能）を足すとき、埋め忘れても型は何も言わない——上の Windows Toolchain がまさにその形で漏れている。README も Android 節（:56-62）にだけ表があり、Windows の欠落は書かれていない。
  - 対策: 合成の根（internal/app.build）で `Capabilities{HardwareKeys, Agent, Biometric, SelfUpdate bool}` を 1 度だけ確定させ、各サブシステムはその値を受け取る（自分で nil を判定しない）。GET /api/v1/health か既存の状態レスポンスにその文書を載せれば、UI 側の出し分けも 1 箇所になる。nil interface は「注入し忘れ」と「その OS に無い」を区別できないので、区別が要る場所では bool を運ぶ。
- **Android は build tag を 1 つも持たず、不変条件を守っているのは mobile/dependencies.go の struct literal 1 箇所だけ。既に Biometric と ShutdownTimeout が黙って欠けている**（confirmed）
  - 場所: `mobile/dependencies.go:30`, `mobile/dependencies.go:17`, `mobile/dependencies.go:43`, `mobile/dependencies.go:50`, `cmd/sshc/engine.go:172`, `cmd/sshc/engine.go:189`
  - 影響: 「Android では Toolchain と KeyAgent が nil」という README:60 の約束を守っているのは 1 ファイルの 2 行だけで、build tag も型も守っていない。ShutdownTimeout の欠落は Android で defaultShutdownTimeout（4 秒、run.go:113）に落ちるだけなので今は無害だが、Biometric は docs/superpowers/specs/2026-08-19-biometric-unlock-design.md が Android を対象に挙げているので、実装しても受け口が無いことに気づく仕組みが無い。
  - 対策: app.Dependencies を struct literal ではなく、必須項目を引数に取る構築関数（あるいは各フィールドの意図を表明する Options ビルダ）で作り、cmd/sshc と mobile の両方をそこへ通す。少なくとも、両者が同じフィールド集合について明示的な意図（値 or 明示的な nil）を持つことを検査するテストを置く。
- **「デスクトップ外殻の居場所」が 3 OS で 3 つの transport・3 つの配置・3 人の書き手に分かれ、Go 側の登録 API は NSIS に役目を取られて死んでいる**（confirmed）
  - 場所: `cmd/sshc/launch_darwin.go:12`, `cmd/sshc/launch_linux.go:20`, `cmd/sshc/launch_linux.go:90`, `cmd/sshc/launch_windows.go:26`, `desktop/launcher.js:13`, `desktop/launcher.js:52`
  - 影響: 同じ 1 つの問い（端末から打った sshc は、どの実体を起こせばよいか）に 3 つの記録方式と 3 人の書き手が対応し、そのうち 2 つは言語をまたぐ手動同期になっている。Go 側に用意された Register/Remove は使われないまま残り、「Go が書く」と「NSIS が書く」の 2 通りの方針が同時に存在する。片方だけを変えれば、インストールは成功するのに端末から起こせない、という壊れ方をする。
  - 対策: 記録の綴り（キー名・ファイル名・スキーマ）を Go の 1 パッケージに集約し、Electron と NSIS はその値を生成物（例：ビルド時に出力する定数ファイル）から受け取る。使われない RegisterDesktopExecutable / RemoveDesktopExecutable は、NSIS に任せる方針を採るなら削除し、Go に寄せるなら NSIS 側を呼び出しに置き換える——どちらか一方に決める。
- **Windows のパス安全性判定が 3 実装あり、UNC を通すか拒むかで答えが食い違う**（confirmed）
  - 場所: `internal/platform/nativepath/nativepath.go:29`, `internal/platform/nativepath/nativepath.go:36`, `internal/platform/windowsacl/path_policy.go:15`, `internal/platform/windowsacl/path_policy.go:27`, `internal/platform/windowsacl/path_policy.go:77`, `internal/platform/windows/shell.go:81`
  - 影響: 「この綴りを exec に渡してよいか」という同じ危険判断に 2 つの答えがある。片方（登録された外殻の起動）はレジストリの値をそのまま実行する経路で、UNC を通すということはネットワーク共有上の実体を起こしうるということ。方針が 1 つに決まっていないので、どちらが意図でどちらが漏れなのかコードから読めない。
  - 対策: 「exec に渡してよい Windows パス」を 1 つの述語に統合し（最も厳しい trustedProgramPath 相当）、windowsregistry と windowsacl はそれを呼ぶ。UNC を許すかどうかは用途ごとに引数（フラグ）で表し、暗黙の差にしない。isASCIILetter の 2 定義も同時に畳める。

### 過剰な抽象化（seams / 間接層が割に合っているか）

この 13 日間のピボットが残した「過剰な抽象化」は、interface の数そのものではなく、**(a) 1 ビットのために積まれた 5 パッケージ分の層、(b) 本番が誰も設定しない注入点とその nil 分岐、(c) 同じ概念に 2 つの抽象が並走し、どちらも半分しか適用されていない状態** の 3 形に集中している。

最も高くついているのは internal/api の生成モデル層で、203 型のうち 77 型（33 型はドメインモデル、44 型は oapi-codegen の別名）が Go から一度も名指されず、その 33 型のうち 26 型には internal/application に json タグまで一致する手書き双子が実在する（`grep '^type X'` で 26/33 一致を確認）。httpserver は同一パッケージ内で 3 方針を採り、config 系 12 ルートは api 参照 1 件、connections 系 2 ルートは 8 関数・約 300 行の手変換を持つ。

2 番目は platform.Toolchain で、5 パッケージ・本番 148 行＋テスト 246 行が最終的に `if _, err := reader.Toolchain.KeyGen(); err != nil` という **boolean 1 個**にしか使われず、返り値のパスもエラー値も本番で捨てられている（`ErrProgramNotFound` の errors.Is は全件テスト、`Stat` の代入も全件テスト）。

3 番目は「本番が誰も設定しない注入点」で、app.Dependencies の 5 フィールド、terminal.Spec.Cleanup、sshclient.Dialer.Dial / Auth.ReadFile、keys.ServiceOptions.StoredPassphrase、process.Toolchain.Stat、objectstore.Client.HTTP / RequestTimeout、httpserver.PasswordHandlers.ResealSnapshot / SyncHandlers.Reach、handoff の writeOperations.marshal / handoffFileOperations.read — 計 15 個以上を機械的に確認した。それぞれが本番コードに nil 分岐を 1 つずつ残している。

一方で、疑われがちだが**畳めない**ものも反証した。internal/envelope（2 独立消費者、remotesync→secret 依存を作らない）、internal/remotekey（keys の外向き依存を 3 つに保つ）、internal/ui（go:embed のパス制約）、internal/app（本番 821 行、20 依存→24 フィールドの本物の合成の根）、internal/enginelock（handoff の lock とは意味論が違う）はいずれも統合すべきでない。

**確度の高い指摘**

- **internal/api 生成モデル層: 203 型中 77 型が Go から到達不能、うち 26 型は application に json タグまで一致する手書き双子を持つ**（confirmed）
  - 場所: `internal/api/models.gen.go:1`, `internal/api/models.gen.go:1155`, `internal/application/effective.go:15`, `internal/application/effective.go:23`, `internal/application/projection.go:75`, `internal/application/projection.go:90`
  - 影響: 同じ JSON 形状の定義が Go 側に 2 つ（生成型と application の手書き型）あり、どちらが正本かをコードから決められない。config/metadata/history 系のエンドポイントは application 型を直に c.JSON するので、openapi.yaml と実際のレスポンスがずれても Go 側で検出する手段が無い（TypeScript は openapi.yaml から独立に生成されるので、UI 側だけが契約に追随して壊れる）。make verify-generated は「生成物が仕様と一致するか」しか見ておらず、「実際に返る JSON が仕様と一致するか」は誰も見ていない。
  - 対策: 二択でどちらかに寄せる。(A) 生成型を通信の正本にする: config_handlers.go:26-55 の手書き 6 型（groupRenameRequest / groupDeleteRequest / historyList / restoreRequest / recoverRequest / recoverResponse、約 30 行）を api.GroupRenameRequest / GroupDeleteRequest / HistoryList / RestoreRequest / RecoverRequest / RecoverResponse へ置換し、decodeEdit を api.EditRequest 経由にする。生成物の行数は減らないが未使用型が 33 → 20 前後になり、契約違反がコンパイルエラーになる。(B) Go 側の生成をやめる: models.gen.go 1524 行を丸ごと削除し、httpserver は application 型と手書き request 型のみを使う。TypeScript は無傷だが、現に使われている 126 型・9 ファイル約 190 参照の書き換えが必要。**畳んだ場合に消える行数**: (A) 約 30 行＋曖昧さ、(B) 1524 行＋ contract_test.go 179 行。**失われるもの**: (B) を採ると connections/keys/sync/password 系で「openapi.yaml を変えたら Go がコンパイルエラーになる」という現在唯一の契約保証が消える。したがって (A) を推す。
- **platform.Toolchain: 5 パッケージ・本番 148 行＋テスト 246 行が boolean 1 個にしか使われず、返り値のパスもエラー値も本番で捨てられる**（confirmed）
  - 場所: `internal/platform/command.go:64`, `internal/platform/process/toolchain.go:20`, `internal/platform/process/toolchain.go:22`, `internal/platform/linux/toolchain.go:12`, `internal/platform/macos/toolchain.go:12`, `internal/platform/windows/toolchain.go:35`
  - 影響: interface・3 つの OS 別パッケージ・2 つの sentinel error・1 つのテスト注入フィールドという 4 段の間接が、最終的に「ハードウェア鍵の項目を一覧に出してよいか」という 1 ビットしか運んでいない。しかもその 1 ビットを Windows では誰も配線していないので、層があること自体が欠落を隠している（nil は「機能の不在」として正常に扱われるため、静かに失敗する）。ssh-keygen 以外を足す予定が無い以上、この形は逆に「足すのは簡単そうに見えるが実際は 5 パッケージを触る」という誤ったコストシグナルを出している。
  - 対策: interface と 3 パッケージを畳み、internal/platform に build tag 付きの `func HasKeyGen() bool`（もしくは `KeyGenPath() (string, error)` を 1 実装）を置く。app.Dependencies.Toolchain は `HardwareKeysAvailable func() bool` に変える（Android は nil、Windows は windows 側の実装を渡す。これで未配線バグも同時に直る）。**畳んだ場合に消える行数**: 本番約 110 行（148 → 40 前後）、テスト約 180 行（246 → 60 前後）。internal/platform/linux パッケージは丸ごと消える。**失われるもの**: 「探索ディレクトリの並びは OS ごとに違うが探し方は同じ」という現在の分割が表現していた区別。ただしその区別は 3 要素の配列 2 本でしか使われておらず、build tag 付き 1 ファイルで同じことが書ける。
- **httpserver.KeyService だけが 15 メソッドの interface で、同じ Options が抱える他の 9 サブシステムは全て具象ポインタ。しかも stub はこの interface に無い 2 メソッドも背負っている**（confirmed）
  - 場所: `internal/httpserver/keys.go:44`, `internal/httpserver/server.go:56`, `internal/httpserver/server.go:60`, `internal/httpserver/server.go:63`, `internal/httpserver/server.go:69`, `internal/httpserver/keys_test.go:26`
  - 影響: 15 メソッドの interface は「HTTP 層が必要とする一部分」（keys.go:41-43 のコメント）を名乗っているが実際は全面で、抽象として何も狭めていない。にもかかわらず同じ位置の 9 兄弟は具象型のままなので、読み手は「鍵だけ特別な理由があるのか」を探す羽目になり、答えはコードに無い。1 つの具象型に 2 つの interface が被さっているせいで、テスト stub が「どちらの契約に属するか」不明な 2 メソッドを持ち、削除してよいかを機械的に判定できない。
  - 対策: web/src 側が既にやっているように、ハンドラ群ごとに狭い interface へ分ける: KeyHandlers は 15 全部、ConfigHandlers と ConnectionHandlers は `interface{ Inventory() (*keys.Inventory, error) }` だけ。あわせて keys_test.go の VerifyPassphrase / RevalidatePassphrase を application 側の stub へ移す。**畳んだ場合に消える行数**: interface 宣言自体は増減ほぼなしだが、config_handlers_test.go / connections_test.go が 15 メソッドの stub（keys_test.go:26-141 の約 115 行）を共有する必要が消え、テストの依存が 1 メソッドになる。**失われるもの**: なし。（Keys を具象型 *keys.Service に戻す案もあるが、その場合はハンドラテストが filesystem / agent に触れないという実在の価値を失うので推さない。）

### dead code / 残骸

機械的な参照カウント（Go: エクスポート識別子 1693・非エクスポート 1112・メソッド 468・構造体フィールド 1635 / web: export 322・i18n キー 1094）を import エイリアス解決つきで実施し、候補を 1 件ずつ grep で裏取りした。結論として、このリポジトリの dead code は「散らばったゴミ」ではなく**方針転換 3 系列の切り株がそのまま残ったもの**に集中している。(1) 外部プロセス実行の廃止（2026-08-13）: platform.OutputRunner 継ぎ目一式 + internal/platform/process/command.go 117 行が本番から完全に到達不能で、テストの stub すら一度もインスタンス化されていない。secret の askpass 定数も残骸。(2) single-app → explicit-engine-owners（2026-08-14→15）: launchBackground が 4 OS 分 0 呼び出しで残り、その結果 Electron 側の `--hidden` 起動モード（desktop/main.js:70,361）が Go 側から到達不能になっている。cmd/sshc/launch_unsupported.go は対応する newPlatformParts が無くどの GOOS でもコンパイルできない。(3) Windows 移植の未完（2026-08-15）: windows.Toolchain は実装もテストも完成しているのに wiring されず、windowsregistry の exported 2 関数と 3 つの _other.go スタブ（計 67 行）は呼び出し元が一人も現れなかった。加えて internal/api の生成型 203 個に対し到達可能性解析を行い、実質スキーマ 33 型（+ boilerplate 43）が本番 Go から到達不能であることを独立に確認した。「テストは通るが本番から呼ばれない」類は secret（Has/Rename/AssignedCredential/HasKeyPassphrase/Vault.RemoveKeyPassphrase）、terminal（Prune/Ring.Cap/Ring.Len）、storage、handoff に分布する。誤検出回避のため build tag・gomobile バインド（Java 側から Mobile.start/stop/lastStartFailureKind は実際に呼ばれている）・JSON タグ経由（GroupMetadata.Hidden は web が消費している）は個別に確認して除外した。なお完全な死は少なく、真に 0 参照なのは 1693 中 67（うち 49 は生成物）、非エクスポート 1112 中 3 のみで、コード全体の衛生状態は悪くない。

**確度の高い指摘**

- **外部プログラム実行の継ぎ目一式が本番から完全に到達不能（platform.OutputRunner + process/command.go 全体）**（confirmed）
  - 場所: `internal/platform/command.go:11`, `internal/platform/command.go:16`, `internal/platform/command.go:19`, `internal/platform/command.go:27`, `internal/platform/command.go:40`, `internal/platform/command.go:50`
  - 影響: platform/command.go の 9-52 行（44 行）と process/command.go 全体（117 行）+ command_test.go（120 行）の計 280 行超が、本番バイナリのどの経路からも呼ばれない。しかも interface が生きているように見えるため、次に「外部プログラムを起こしたい」と考えた人はこの継ぎ目を再利用しようとし、廃止済みの方針に戻ってしまう。テストが緑なので CI もこれを検出しない。sshclient/exec.go:17,20-26,95-114 に後継（MaxCapturedOutput / Output / cappedBuffer）が既にあり、上限値 64<<10 まで同一の二重定義になっている。
  - 対策: internal/platform/process/command.go と command_test.go を削除し、internal/platform/command.go から MaxCapturedOutput・ErrTimedOut・ErrProgramPathNotAbsolute・Command・Output・OutputRunner（9-52 行）を削除する。Toolchain interface（54-66 行）だけを残す。あわせて内部 app/run_test.go:184、httpserver/diagnostics_test.go:25-33 の stubRunner、keys/service_test.go:29-40 の newQueryRunner/recordingRunner と newTestService/newServiceWithAgent の runner 引数を落とす（約 30 箇所の呼び出しが機械的に短くなる）。
- **Windows の Toolchain は実装もテストも完成しているのに配線されず、Windows でハードウェア鍵の項目が黙って出ない**（confirmed）
  - 場所: `internal/platform/windows/toolchain.go:35`, `internal/platform/windows/toolchain.go:43`, `cmd/sshc/wiring_windows.go:16`, `cmd/sshc/wiring_windows.go:22`, `cmd/sshc/wiring_darwin.go:14`, `cmd/sshc/wiring_linux.go:14`
  - 影響: これは dead code というより「配線漏れによる機能欠落」で、この観点で見つかったもののうち唯一ユーザーに見える実害がある。internal/keys/catalogue.go:74-80 が Toolchain == nil で早期 return するため、%WINDIR%\System32\OpenSSH\ssh-keygen.exe が実在する Windows でもハードウェア鍵（PIN/タッチ/libfido2）の項目が画面に出ない。README.md:60 は Toolchain が nil になるのを Android だけと書いており、Windows 節（101-153）にこの欠落の記述が一切ないので、文書からも気づけない。実装 69 行 + テスト 106 行が待機したまま。
  - 対策: cmd/sshc/wiring_windows.go で `windows.NewToolchain(<WINDIR>)` を platformParts.Toolchain に入れる。WINDIR の取得は同パッケージの shell_windows.go:45 systemLookup が既に持っている信頼の起点をそのまま使える。あわせて :16-17 の陳腐化したコメントを消す。もし配線しない判断なら、windows/toolchain.go とそのテストを削除し README.md:101-153 に「Windows ではハードウェア鍵の項目は出ない」と明記する。
- **launchBackground が 4 OS 分で 0 呼び出し。その結果 Electron 側の --hidden 起動モードが Go 側から到達不能になっている**（confirmed）
  - 場所: `cmd/sshc/launch_darwin.go:53`, `cmd/sshc/launch_linux.go:126`, `cmd/sshc/launch_windows.go:54`, `cmd/sshc/launch_unsupported.go:25`, `desktop/main.js:70`, `desktop/main.js:361`
  - 影響: Go 側の死んだ関数 4 本（計約 20 行）だけでなく、Electron 外殻の「窓を作らずメニューバー項目だけで起きる」モード全体が到達不能になっている。この機能は README.md:380 が今も現行仕様として説明しているため、コードと文書の両方が実在しない挙動を約束している状態。desktop 側の hidden 分岐を将来リファクタする人は、それが生きていると信じて設計判断をしてしまう。
  - 対策: 4 つの launchBackground 定義と launch_darwin.go:49-52 のコメントを削除する。そのうえで desktop/main.js の hidden モードを (a) 意図的に残すなら Go 側の呼び出し経路を復活させる、(b) 不要なら :70 と :361 の分岐も削除する、のどちらかを決める。README.md:380 は現行の connectflow.go の挙動（隠し起動ではなく通常の Launch、待ち時間は connect.go:99-108 の 40×100ms = 4 秒）に書き換える。
- **cmd/sshc/launch_unsupported.go はどの GOOS でもコンパイルが通らない（対応する newPlatformParts が存在しない）**（confirmed）
  - 場所: `cmd/sshc/launch_unsupported.go:1`, `cmd/sshc/wiring_darwin.go:12`, `cmd/sshc/wiring_linux.go:12`, `cmd/sshc/wiring_windows.go:22`, `cmd/sshc/engine.go:171`
  - 影響: 「移植先が増えたときに壊れないための保険」として書かれた 25 行が、実際にはその GOOS でビルドを試した瞬間に別の理由で落ちるので保険になっていない。しかも 4 ファイル 1 シリーズの中で launch_darwin.go だけが明示 build tag を持たず（ファイル名の暗黙制約に頼る）、他 3 つが明示タグを持つという体裁の不揃いも同居している。
  - 対策: launch_unsupported.go を削除し、launch_darwin.go / launch_linux.go / launch_windows.go の 3 本で他の *_darwin/*_linux/*_windows 系（wiring, ownership, vault_terminal）と同じ被覆にそろえる。移植先を本当に増やす日が来たら、wiring / ownership / signals / vault_terminal と一緒に追加する。
- **生成モデル internal/api の実質スキーマ 33 型が本番 Go から到達不能。httpserver 側に同形の手書き双子が存在する**（confirmed）
  - 場所: `internal/api/models.gen.go:398`, `internal/api/models.gen.go:553`, `internal/api/models.gen.go:569`, `internal/api/models.gen.go:621`, `internal/api/models.gen.go:875`, `internal/api/models.gen.go:995`
  - 影響: TypeScript 型は openapi.yaml から openapi-typescript で直接生成されるので、Go 側の生成モデルは httpserver が使うためだけに存在するはず。その 3 分の 1 弱が誰にも使われず、代わりに同じ形の手書き型が並んでいるため、これらの endpoint では openapi.yaml との乖離を Go のコンパイルで検出する手段が無い。make verify-generated は models.gen.go が openapi.yaml と一致することしか見ないので、契約から外れた endpoint には効かない。
  - 対策: 手書き双子（config_handlers.go:26-55 の 6 型）を対応する api.* に置き換えるか、逆に「config 系は application 型を直に通信形式にする」と決めて openapi.yaml 側からそれらのスキーマを外す。どちらか一方に寄せれば 33 型の大半が解消する。少なくとも今の 3 方針併存（生成型を使う / 手書き双子 / application 型を直流し）は endpoint ごとにコメントで理由を書くべき。
- **web/src/diagnostics/PasswordPanel.tsx のコンポーネント（288 行）は本番で一度もレンダリングされず、同居する 5 行のヘルパーだけがファイルを延命させている**（confirmed）
  - 場所: `web/src/diagnostics/PasswordPanel.tsx:43`, `web/src/diagnostics/PasswordPanel.tsx:39`, `web/src/diagnostics/PasswordPanel.tsx:16`, `web/src/diagnostics/PasswordPanel.test.tsx:4`, `web/src/connections/ConnectionBasicForm.tsx:18`, `web/src/connections/ConnectionBasicForm.tsx:799`
  - 影響: 1 ファイルで 330 行 + テスト 226 行 = 556 行が、5 行のヘルパーを import するためだけにバンドルへ残り続けている。テストが 12 ケース通り続けるので緑のままで、削除の圧力がかからない。加えて features 間の依存 connections → diagnostics がこのためだけに生まれており、その参照先のコンポーネント本体が死んでいるという歪んだ形になっている。
  - 対策: eligibilityText と eligibilityKeys（:20-38）を web/src/connections/ 配下（例 authenticationPolicy.ts の隣）か共有の場所へ移し、PasswordPanel.tsx と PasswordPanel.test.tsx を削除する。ConnectionBasicForm.tsx:18 の import 先を差し替えるだけで完了する。

## 9. 付録 C — サブシステム別の所見

### cmd/sshc（CLI/engine 入口 20+ ファイル）, mobile/（gomobile バインディング）, startup.sh, Makefile

**責務** — cmd/sshc は sshc の唯一の実行可能入口で、argv を副作用なしに `invocation` へ写し（invocation.go）、その Kind ごとに 6 種類の「持ち主」へ処理を振り分ける（main.go:45-93）。engine 本体（internal/app.Run）を起こすのは `sshc engine` / `sshc headless` の 2 経路だけで、それ以外（bare `sshc`, `<alias>`, `run`, `connect`, `list`, `open`, `status`, `vault`）は既に走っている engine に HTTP で問い合わせるか、ローカルの ~/.ssh/config を読むだけである。OS 差分は wiring_*（部品組み立て）、launch_*（デスクトップ外殻の起こし方）、signals_*/ownership_*/vault_terminal_*（OS API）の 4 系統の build-tag ファイル群に閉じている。mobile/ は同じ internal/app を Android の同一プロセス内で起こすための第 2 の配線経路で、cmd/sshc を一切通らない。

**所見** — この領域は「一貫性が壊れている」というより、**2026-08-14 → 08-15 → 08-16 の 3 回の方針転換が、コードには反映されて README とごく一部の関数には反映されていない**という形で層になっている。

転換点は plan の日付順にはっきり出る。(1) 2026-08-14 `single-app`：外殻が engine を子として持ち、「端末からはマスターパスワードを答えられるようにし、解錠はエンジンの中に残る」（plan 冒頭 Architecture）。(2) 2026-08-15 `explicit-engine-owners-and-vault-cli`：「コマンド解析を副作用から分離し、`sshc engine` と `sshc headless` だけが共通の engine runner に入る」——invocation.go / engine.go / ownership*.go / vault*.go はここで生まれ、端末での解錠は独立した `sshc vault unlock` に移された。(3) 2026-08-15 `windows-platform`、(4) 2026-08-16 `android-engine`。

現在の cmd/sshc の骨格（invocation → dispatch → 6 owner）は (2) の設計に忠実で、内部的には一貫している。破綻しているのは**境界の掃除**である:

- (1) 時代の遺物がコードに残っている: `launchBackground`（4 実装 0 呼び出し、macOS の `--hidden` 起動を抱えたまま）、`sshFinder`（0 参照）、`connect.go` の `waitForHandoff` が想定していた 20 秒→実装 4 秒。
- (1) 時代の記述が README に残っている: README:380-381 は「端末でマスターパスワードを尋ねる」「`/cli/unlock` へ渡す」「Linux には起こし方がまだ無い」と書き、3 つとも現行実装と食い違う。README を正本として読むと (2) 以降の設計が見えない。
- (3) が途中で止まっている: `internal/platform/windows.NewToolchain` は実装もテストも完成しているのに wiring_windows.go が接続していない。wiring_windows.go:16-17 のコメントが「本物の Windows toolchain はその task が入れる」と、未着手の前提のまま残っている。これは 4 OS のうち Windows だけ機能が黙って欠ける形になっており、findings 中で最も実害が大きい。
- (4) が別経路として増えた: mobile/dependencies.go は engine.go:171-193 の配線を複製したもので、struct literal 経由なので app.Dependencies にフィールドが増えても壊れない。既に Biometric と ShutdownTimeout の 2 つが黙って落ちている。コメントが今も「Kotlin」と書いているのは、plan 自身が記録した Java への変更（android-engine plan:32）が mobile/ 側へ戻ってきていない証拠。

OS 分岐そのものは**概ね妥当だが、粒度が揃っていない**。本当に OS で違うのは ①デスクトップ外殻の起こし方（LaunchServices / desktop.json / HKCU）②所有権チャンネルの見張り方（poll / PeekNamedPipe）③no-echo 入力（termios+poll / ConsoleMode+ReadConsoleInputW）④Toolchain と Biometric の有無、の 4 つだけである。それに対して signals_*（本体が完全同一）、wiring_*（3 ファイル 59 行で 2 フィールドの差）、`Stop()`（2 実装が逐語同一）、launchBackground（4 実装が実質 2 種）は分岐の必要がないところまで分岐している。逆に launch 系だけが unsupported フォールバックを持ち（しかも wiring に対応物が無いので到達不能）、signals / ownership / vault_terminal / wiring は unix|windows で尽くしているという不揃いもある。

Makefile は「Makefile 自身は薄く、実体は internal/buildcontract/nativebuild へ」という方針が徹底されており、この領域では最も設計が揃っている。ただし Windows の release job が「recipe が POSIX 前提」を理由に make を迂回した結果（release.yml:151-155）、bundle 一覧が Makefile:92 と release.yml:177 に二重化し、それを消す代わりに契約テストで固定するという選択がされている。同時に build-cli / desktop-version / release-binaries / desktop-bundle-windows の 4 ターゲットが「契約テストが存在を要求するから残っている」状態になっており、テストが dead target を延命させる構図ができている。

startup.sh は 481 コミット前から取り残されており、参照も無く、内容（bare `sshc` を常駐として exec）は現在の起動モデルと正面から矛盾する。

**確度の高い指摘**

- [dead-code] **launchBackground が 4 OS 分実装されているが呼び出し側が 1 つも無い**
  - 場所: `cmd/sshc/launch_linux.go:126, cmd/sshc/launch_darwin.go:53, cmd/sshc/launch_windows.go:54, cmd/sshc/launch_unsupported.go:25`
  - 根拠: `rg -n '\blaunchBackground\b' --type go` の全 7 件が 4 つの定義行と、それぞれの直上のドキュメントコメント 3 行のみ。呼び出しもテストも 0 件。現在の接続経路は connectflow.go:119 と :155 で `launcher.Launch(ctx)` を呼んでおり、macOS ではそれが launch_darwin.go:46 の `runOpenBundle(ctx, "-b", bundleID)`（-g も --hidden も無し）になる。すなわち launch_darwin.go:54 の `-g -b <id> --args --hidden` は、リポジトリ内で唯一この死んだ関数からしか参照されていない
- [dead-code] **Windows の Toolchain 実装は完成しているのに wiring_windows.go が nil を返し続けている**
  - 場所: `cmd/sshc/wiring_windows.go:11-24, internal/platform/windows/toolchain.go:35`
  - 根拠: internal/platform/windows/toolchain.go:35 に `func NewToolchain(windowsDirectory string) Toolchain` があり、KeyGen() は toolchain.go:43-53 で実装済み、toolchain_test.go に 5 ケースのテストがある。シグネチャ `KeyGen() (string, error)` は internal/platform/command.go:64-66 の `platform.Toolchain` interface をそのまま満たす。にもかかわらず `rg -n 'NewToolchain'` の結果、windows.NewToolchain の参照は自ファイルと自テストのみで、本番コードからの呼び出しは 0 件。wiring_windows.go:16-17 のコメントは「本物の Windows toolchain はその task が入れる。それまでのあいだ、偽物を置いて動いているふりをしない」と、まだ存在しない前提で書かれたまま残っている。結果として Windows では鍵一覧のハードウェア鍵項目が黙って出ない
- [stale-doc] **README:327 が「Go 側でプログラムを起こす場所は 2 つ」と書いているが実際は 5 つ**
  - 場所: `README.md:327, internal/acceptance/programs_test.go:32-53`
  - 根拠: README.md:327 は launch_darwin.go と internal/terminal/pty_unix.go の 2 件だけを名指ししている。実際の allowedToStartPrograms（programs_test.go:32-53）は cmd/sshc/launch_darwin.go, cmd/sshc/launch_linux.go, cmd/sshc/launch_windows.go, internal/terminal/pty_unix.go, internal/buildcontract/nativebuild.go の 5 件。launch_linux.go:76 と launch_windows.go:49 の `exec.CommandContext(ctx, path).Start()` は README の記述の外にある
- [stale-doc] **README:380-381 が 2026-08-14 版の接続フローを説明したまま。実装は 08-15 版に置き換わっている**
  - 場所: `README.md:380, README.md:381, cmd/sshc/connectflow.go:146-160, cmd/sshc/connect.go:99-108, internal/httpserver/vault_cli.go:17-22`
  - 根拠: README:380 は「macOS ではアプリを隠しで起こします（open -g -b <bundleID> --args --hidden）」「~/.ssh/sshc/cli が現れるまで最大 20 秒待ちます」「Linux にはこの起こし方がまだありません」と書くが、(a) --hidden 経路は死んだ launchBackground の中にしかない、(b) waitForHandoff（connect.go:99-108）は 40 回 × 100ms = 4 秒、(c) launch_linux.go は 2026-08-15 の plan で追加済み。README:381 は「vault が施錠されていれば、その場でマスターパスワードを端末で尋ねます…本体の /cli/unlock へ渡して解錠を試みます」と書くが、connectflow.go:146-160 は尋ねずに「run sshc vault unlock」と案内して待つだけで、/cli/unlock は internal/httpserver に存在しない（vault_cli.go:17-22 は /cli/vault/* のみ。/cli/unlock は vault_cli_test.go:347 に「もう無い」ことを確かめる test としてだけ残る）

### internal/remotesync, internal/objectstore, internal/selfupdate, internal/buildcontract, internal/diagnostics（および対応する web/src/diagnostics）

**責務** — この5領域は「アプリ本体が自分以外の世界と接する面」をそれぞれ担う。remotesync + objectstore は ~/.ssh のワークスペース全体を1オブジェクトの暗号化 tar.gz として S3 互換ストアへ条件付き PUT で往復させ、pull を storage.Request 1件へ畳んでトランザクション層の安全性（journal・世代バックアップ・再解析）を丸ごと継承する。selfupdate は GitHub の最新リリースを1回 GET して版を比べるだけで、取得も置換もしない。diagnostics は設定グラフの検査・直接 TCP ダイヤル・プロセス内 SSH 認証テストという3つの「人が明示的に押す検査」を提供する。buildcontract だけは配布物ではなくビルド系の住人で、Makefile / GitHub Actions から `go run` される移植可能なビルド CLI と、Makefile・workflow YAML・シェルスクリプトの中身を固定するメタテスト群を抱えている。

**所見** — この5領域は「設計が破綻している」というより、**方針転換のたびに古い側を消し切らずに残したことが一定量ある**、という状態である。転換の痕跡は3系統に分けられる。

**(1) 目的を果たして縮んだ領域は、むしろ健全に縮んでいる。** objectstore は 2026-08-05 の実装プランでは手書き SigV4（`sigv4.go` + AWS 公開テストベクタ）を持つ設計だったが、いまは aws-sdk-go-v2 に置き換わり、`sigv4.go` は残骸を1バイトも残さず消えている。`rg 'aws-sdk-go-v2' --glob '*.go'` は internal/objectstore の外に1件もヒットせず、SDK 直呼びとの二重化は無い。残っている322行は「https 強制」「本文上限」「非2xx 本文の破棄」「UNSIGNED-PAYLOAD の巻き戻し」という SDK が面倒を見ない4点に絞られており、包む理由がコードとして立っている。selfupdate も同様で、README「Android に自己更新は無い」は正しく、cmd/sshc/engine.go:177 → app.Dependencies.Updates → httpserver/server.go:365 → /api/v1/update → web/src/shell/UpdateBadge.tsx まで3 OS で完全に配線され、mobile/dependencies.go:50 が `Updates: nil` を明示コメント付きで置いている。**骨だけの未配線パッケージではない。**

**(2) 逆向きに転換した箇所で、古い側の説明とテストが生き残っている。** 最も濃いのは remotesync の `Entry.Secret` で、2026-08-05 プランの「secret 印 → SkipBackup で鍵のバックアップを作らない」がバックアップ封印化によって撤回されたのに、フィールド・代入・「SkipBackup を付けさせる」という2つのコメント・その根拠で存在するテストが残り、同じパッケージの plan_test.go が真逆を表明している。診断側の `DefaultConnectTimeout` も外部 ssh 廃止(2026-08-13)の取り残しで、httpserver/diagnostics.go:198 の TerminalCommand コメントも同じ性質。これらは動作を壊さないが、**次に読む人へ誤った制約を教える**という意味で害がある。

**(3) UI と CI 側でだけ、旧方針の構造がそのまま残っている。** web/src/diagnostics/PasswordPanel.tsx は、パスワード UI が connections/secrets へ移ったあとヘルパー1本のためだけに556行が生き残った典型で、ディレクトリ名（diagnostics）と中身（保管庫）も一致していない。ビルド側では、単一 macOS ランナーで全部クロスビルドする旧戦略の `matrix` 経路（RELEASE_TARGETS 一式）が、OS ごとの job に切り替わった今も Makefile に残り、CI からは呼ばれない。README.md:368 が「1台の macOS ランナーから全部作れます／Windows と Android は成果物に載せません」と書いているのは、まさにその旧戦略の文章がそのまま残っているためで、同じ README の :176-186 と正面から矛盾している。

**一貫性という観点で最も気になるのは抽象境界の置き場所である。** 同じ「サーバーが画面へ理由を渡す」問題に対し、remotesync は符牒だけを返して UI で訳す（auto.go:230 に理由まで書いてある）のに、diagnostics は英語の完成文と機械符牒を素のまま画面へ出している。同じ「オブジェクトストアを差し替える」問題に対し、httpserver は設定チェックの HEAD 1本にだけ関数継ぎ目を持ち、実際にバイトを運ぶ Push/Pull/Rekey には継ぎ目が無いので、テストは毎回 TLS サーバーと SDK の署名経路を通す。片方の判断は明文化されているのに、もう片方に適用されていない——これは「方針が変わった」というより**方針が広がり切っていない**部類で、直すコストは低い。

なお buildcontract は「テストが実装の2倍」だが、その内訳は自パッケージのテスト780行とリポジトリ契約メタテスト1428行であり、後者は buildcontract のシンボルを1つも呼ばない。本番コードパス（配布される sshc バイナリ）には一切載らないが、Makefile と両 workflow から `go run` される正真正銘のビルド経路であり、internal/ に置くこと自体は妥当。問題は同種のメタテストの住処が internal/acceptance にも既にあり、二重の置き場所になっていることの方である。

**確度の高い指摘**

- [stale-doc] **README「更新の境界」の記述が同じ README の「リリース」節および release.yml と正面から矛盾する**
  - 場所: `README.md:368, README.md:176-186, .github/workflows/release.yml:134-282`
  - 根拠: README.md:368 は「リリースは 4 つのバイナリを出します（darwin-arm64/amd64, linux-amd64/arm64）」「1 台の macOS ランナーから全部作れます」「Windows と Android はコードとしては対応していますが、リリース成果物にはまだ載せません」「Android は make android-bind と android/gradlew assembleDebug で手元から作れます」と書く。一方 README.md:176-186 の表は Windows(`sshc-windows-{amd64,arm64}.exe` と NSIS setup 2本) と Android(署名済み .apk) を配ると書き、release.yml には macos/linux/windows/android の4 job があり、windows job は release.yml:171 で `sshc-windows-*` を、android job は release.yml:274 で署名済み APK を生成し、publish job(release.yml:283) が `needs: [macos, linux, windows, android]` で全部を公開する。internal/acceptance/documentation_test.go は README の別の記述だけを固定しており、この段落は守られていない
- [dead-code] **web/src/diagnostics/PasswordPanel.tsx は本体から一度も描画されない（556行が実質死んでいる）**
  - 場所: `web/src/diagnostics/PasswordPanel.tsx:43, web/src/diagnostics/PasswordPanel.test.tsx:41, web/src/connections/ConnectionBasicForm.tsx:18`
  - 根拠: `grep -rn "PasswordPanel" web/` の結果は PasswordPanel.tsx 自身と PasswordPanel.test.tsx(226行, 12ケース)のみ。App.tsx は DiagnosticsPanel と SecretsPanel を lazy import するが PasswordPanel は import しない。生きているのは同ファイルの `eligibilityText`(:39) だけで、これを web/src/connections/ConnectionBasicForm.tsx:18 が import して :799,:802 で使っている。パスワード UI は web/src/connections/{ConnectionBasicForm,CreateConnectionModal} と web/src/secrets/ へ移ったが、旧パネルはヘルパー1本の置き場所として残り、テストだけが走り続けている
- [dead-code] **Manifest の Secret フラグは書かれるだけで誰も読まない。しかも3箇所の説明が今の実装と逆のことを言っている**
  - 場所: `internal/remotesync/snapshot.go:82-89, internal/remotesync/service.go:419-421, internal/remotesync/snapshot_test.go:72-73, internal/remotesync/plan_test.go:20,105-109`
  - 根拠: `rg -n '\bSecret\b' internal/remotesync internal/storage` の結果、Secret を代入するのは service.go:421 のみ、読むのは snapshot_test.go:83 と plan_test.go の fixture ヘルパーのみで、plan.go は一度も参照しない。snapshot.go:84 のコメントは「**いまこれを読む側は居ない**」と自ら述べている。にもかかわらず service.go:419-420 は今も「この印が、pull にそれを SkipBackup 付きで適用させる」と書き、snapshot_test.go:73 は「pull は secret のエントリを SkipBackup 付きで適用する。この印を失えば…鍵素材のコピーが残る」を根拠に TestAPrivateKeyIsMarkedSecret を維持している。ところが同じパッケージの plan_test.go:105-109 は逆に「どの change も SkipBackup を要求してはならない」を表明している。docs/superpowers/plans/2026-08-05-ssh-ui-remote-sync-implementation-plan.md:45,282 が旧方針（secret→SkipBackup）の出所

### internal/sshclient, internal/terminal, internal/knownhosts, internal/sshintegration, internal/acceptance（プロセス内 SSH クライアントと端末セッション層）

**責務** — この領域は「外部の ssh(1) を一切起こさずに SSH を話す」という 2026-08-13 の方針転換の受け皿である。internal/sshclient が golang.org/x/crypto/ssh で接続・認証・ホスト鍵検証・ポート転送・リモート実行を担い、internal/terminal が SSH かローカルシェルかを問わない端末セッションのレジストリ（生存管理・スクロールバック・停止の二段階）を持ち、internal/knownhosts が known_hosts の無損失パースとトランザクション書き込みを担う。internal/sshintegration と internal/acceptance は実装 0 行のテスト専用パッケージで、前者は本物の sshd に対する結線検査、後者は本物のサーバーをプロセス内で立ててルート横断の hardening を固定する。

**所見** — **方針変更の痕跡は「消し残り」ではなく「出口を失った計算」として現れている。** この領域は 2026-08-13 の 2 本の計画（in-process-ssh-client → retiring-the-external-ssh）で外部 ssh 経路を丸ごと置き換えており、外部プロセス起動そのものは徹底的に潰されている。`rg 'exec.Command'` の非テストヒットは 7 件で、うち SSH に関わるものは 0 件。`ssh` / `ssh-add` / `ssh-keyscan` / `ssh-agent` を argv に載せる箇所も無い。しかも internal/acceptance/programs_test.go:60 が全 .go を歩いて起動箇所を allowlist と突き合わせ、README.md:327 がその allowlist の各エントリの理由を文章で持つ。**この方針転換自体は完遂されている。**

破綻はその周辺に 3 つの型で残っている。

第一に、**「作ったが繋がなかった」もの。** NewTarget の Notice はもっとも大きい例で、unhonoured 表・parseForwards の束ね notice・noticesFor という 3 つの生成器が動いているのに、production の受け取り手 3 箇所すべてが `_` で捨てる。README.md:328/330 は届く前提で書かれている。同型のものとして terminal.Spec.Cleanup（設定者はテストのみ、理由の FreezeSSHConfig は削除済み）、platform.OutputRunner 一式（production 参照ゼロ）、Dialer.Dial と Auth.ReadFile（テスト専用の注入点が公開 API に露出）がある。

第二に、**「後から足した経路が、先にあった経路の学習を継がなかった」もの。** 最も重いのが Probe の HostKeyAlgorithms 欠落で、hostkey.go:76-86 が「実際そうなっていた」とまで書いて記録した障害が、probeChain という写しの側に温存されている。Run と Stream、chain と probeChain も同じ形で、**分岐が要るのは末端だけなのに手前の 40〜50 行ごと写している。** closers の逆順クローズが closeAll を持ちながら 4 箇所に手書きされているのも同じ癖である。

第三に、**説明が実装より遅れている。** README.md:329 と :336 は同じ節の中で正反対のことを言い、:343 は diagnostics/authentication.go:78 が「もう無い」と明言した仕組みを説明する。internal/terminal/terminal.go:3 の package doc は internal/platform が ssh のコマンドラインを組んでいた頃のまま。internal/acceptance/documentation_test.go は README を検査しているが見ているのは語句 10 個程度で、矛盾も陳腐化も捕まえない。**説明を検査する仕組みがある**という点は評価できるが、いま守っている範囲は「消した入口の名前が残っていないか」だけである。

**抽象の量そのものは概ね妥当である。** terminal は内部依存ゼロの leaf で、Spec.Open という 1 つの継ぎ目だけで SSH を知らずに SSH を受け取るという設計を実際に成立させている。関数注入（Scanner.Collect、HostKeys.Read/Add、Resolver）はいずれも「読む場所を一つにする」「import の輪を作らない」という理由が明記され、その理由が今も成立している。過剰なのは interface ではなく、**使われない公開シンボル（sshclient だけで 8 個）と、テスト専用の注入点** の方である。

逆に**足りていない**のは 2 つ。(1) known_hosts のホスト欄書式が 3 箇所で独立に組まれ、そのうち 1 箇所が「同じでなければならない」とコメントで祈っている。(2) glob マッチャが 21 行完全一致で 2 パッケージにある。どちらも「重複していることは分かっているが寄せていない」状態で、コメントが寄せない理由を説明しているが、その理由（大小文字の扱いが違う）は引数 1 つで吸収できる。

**forceCloser は doc と実装が食い違ったまま停止設計の根拠になっている。** terminal.go:144 が「プロセス内 SSH のホップ」を実装者に数え、shutdown_test.go:82 が KindSSH の stub に ForceClose を持たせているため、締切での強制停止が SSH に効かないことがテストからは見えない。これは陳腐化した doc というより、**テストが production 型の代わりに doc の主張を検証している**構図で、この領域で最も直しにくい類の破綻である。

**テストのみのパッケージ 2 つという構成自体は妥当。** internal/acceptance は app.Build で本物のサーバーを隔離 HOME に立てて全ルートを横断するので、どの 1 パッケージにも属さない。internal/sshintegration は「単体テストの相手が実装のもう半分なので、両方が同じ勘違いをしていれば緑になる」（client_test.go:5-8）という明確な理由を持ち、環境変数未設定で全 skip するため `go test ./...` の密閉性も壊さない。`internal/` 直下に置くことで外部から import されないことも保証される。**問題は配置ではなく命名**で、`integration/`（プロセス実体の検査、make test が回す）と `internal/sshintegration`（実 sshd、make integration が回す）の名前が近すぎ、Makefile:318-321 が 4 行かけて別物だと説明する羽目になっている。`internal/sshdconformance` のような、相手を名指しする名前が本来ふさわしい。

**確度の高い指摘**

- [dead-code] **NewTarget が返す Notice を production の呼び出し元 3 箇所すべてが捨てている**
  - 場所: `internal/sshclient/target.go:51, internal/sshclient/target.go:275, internal/app/ssh.go:66, internal/app/ssh.go:249, internal/app/ssh.go:268`
  - 根拠: `rg 'NewTarget\(' --type go -g '!internal/sshclient/*'` のヒットは 3 件で、すべて `target, _, err := sshclient.NewTarget(...)` と notice を `_` で捨てている。`rg 'sshclient\.Notice'` の外部参照は 0 件。`grep -i notice README.md` も 0 件で、API 応答にも UI にも出口が無い。それでも unhonoured 表（target.go:51-59、7 キーワード分の説明文）、noticesFor（target.go:275-289）、parseForwards のループバック束ね notice（target.go:255-268）が毎回計算されている。README.md:328 は「落とした機能は、黙って無視せず理由を添えて注意を出します」、README.md:330 は「それ以外が書かれていたら束ねて注意を出します」と書いているが、その注意が届く先は存在しない。
- [stale-doc] **README の同じ節の中で、ポート転送が「動く」と「まだありません」が併記されている**
  - 場所: `README.md:329, README.md:336`
  - 根拠: README.md:329「**ポート転送（`LocalForward` / `DynamicForward`）と agent 転送（`ForwardAgent`）は動きます。開いているものはコンソールの一覧に出ます。**」に対し、7 行下の README.md:336 は「転送（`LocalForward` / `RemoteForward` / `DynamicForward`）、`ForwardAgent`、`ForwardX11`、`ControlMaster`、`SendEnv` は**まだありません**」と書く。実装は internal/sshclient/forward.go:154 forwards.open と :271 forwardAgent にあり、329 が正しい。336 は 2026-08-13 の port-forwarding 実装より前の記述が消し忘れられたもの。internal/acceptance/documentation_test.go は語句を数個しか見ていないのでこの矛盾を捕まえない。
- [stale-doc] **README の認証テストの説明が、コード側が「もう無い」と明言している旧実装を説明している**
  - 場所: `README.md:343, internal/diagnostics/authentication.go:78`
  - 根拠: README.md:343「認証テストはタイムアウトとキャンセルを持ち、出力を上限つきで取得し、forwarding と `LocalCommand` をコマンドライン優先設定で無効化します」。一方 internal/diagnostics/authentication.go:78-80 の doc は「**無効化すべき機能の一覧はもう無い。** かつては転送も LocalCommand も SessionType も、外部の ssh に「するな」と言う必要があった。このクライアントにその機能が無いので、言う相手がいない」と書く。実装（authentication.go:83-116）にコマンドライン優先設定を組み立てる箇所は無く、`rg 'HardeningOptions'` は 0 件。README が旧 ssh -v 経路の説明のまま残っている。
- [other] **Probe だけが HostKeyAlgorithms を渡さないので、認証テストが実接続の通るホストを host_key_changed と報告しうる**
  - 場所: `internal/sshclient/probe.go:106, internal/sshclient/client.go:152, internal/sshclient/hostkey.go:76`
  - 根拠: connectOne は client.go:152 で `HostKeyAlgorithms: d.HostKeys.Algorithms(target)` を設定するが、probeChain が組む ClientConfig（probe.go:106-112）には HostKeyAlgorithms フィールドが無い。hostkey.go:76-86 のコメントは「三種類の鍵を持つ普通の Ubuntu が相手だと RSA が選ばれ、known_hosts にあるのが ed25519 の 1 行だけなら、正しいホストの正しい鍵が「一致しない鍵」として現れる——**実際そうなっていた**」と、この欠落が起こす障害を名指しで記録している。同じ欠落が Probe 側に残っている。Probe は internal/app/ssh.go:111 経由で Diagnostics 画面の認証テストに使われる。
- [dead-code] **外部プロセス実行の継ぎ目一式（platform.OutputRunner / Output / process.NewOutputRunner）が production から到達不能**
  - 場所: `internal/platform/command.go:39, internal/platform/command.go:50, internal/platform/process/command.go:26`
  - 根拠: `rg 'NewOutputRunner\(\)'` のヒットは定義 1 件と internal/platform/process/command_test.go の 4 件のみ。`rg 'OutputRunner' --type go -g '!internal/platform/*'` は internal/keys/service_test.go の 2 件（テストヘルパの引数）だけ。README.md:327 自身が「**`RunOutput` を経由して外部プログラムを起こす場所は、もうありません。**」と書いており、実際そのとおりなので、interface・実装・Output 型・ErrProgramPathNotAbsolute まで丸ごと呼ばれない。48d2bdb「askpass のあとに余った継ぎ目を消す」で消し残った分。
- [other] **ForceClose がプロセス内 SSH セッションに効かない（doc は効くと書いている）**
  - 場所: `internal/terminal/terminal.go:144, internal/httpserver/terminal.go:231, internal/sshclient/session.go:112`
  - 根拠: terminal.go:144 の forceCloser doc は実装者を「Unix の PTY のプロセスグループと、**プロセス内 SSH のホップ**」と名指しする。しかし `rg 'func .*ForceClose'` の production ヒットは pty_unix.go:85 / pty_windows.go:337 / httpserver/terminal.go:231 の 3 つだけで、sshclient.Session に ForceClose は無い。sessionLifetime.ForceClose は内側の Process が実装していなければ s.cancel() だけを行うが、Dialer.Open が作った ctx を握手完了後に見張る goroutine は存在しない（client.go:158 で SetDeadline を解除したあと、session.run は remote.Wait() で塞がる）。締切での強制停止は SSH セッションの輸送を切らない。internal/terminal/shutdown_test.go:82 の openStubborn は `Kind: terminal.KindSSH` の stub に ForceClose を持たせているので、この差はテストに現れない。

### internal/platform/ (4 OS 抽象の集約層: 直下フラット群 + linux/ macos/ windows/ nativepath/ process/ windowsacl/ windowspipe/ windowsregistry/)

**責務** — OS ごとに違う「外の世界」への接触点を一箇所に集め、上位層（app / keys / storage / handoff / httpserver / secret）が GOOS を知らずに済むようにする層。具体的には (1) ログインシェルの選定と起動引数、(2) ssh-keygen の在り処（Toolchain）、(3) ssh-agent への到達手段（KeyAgent interface）、(4) パス文法とプライベート状態の権限（nativepath / windowsacl）、(5) Windows 固有 OS API（named pipe / レジストリ / DACL）を担う。ただし現状の実体は「OS 抽象」より「Windows 移植のための置き場」に近く、Go LOC の 73.7%（3805/5162）が Windows スコープ、linux/ は 35 行、macos/ は Go 137 行 + ObjC 125 行しかない。

**所見** — 【確認済み】この領域の設計は 3 回、方針が入れ替わった痕跡を残している。

第 1 期（08-04〜08-09）は「OS ごとのアダプタ集」だった。実装計画 docs/superpowers/plans/2026-08-09-linux-support-implementation-plan.md には internal/platform/{terminal.go, application_darwin.go, application_linux.go}, macos/{command,keyagent,terminal,browser,loginitem,toolchain}.go, linux/{terminal,browser,loginitem,toolchain}.go, process/{command,keyagent,toolchain}.go が並んでおり、linux/ と macos/ は対称だった。この期の中核が「外部プログラムを起こす継ぎ目」= platform.OutputRunner である。

第 2 期（08-13〜08-14）で外部プロセスを全廃した。同じ 08-13 の日付で embedded-terminal 計画が internal/platform/interactive.go を新設し（同計画 :41「二つ目の実装は作らない」）、retiring-the-external-ssh 計画が同ファイルと Toolchain.KeyScan を削除している（同計画 :115,118）。single-app 計画（:1018-1019）が loginitem を両 OS から消し、残ったのが今の linux/toolchain.go 16 行・macos/toolchain.go 16 行である。**この期の最大の負債は、後継（internal/sshclient/exec.go の Output / MaxCapturedOutput / cappedBuffer）を作ったのに前任（platform/command.go + process/command.go）を消さなかったことで、継ぎ目の型・上限定数・上限付きバッファがそっくり二重化したまま残っている。**

第 3 期（08-15）の Windows 移植が現在の非対称を作った。windowsacl / windowspipe / windowsregistry / windows の 4 パッケージが 1 本の計画で一度に生まれ、当層 Go LOC の 73.7%（3805/5162）を占めるに至った。**この非対称のうち、windowsacl と windowspipe は必然である**——DACL・所有権・reparse point と named pipe は Unix に対応物が無く、それぞれ 6 パッケージ / 1 パッケージから実際に使われている。**しかし windows と windowsregistry の 2 つは「後付けで膨らんだ」側に属する。** windows パッケージは半分（Toolchain 69 行 + テスト 106 行）が未配線のまま、windowsregistry は exported 3 関数のうち 2 つが誰にも呼ばれず、同じ方針が NSIS 側（desktop/build/installer.nsh:25）にも書かれている。加えて 3 つの _other.go スタブ（計 67 行）は「非 Windows の呼び出し元が build tag を持たずに済むように」書かれたのに、その呼び出し元は一人も現れなかった。

【確認済み】継ぎ目の置き場に一貫した規約が無い。Toolchain は interface が platform 直下・OS 別実装が platform 配下、KeyAgent は interface が platform 直下・OS 別実装が internal/keys 配下、Guardian は interface が internal/secret・実装が platform/macos 配下（依存が逆流）。3 つの継ぎ目に 3 つの配置がある。

【確認済み】Android は build tag を一切持たず（`//go:build.*android` は 0 件）、分岐は依存注入 1 箇所（mobile/dependencies.go:43-50）と runtime.GOOS 1 箇所（internal/platform/shell_unix.go:45 → :27）に限られる。README が言う nil を受け止める側の判定は 10 箇所（Toolchain 1: internal/keys/catalogue.go:74、KeyAgent 3: internal/keys/service.go:670,732,776、Guardian 6: internal/app/run.go:268 と internal/secret/biometric.go:70,101,142,196 と internal/secret/service.go:1092）で、internal/platform 内にはゼロ。散らばりは深刻ではないが、**「Android では nil」という不変条件を守っているのは mobile/dependencies.go 1 ファイルだけで、build tag では守られていない**（GOOS=android は linux タグを満たすので cmd/sshc/wiring_linux.go が Linux の Toolchain を渡す）。

【推測】windows/toolchain.go が未配線のまま残った直接の原因は、計画 2026-08-15 の Task 4 が「production wiring は Task 6 でまとめて」と先送りし（:246）、Task 6 の記述が ConPTY と signals に集中していたため（:378 の一文に埋もれた）だと思われる。ただしこれはコミット履歴で検証していない。

【推測】alias.go が internal/platform に置かれた元の理由は「外部プログラムの argv に載る文字列の安全性」であり、外部実行が消えた今、この 71 行は本来 internal/config か internal/application に属する入力検証である。ただし移動可否は当層の外の呼び出し 44 箇所を見る必要があり、この調査では判断していない。

**確度の高い指摘**

- [dead-code] **外部プログラム実行の継ぎ目一式（OutputRunner / Command / Output / process パッケージの半分）が完全に死蔵**
  - 場所: `internal/platform/command.go:9-52, internal/platform/process/command.go:1-117`
  - 根拠: `rg '\bOutputRunner\b'` の非テスト参照 6 件はすべて定義（command.go:49-51, process/command.go:26,28）と cmd/sshc/launch_darwin.go:17 のコメント。RunOutput を呼ぶ production コードはゼロ。それどころか internal/acceptance/programs_test.go:16-30 が「`RunOutput` の継ぎ目を通ってプログラムを起こす場所はひとつも残っていない」と明言し、同 test の allowedToStartPrograms 5 件はいずれも os/exec を直接使う。process/command.go は 117 行 + テスト 120 行が丸ごと本番から到達不能
- [dead-code] **internal/platform/windows.Toolchain は完成しているが一度も配線されず、Windows でも Toolchain が nil のまま**
  - 場所: `internal/platform/windows/toolchain.go:29-57, cmd/sshc/wiring_windows.go:22-24`
  - 根拠: `rg '\bNewToolchain\b'` の production 呼び出しは cmd/sshc/wiring_darwin.go:14 と wiring_linux.go:14 の 2 件のみ。windows.NewToolchain の参照は自身の定義と toolchain_test.go の 5 件だけ。wiring_windows.go は `platformParts{KeyAgent: keys.NewAgent(os.LookupEnv)}` を返すので Toolchain はゼロ値 nil であり、cmd/sshc/engine.go:187 → internal/app/run.go:147 → internal/keys/catalogue.go:74 で早期 return し、Windows でもハードウェア鍵の項目は出ない。wiring_windows.go:16 のコメントは「本物の Windows toolchain はその task が入れる」と書いたまま残っている。計画側の意図も docs/superpowers/plans/2026-08-15-windows-platform-and-nsis-implementation-plan.md:246（「全 production wiring は Task 6 で一度に接続する」）と :378（「wiring_windows.go は trusted Windows toolchain、named-pipe key agent、ConPTY starter を production dependencies に渡す」）で明示されており、KeyAgent だけ接続され Toolchain が取り残された。windows パッケージの Toolchain 側は 69 行 + テスト 106 行
- [dead-code] **3 つの !windows スタブファイルが、存在しない呼び出し元のために置かれている**
  - 場所: `internal/platform/windowsacl/acl_other.go:7,10,14, internal/platform/windowspipe/conn_other.go:14,20, internal/platform/windowsregistry/launcher_other.go:11,17,27,29,31`
  - 根拠: import 元を全走査した結果、windowsacl は internal/{enginelock,handoff,keys,storage}/*_windows.go と *_windows_test.go のみ、windowspipe は internal/keys/agent_windows.go のみ、windowsregistry は cmd/sshc/launch_windows.go のみが import する。すなわち非 Windows ビルドでは 3 パッケージとも依存グラフに入らず、これら 67 行は到達不能。各ファイルの doc コメントは「これを読む側が build tag を持たずに済むため」と理由を述べているが、実際の読み手は全員 build tag を持っている。計画 docs/superpowers/plans/2026-08-15-windows-platform-and-nsis-implementation-plan.md:98 は Task 2 で cross-platform な `internal/handoff/handoff.go` を修正する想定だったが、実装は internal/handoff/files_unix.go / files_windows.go という OS 分割に落ち着き、スタブが取り残された
- [dead-code] **windowsregistry の exported 3 関数のうち 2 つ（Register / Remove）が production・テストとも未使用**
  - 場所: `internal/platform/windowsregistry/launcher_windows.go:96-98,117-119`
  - 根拠: `rg 'RegisterDesktopExecutable|RemoveDesktopExecutable'` のヒットは定義とコメントのみ。テスト（launcher_windows_test.go:75,82,98,120,124,131,144,148,160）はすべて小文字の registerDesktopExecutable / removeDesktopExecutable を呼んでおり、exported ラッパは誰も通らない。実際にレジストリを書き消しするのは NSIS 側（desktop/build/installer.nsh:25 の SSHC_LAUNCHER_KEY）で、同じ方針が Go と NSIS に二重実装されている
- [duplication] **platform.Output / MaxCapturedOutput / boundedBuffer が internal/sshclient に丸ごと再実装されている**
  - 場所: `internal/platform/command.go:11,40-47 と internal/platform/process/command.go:84-117 vs internal/sshclient/exec.go:17,20-26,95-114`
  - 根拠: sshclient.MaxCapturedOutput は `64 << 10` で platform.MaxCapturedOutput と同値同定義。sshclient.Output は Stdout/Stderr/ExitCode/Truncated/Elapsed を持ち platform.Output から Stopped を除いただけ。sshclient.cappedBuffer は process.boundedBuffer と同じ「上限まで書いて以降は捨て、truncated を立てる」実装。外部プロセス実行を in-process SSH に置き換えた際（docs/superpowers/plans/2026-08-13-retiring-the-external-ssh-implementation-plan.md）に後継が作られ、前任が消されなかった形
- [duplication] **ValidateAlias が platform と application に同名で二重実装されている**
  - 場所: `internal/platform/alias.go:32,39-44 vs internal/application/edit.go:161-175`
  - 根拠: platform 側は正規表現 `^[A-Za-z0-9][A-Za-z0-9._-]*$` + MaxAliasLength(64)、application 側は同じ規則をバイト走査で書き直し、上限 64 をリテラルで直書き（edit.go:162）。返すエラーも別（ErrUnsafeAlias / ErrInvalidAlias）。両者は現在同値だが、httpserver/config_requests.go:202 と application/connectioncreate.go:156 が application 版を、httpserver/password.go:293 などが platform 版を使っており、片方だけ変えても誰も気付けない

### internal/application, internal/config, internal/effective, internal/api（SSH 設定のドメインモデル層）

**責務** — この 3+1 パッケージは「ssh_config を読む → 値を決める → 編集して安全に書き戻す → HTTP 契約に載せる」という一本の流れを担う。責務の建前は config=無損失パーサと Include グラフ解決、effective=1 つの alias に対する実効値の決定（OpenSSH 相当の解決器）、application=ユースケース層（読み取りビュー構築、編集計画、トランザクション commit）、api=OpenAPI からの生成モデル、である。実際の依存方向は config → effective → application の一方向で保たれており（effective の製品コードは config しか import しない）、プラットフォーム差分もこの 4 パッケージには build tag 付き製品ファイルが 1 つも無く internal/platform に押し出されている。破綻しているのは「値を決める」責務で、2026-08-13 の resolver authority 移行が中途で止まり、旧実装 effective.Project と新実装 effective.Resolve が今も並走している。

**所見** — この領域の設計は「一貫していない」というより「一度だけ大きく方針を変え、その移行を最後まで走らせていない」状態にある。

まず擁護すべき点を先に置く。依存方向 config → effective → application は製品コードで厳密に守られており（effective の非テストファイルは internal/config しか import しない）、plan の Global Constraints が要求した一方向性は達成されている。4 パッケージのどれにも build tag 付きの製品ファイルが無く、Windows/macOS/Linux/Android の差は internal/platform と nativepath に押し出されている。internal/config は「無損失パーサ + Include グラフ」に責務が絞られていて肥大化しておらず、internal/api は openapi.yaml の 156 スキーマと機械的に一致する正当な生成物である（Makefile の verify-generated が再生成差分で落ちる仕組みも入っている）。application.MatchHostLine が effective.MatchPattern へ委譲した経緯（projection.go:136-141）のように、重複を明示的に潰した成功例も残っている。

破綻しているのは「値を決める」責務ただ一点で、そこから全部が派生している。2026-08-13 の config-resolver-authority は 8 タスク構成で、Task 8（ssh -G 経路の削除）は完了している——evaluate.go は消え、ParseValues は差分テスト専用として明示的に隔離され、README:245 も新しい約束を書いている。ところが Task 6（呼び出し側を Resolve へ寄せる）は ComputeEffective だけで止まり、effective.Project が製品コード 9 箇所に生き残った。順序が逆転している: spec が「先に 1 つへ寄せてから昇格させる」と警告していたのに、削除だけ先に済ませ統合は残った。結果として、旧エンジンの残骸は「使われないコード」ではなく「使われている 2 つ目の権威」として残っており、dead code より厄介である。実害も具体的で、Match ブロックを完全に無視する Project が PasswordEligibility の判定に使われている限り、Match 配下の PasswordAuthentication no は検出されない。

application の 21 ファイル 7613 行については、「機能追加のたびにファイルが増えただけ」という疑いは半分当たっている。純関数群（edit / move / region / groups / grouppath / diff / walk / projection）は責務で切られており粒度も揃っている。崩れているのは Service のメソッド群で、EditKind dispatch は service.go と fileops.go に理由なく二分され、connection / group / key の 3 ユースケースはそれぞれ独自の commit 経路を持ち込んだ。commit + コンフリクト変換が 4 回書かれ、しかも 4 つとも微妙に違う（KeyRelocations を運ぶか、非設定ファイルを素通しするか、requestFor を使うか）のは、共通化のタイミングを 3 回逃した結果に見える。grouprename.go:62 の「保存と同じコミット経路に通す」というコメントは、書いた時点の意図と実装が既にずれている。

internal/api は「過剰な抽象化」の教科書的な形をしている。TypeScript は openapi.yaml から直接生成されるので、Go 側の生成モデルは本来 httpserver が使うためだけに存在するはずだが、実際には 33 型が誰にも使われず、application 側に json タグまで一致する手書き双子が 36 組ある。しかも httpserver は同一ファイル内で 3 通りの方針（生成型を使う / 手書き双子を作る / application 型を直に流す）を採っており、どの endpoint が契約に縛られているかがコードから読めない。今のところ 36 組は形が一致しているが、それを保証しているものは何も無い。

最後に、コメントの陳腐化が構造的な問題と絡んでいる点を指摘しておく。このリポジトリのコメントは異様に密度が高く、判断の理由を長文で残す文化がある。それ自体は強みだが、方針変更に追随しなかったコメントは通常の陳腐化より有害になる——region.go:216-225 は既に修正済みの欠陥を「まだ直っていない」と説明し、httpserver/diagnostics.go:94-97 は存在しない action トークンの要件を説明し、application/effective.go:133 は実装されていないフォールバックを約束している。次に読む人はコードよりコメントを信じる可能性が高い。

**確度の高い指摘**

- [duplication] **「この alias は何に解決されるか」に答える実装が 2 つ並走している（resolver authority 移行が未完）**
  - 場所: `internal/effective/provenance.go:106, internal/effective/resolve.go:95, internal/application/passwordeligibility.go:82, internal/application/connectionupdate.go:406, internal/application/connectionupdate.go:442, internal/diagnostics/service.go:113, internal/diagnostics/service.go:170, internal/diagnostics/service.go:187, internal/effective/jump.go:170, internal/effective/jump.go:209, cmd/sshc/tui.go:51`
  - 根拠: docs/superpowers/specs/2026-08-13-config-resolver-authority-design.md:63-72 が決定事項 4 として「2 つある実装を 1 つにまとめる。片方を権威に昇格させ、もう片方を残すのは MatchHostLine のコメントが警告している状態そのものを権威の位置に作ることである。先に 1 つへ寄せてから昇格させる」と明記。plan の Task 6 も internal/diagnostics/service.go:179 の書き換えを指示している。実際には ComputeEffective のみ Resolve へ載せ替えられ（application/effective.go:65-67）、Project は残置。`rg 'effective\.Project\(|Project\(w\.graph'` の製品コード（非テスト）ヒットは 9 件。plan Task 8 の evaluate.go 削除は完了しているので、削除されたのは ssh -G 経路だけで、2 実装の統合という前提条件は飛ばされている
- [layering] **Project は Match ブロックを一切適用しないため、PasswordEligibility が Match 配下の PasswordAuthentication no を見落とす**
  - 場所: `internal/application/passwordeligibility.go:82, internal/application/passwordeligibility.go:120, internal/effective/provenance.go:117, internal/effective/provenance.go:252`
  - 根拠: provenance.go:252-253 の blockApplies が BlockMatch に対して常に ("", false) を返し、Project の enterBlock（:118-127）は Match を complexity に積んで即 return するため、Match 配下のディレクティブは Source に一切現れない。PasswordEligibility は projection.Value("PasswordAuthentication") しか見ず（:120-123）、projection.Complexities を読まない。結果、`Match host db` 内で PasswordAuthentication no と書かれた設定では BlockerPasswordAuthenticationOff が立たず、コメント（:20-24）が「使えない秘密を保存することになり両方の意味で最悪」と断じている状態がそのまま起きる。一方 Resolve は同じ設定で Match を評価する（resolve.go:179-204）ので、接続経路とは答えが食い違う
- [duplication] **commit と ConflictError→ConflictReport 変換のパイプラインが 4 回書かれ、しかも挙動が揃っていない**
  - 場所: `internal/application/service.go:532, internal/application/grouprename.go:63, internal/application/connectioncreate.go:369, internal/application/keymove.go:124`
  - 根拠: 4 箇所すべてが「pendingBase/pendingBaseline を立てる → manager.Commit → errors.As(&conflict) → 該当 change を探して BuildConflictReport」という同じ 15〜20 行を持つ（var conflict *storage.ConflictError の出現行: service.go:556, grouprename.go:80, connectioncreate.go:379, keymove.go:147）。差分は網羅されていない: (1) connectioncreate.go:381-383 だけが prepared.base に無いパスを設定ファイルでないとして素通しし、他 3 つは base[cleaned] が nil のまま Report を組む。(2) grouprename.go:104 は SaveResult に KeyRelocations を載せるが service.go:576 の Save は載せない。(3) keymove.go:136-138 だけがトランザクション外で EnsureDirectory を呼び、requestFor も使わず storage.Request を直に組むので removals / directories が運ばれない。grouprename.go:62 のコメントは「保存と同じコミット経路に通す」と書いているが、実際には経路を共有せず複製している
- [dead-code] **生成モデル 33 型が Go から未使用で、internal/application に json タグまで一致する手書き双子が 36 組ある**
  - 場所: `internal/api/models.gen.go:1, internal/application/effective.go:15, internal/application/projection.go:73, internal/application/service.go:80`
  - 根拠: models.gen.go の 203 型のうち、*JSONRequestBody / *Params を除いて Go の他パッケージから一度も参照されない型が 33 個（Effective, EffectiveEntry, EffectiveChange, EffectiveDiff, Source, EditRequest, FieldEdit, FileDiff, DiffLine, ConflictReport, HostEntry, HostForm, FormField, FileRef, FileNode, FileContents, Metadata, GroupMetadata, HostMetadata, GroupView, HistoryEntry, PendingTransaction, IncludeReference, Setting, Diagnostic, EmbeddedTerminal, RecoverRequest/Response, RestoreRequest, GroupRenameRequest, GroupDeleteRequest ほか）。同名・同 json 形状の手書き型を application 側と機械比較すると 36 組が一致し、不一致は SaveResult の json:"-" と TerminalSettings（json タグを持たない設計）のみ。TypeScript は web/package.json:9 の openapi-typescript が openapi.yaml から直接生成しているため、これら 33 型は Go 側では純粋な重量物。なお生成物自体は生成元と一致している（openapi.yaml の 156 スキーマがすべて型として存在し、余剰は oapi-codegen が作る別名のみ）

### HTTP境界・セッション・合成の根・エンジン所有権（internal/httpserver, session, app, enginelock, handoff, ui）

**責務** — エンジン常駐プロセスの「外殻」一式。internal/app が全サービスを組み立てて internal/httpserver に注入する合成の根であり、httpserver は 83 本のルートで 3 つの認証面（ブラウザ用 /api/v1/*、CLI用 /cli/*、WebSocket用 /terminal/stream）を提供する。internal/session は localhost セッション cookie・CSRF・一度限りの action token を、internal/enginelock は「エンジンは1台」を OS のファイルロックで、internal/handoff は「動いているエンジンの居場所」を state ディレクトリの原子的な文書で保証する。internal/ui は go:embed が自パッケージ配下しか埋め込めないという制約だけのために存在する 13 行の薄膜である。

**所見** — **この領域の骨格は健全である。** ルート登録は13個の registerXxxRoutes を server.go の New() が一箇所から呼ぶ形に集約されており（server.go:288-416）、散らばってはいない。停止順序（server.go:130-232 と app/run.go:479-540 の unwind 8段）、エンジンの単一性（enginelock）、handoff の原子性と所有権（secret による削除認可）は、いずれも「なぜそう書いたか」がコメントに残る形で設計されており、破綻していない。internal/ui の13行も、go:embed のパス制約という明確な理由を持ち CI が鮮度を強制している（ci.yml:358）ので、過剰抽象ではない。internal/app も薄いラッパーではなく、20依存を組み立てて httpserver.Options の24フィールドを埋める本物の合成の根である。

**方針変更の痕跡は、主に「通信形式をどこが所有するか」に集中している。** internal/api（OpenAPI 生成型）を通信の正本にする方針と、application 層の型に json タグを付けて直接シリアライズする方針が、同じ httpserver パッケージの中で同居している。config/metadata/history 系は後者、connections/keys/terminal/sync/password 系は前者。その結果、生成された203型のうち82型が Go から一度も名指されず、config_handlers.go:26-55 は生成型と字面まで同じ構造体を6個手書きし、problem の本体も api.Problem と problemPayload の2型に割れ、片方は openapi.yaml:1293 の additionalProperties: false に違反する blockers を返す——しかもその blockers を読む画面は web/src に存在しない。docs/superpowers/plans/ に 2026-08-04 の foundation から 08-05 の config-engine、08-10 の connection-creation-modal、08-13 の embedded-terminal と積み上がっている順序を見ると、後から足された機能ほど生成型を使っており、最初期の config エンジンだけが手書きのまま取り残されたと読める。

**2つ目の痕跡は「外部 ssh を起こす」から「プロセス内で SSH を話す」への転換（2026-08-13 の in-process-ssh-client / retiring-the-external-ssh）である。** これは概ね完了しているが、secret パッケージに askpass トークンの定数（TokenTTL, ErrUnknownToken）が参照0件で残り、そのコメントは「OpenSSH がパスワードのプロンプトに到達するまで」という消えたフローを説明し続けている。同じ転換で internal/app/ssh.go に CLIConnection が生まれたが、これは cmd/sshc 専用で app.Run/Build からは一切使われず、README.md:389 と ssh.go:27-29 が「組み立てる場所はひとつ」と主張しているのに実際は sshParts と2系統ある。複製された相対パス判定（ssh.go:132 と :224）が片方だけ緩い、という典型的な劣化も出ている。

**3つ目は Android の後付け（2026-08-16）である。** desktop/headless の2値しかない handoff.Owner に対して3つ目の経路が現れ、Android は OwnerDesktop を名乗ることで解決している。app.Run は Owner を問わず CLI secret を発行し handoff を書き（書けなければ致命）、httpserver は無条件に /cli/* を8本立てるが、README.md:52 のとおり Android に CLI は無い。enginelock の機構自体は3経路が同じ Acquire を通っており、ここは分岐していない——分岐しているのは「エンジンの持ち主」という語彙の方である。

**確認できていないこと:** go/node が無いためコンパイルもテストも走らせていない。上記の未使用判定はすべて `rg` による静的な参照数え上げに基づくもので、リフレクション・ビルドタグ差（windows 専用ファイルは読んだが実行検証はしていない）・go:generate 経由の間接参照は捕捉できていない。openapi 契約違反（blockers）は yaml とコードの突き合わせによる判断で、実際のレスポンスをスキーマ検証にかけたわけではない。

**確度の高い指摘**

- [duplication] **生成済み契約型を無視して同じ形の構造体を手書きしている（config 系6個）**
  - 場所: `internal/httpserver/config_handlers.go:26-55, internal/api/models.gen.go:398,553,569,621,995,1001,1093`
  - 根拠: config_handlers.go が groupRenameRequest(26)/groupDeleteRequest(31)/historyList(39)/restoreRequest(43)/recoverRequest(48)/recoverResponse(53) を手書きしているが、api/openapi.yaml から生成された api.GroupRenameRequest(569)/GroupDeleteRequest(553)/HistoryList(621)/RestoreRequest(1093)/RecoverRequest(995)/RecoverResponse(1001) が JSON タグまで同一の形で既に存在する。`rg 'api\.EditRequest' --type go` は0件で、生成型 api.EditRequest(models.gen.go:398) は完全に未使用。生成された203型のうち82型が非テスト Go コードから一度も名指されていない（うち約30は oapi-codegen が常に吐く *JSONRequestBody）。make generate / make verify-generated という仕組みがあるのに、config 系エンドポイントだけがそこから外れている
- [layering] **config 系は application 層の型をそのまま通信形式にしている。同じパッケージ内で connections 系とは正反対の方針**
  - 場所: `internal/httpserver/config_handlers.go:177-186, internal/application/service.go:82-101, internal/httpserver/connections.go:34-91`
  - 根拠: ConfigHandlers.decodeEdit は JSON を application.EditRequest へ直接デコードし、Preview/Save の応答も application.SavePreview / SaveResult をそのまま c.JSON する。application/service.go:82 の EditRequest は json タグを持っており、application 層が通信契約を負っている。一方で ConnectionHandlers は api.CreateConnectionRequest / api.UpdateConnectionRequest で受けてから application.* へ約300行かけて手変換する（connections.go:93-400 の decodeStringConnectionChange 等7関数）。同一パッケージ・同一 HTTP 境界で「変換層を置く／置かない」が真逆で、どちらが方針なのかコード上から決められない
- [layering] **problem レスポンスが2つの型で二重定義され、片方は OpenAPI の閉じたスキーマに無いフィールドを返している**
  - 場所: `internal/httpserver/security.go:188-191, internal/httpserver/config_requests.go:45-57,69-75,278-281, api/openapi.yaml:1291-1303`
  - 根拠: application/problem+json の本体が api.Problem（security.go:188 の problem()、15ファイル・約240箇所が使用）と problemPayload（config_requests.go:45、20箇所）の2型ある。openapi.yaml:1293 の Problem スキーマは additionalProperties: false で、プロパティは code/message/detail/path/line/column/diagnostics/conflict の8つ。しかし problemPayload は blockers を持ち（:56）、group_blocked のときに実際に出力する（:278-281）。この応答が返るのは /api/v1/config/groups/rename と /delete で、openapi.yaml:933,957 はどちらも 409 に $ref: Problem を宣言している。契約違反。しかも `rg 'group_blocked' web/src` は0件で、この blockers を読む画面はどこにも無い（web/src/api/schema.d.ts:1049-1058 の生成型にも blockers は無い）
- [duplication] **「プロセス内 SSH は internal/app が一箇所で組み立てる」という README の主張に反し、同じファイル内で2回組み立てている**
  - 場所: `internal/app/ssh.go:30-62, internal/app/ssh.go:195-245, README.md:389`
  - 根拠: sshParts(ssh.go:30) と CLIConnection(ssh.go:195) はいずれも {dialer sshclient.Dialer; resolve sshclient.Resolver; home string} の3フィールドを持ち、newSSHParts(:36) と NewCLIConnection(:206) がほぼ同じ Dialer{Auth{AgentSocket, Stored, Password}, HostKeys{Read, Add}} を別々に組み立てる。ssh.go:27-29 のコメント自身が「**組み立てる場所はここひとつである。**…二箇所で組み立てると、片方だけが vault を見る日が来る」と書いており、README.md:389 も同じ主張をしている。実際には CLIConnection は vault を見ない（別プロセスなので見られない）——README の「埋め込みターミナルも `sshc <接続先>` も…同じ鍵・同じ known_hosts・同じ解決器を使います」は `sshc <接続先>` について事実ではない。CLIConnection は自前で storage.NewWorkspace / application.NewService / knownhosts.NewService を作り直す（ssh.go:211-217）

### internal/storage, internal/secret, internal/envelope, internal/keys, internal/remotekey（永続化・秘密・鍵素材の層）

**責務** — この 5 パッケージは「~/.ssh の中身を安全に書き換える」責務を縦に分担している。storage は ~/.ssh 全体をワークスペースとして固定し、ジャーナル + 世代バックアップ付きの原子的な複数ファイル書き込みと、その巻き戻し／履歴を提供する唯一のディスク経路である（保管庫そのものは持たず、保管庫ファイルもこの層を通って書かれる）。envelope は Argon2id + AES-256-GCM の自己記述的な封をひとつだけ定義し、secret（保管庫）と remotesync（スナップショット）の両方に供給する。secret はその封の上に「マスターパスワードで開く保管庫」と、パスワード／鍵パスフレーズ／同期設定／生体入口の寿命管理を載せる。keys は ~/.ssh 配下の鍵素材の棚卸し・生成・パスフレーズ変更・ごみ箱・agent 連携を持ち、remotekey はリモートの authorized_keys へ公開鍵を追記する単発の実行だけを持つ。

**所見** — この層は「なぜそうしたか」を書き残す規律が非常に高く、方針変更のほとんどはコメントの中に自己申告として残っている。だから破綻の形も特徴的で、壊れているのは個々の判断ではなく「前の方針の下で書かれた足場が、新しい方針の下で撤去されなかった」箇所に集中している。

追える方針変更は 4 本ある。

1. **名前付き資格情報 → dedicated 併存（08-06 → 08-10/08-11）。** 2026-08-06-master-password-implementation-plan.md の Task 1 は Kind による 2 名前空間と `map[Kind]...` の 2 マップだけを設計しており、そこには dedicated という概念が無い。その後 2026-08-10-connection-creation-modal / 2026-08-11-secret-host-assignments / 2026-08-11-connection-key-passphrase-and-settings が「1 ホスト専用・1 鍵専用の秘密は共有できてはならない」を持ち込み、dedicatedPasswords / dedicatedKeyPassphrases が追加された。ただしこの 2 つは Kind で索かれず素のフィールドのまま入ったため、Vault の 6 メソッドが恒久的に kind 分岐を抱えることになり、Unassign の非対称（findings #4）という実挙動の食い違いまで生んでいる。secret 層で最も費用が続いている破綻はここ。

2. **同期設定の置き場所と鍵（08-06 → 08-18）。** 08-06 プランは `func (v *Vault) Sync() SyncSettings` と `SetSync` を保管庫の中に置く設計だったが、実装は保管庫の外の `sshc/sync-settings` へ出した（vault.go:31-40 が「バケットへの鍵をバケットの中に入れることになる」と理由を明記）。さらに 2026-08-18-sync-key-of-its-own-design.md で「スナップショットを封じるのはマスターパスワードではなく専用の鍵」へ再度変わり、SyncSettings に Auto と Key が後付けされた（vault.go:120-138 のコメントが「それを使っていたので…1 台で変えれば他の全端末が締め出された」と前の方針を明示的に否定している）。この変更は行儀よく完了しており、残骸は「同じパス文字列が secret と remotesync に二重定義されている」（findings #13）程度。

3. **askpass 中継の廃止（08-11 → 08-13/08-14）。** docs/superpowers/specs/2026-08-11-askpass-relay-design.md 時代の「単回トークンを発行して別プロセスに引き換えさせる」設計が in-process SSH クライアント（2026-08-13-in-process-ssh-client）で不要になり、README.md:379 が「返るのは単回トークンではなく答えそのものです」と決着を書いている。しかし secret には ErrUnknownToken・TokenTTL と、write() の中の「トークンを全部無効化する」というコードを伴わないコメントが丸ごと残った（findings #5）。これは純粋な削除漏れで、直すのが最も安い。

4. **鍵素材のバックアップ方針の反転（初期 → 封をする now）。** 当初は「秘密鍵の 2 つ目のコピーを backups/ に置かない」ために SkipBackup を使い、その代償として取り消しができなかった。世代バックアップをマスターパスワードで封じるようになってこの理由は消え、パスフレーズ変更はバックアップを取るように戻された（README.md:287、internal/keys/service_test.go:1168-1174 のテストコメントが経緯を書いている）。**この反転だけが未完のまま残っている。** 封をする関数を差す責任は配線側（internal/app/run.go）にあり、そこは production の 3 つの Manager のうち 1 つにしか差していない。鍵 vault は意図的に別 Manager を持つ（設定バリデータを避けるため、run.go:127-137 に理由あり）が、その分岐を作ったときに Seal を一緒に持って行くのを忘れている。テストは自分で Seal を差すため通り、README は封じられている前提で書かれている——コード・テスト・文書の 3 つが独立に正しく見えて、実物だけが平文という形になっている。findings #1/#2 はこの 1 つの穴の 2 つの顔である。

一方で、疑われていたもののうち**成立していない**ものも明確にしておく。

- **envelope は secret に畳めない。** remotesync と httpserver が独立に import しており、畳めば remotesync → secret の依存が生まれる。internal/app/run.go:290-292 がその依存を明示的に禁じ、遵守されている（remotesync の非テストコードは secret を import していない）。380 行で 2 消費者・2 コスト上限プロファイルを支えており、薄すぎも厚すぎもしない。
- **remotekey は独立でよい。** keys の外向き依存は config/platform/storage の 3 つに保たれており、remotekey が必要とする effective/knownhosts/sshclient を keys に持ち込めばこの層の依存が一気に太る。remotekey が keys を import しないのも正しく、両者の橋渡しは httpserver（remotekey.go:41-45）で行われている。235 行のパッケージが 1 つ増える対価としては安い。
- **storage のプラットフォーム差は正しく閉じている。** build tag 付き 12 本のうち非テストは 2 本だけで、差は 6 つの非公開関数 + Windows 専用の ReadPrivateFile に完全に局所化されている。Windows 側が Unix 側の 8.7 倍あるのは NtCreateFile による reparse 無効化という実際の必要から来ており、水増しではない。

最後に、この層の**テスト量**（storage: 本体 2646 行に対しテスト 4433 行、secret: 2141 対 2834）は過剰ではなく、ジャーナルの中断復旧と Windows の ACL という「壊れ方が静かな」領域に集中している。ただし内部 fake が本番配線と食い違ったまま緑になる箇所が実在する（findings #1）ので、テストが多いこと自体は配線の正しさを保証していない。

**確度の高い指摘**

- [layering] **鍵 vault 用の storage.Manager に Seal が配線されず、パスフレーズ変更が平文の秘密鍵を世代バックアップに残す**
  - 場所: `internal/app/run.go:141-142, internal/app/run.go:265-266, internal/keys/service.go:451-458`
  - 根拠: production では storage.NewManager が 3 箇所で呼ばれる（run.go:142 = 鍵 vault 用、run.go:204 = 設定/known_hosts/secret/remotesync 用、app/ssh.go:215 = CLI 用）。`grep -rn '\.Seal *=' --include='*.go'` の結果は run.go:265 の 1 件のみで、対象は run.go:204 の manager。鍵 vault 用 manager に Seal を差す経路は無く（keys.Service に manager のアクセサも無い）、keys.Service.ChangePassphrase は SkipBackup を付けずに秘密鍵を storage.Change でコミットする（service.go:451-458。`grep -rn 'SkipBackup: *true'` の非テスト結果 5 件はすべて secret/remotesync で、keys には 0 件）。したがって置き換え前の秘密鍵が ~/.ssh/sshc/backups/<id>/ に平文で残る。テストが通るのはテスト側が自分で manager.Seal を差しているためで、ヘルパのコメント（internal/keys/service_test.go:56-58「アプリケーションはすべての世代バックアップをマスターパスワードで封じるので、これらのテストもそうする」）はアプリケーションの実際の配線と一致していない。
- [stale-doc] **README の「世代バックアップは全て暗号文」が実装と食い違う**
  - 場所: `README.md:287, README.md:311, internal/app/run.go:141-142, internal/app/ssh.go:215`
  - 根拠: README.md:311「**世代バックアップは全てマスターパスワードで封をします。** ~/.ssh/sshc/backups/ にあるファイルは、置き換えられた設定ファイルであれ秘密鍵であれ vault 自身であれ、暗号文です」、README.md:287「現在はバックアップを取ります。世代バックアップはマスターパスワードで封をするため、平文の鍵が残らなくなったからです」。実際には 3 つの production manager のうち 2 つ（鍵 vault 用と CLI 用）が Seal 未設定で、storage.Manager は Seal が nil のとき平文で書く（internal/storage/transaction.go:283-288 の doc と、バックアップ書き込み時の `if m.Seal != nil` 分岐）。README の記述は run.go:204 の manager を通る書き込みについてのみ真。

### web/ (React フロントエンド: src 全体, e2e, vite.config.ts, package.json)

**責務** — Go バイナリに `internal/ui/dist` として埋め込まれる単一ページアプリケーション。~/.ssh 設定エンジン・鍵サービス・秘密の保管庫・埋め込みターミナル・リモート同期という 5 つのサーバー側サブシステムを、13 個の「セクション」として 1 枚のシェル (App.tsx) の下に並べる。API は OpenAPI から生成した型 (src/api/schema.d.ts) を土台に、実行時ガードを添えた API クライアント群越しに呼ぶ。i18n は en/ja の 2 カタログ、色は index.css の 20 トークン、ルーティングは History API を直接叩く自前実装で、React Router 等のライブラリは使わない。

**所見** — この領域は「破綻」というより**層ごとに健全度が大きく違う**。方針変更の痕跡は主に 3 つの断層に集中している。

**断層 1: connections/ の周りで起きた 1 日の方針反転（最も明確）。** 2026-08-11 の計画は「ConnectionTree.tsx とそのテストを削除し、ConnectionBrowser.tsx を作り、グループのドリルダウンを URL addressable にする」と明記した。翌 08-12 の計画は「接続画面を常時展開の管理ツリーへ戻し、ドリルダウンをホームのクイック接続へ移す」「Create: ConnectionTree.tsx」と明記した。現在の木には、ConnectionBrowser.tsx が無く ConnectionTree.tsx が復活し、08-11 のために作られた connectionBrowser.ts だけが connections/ に取り残されて overview/ から使われている。そして ConnectionTree.tsx は、connectionBrowser.ts が既に持っている index 構築（identityKey / nearestParent / matches / metadata 結合 / 二段ソート）をほぼ 1:1 で再実装している。ユーザーが疑っていた「途中で方針が変わったせいの重複」は、ここに文書と実装の両方で残っている。

**断層 2: 抽象の導入が「全画面移行」まで行かずに止まった。** ui/surface.tsx の Button・Card は connections/ ではほぼ全面採用されているのに、keys / sync / explorer / groups / secrets / knownhosts / diagnostics / history / overview では ui/form.tsx のクラス文字列（101 箇所）が使われ続ける。同じ現象が API 層にもある: `api?: IntegrationsApi` による注入は 7 コンポーネントに入ったが、configApi だけは interface すら公開されず 5 コンポーネントが `vi.mock` に取り残された。生成 request 型も同じで、api/ の古い 5 メソッドだけが使い、後から足された keys/ と remotekeys/ は手書き型に戻っている。**どれも「新しい作法が旧作法を置き換えた」のではなく「新しい作法が旧作法の隣に増えた」形**で、両方が生きている。

**断層 3: 削除されるべきものが、同居する助っ人のせいで生き延びた。** PasswordPanel.tsx（330 行 + テスト 226 行）は ConnectionBasicForm.tsx への UI 統合で役目を終えたが、同ファイル内の 5 行の `eligibilityText` が ConnectionBasicForm から import され続けているために、ファイルごと消せずに残った。ファイル冒頭の「このパネルはホストエディタの内側でしかレンダリングされない」というコメントは、既に事実ではない。同型の残骸が authenticationPolicy.ts:16 の hasDirectIdentityFile（呼び出し側 2 箇所がインライン展開したせいで宙に浮いた）と App.tsx:1025 の到達不能分岐。

**一方で、明確に健全な部分がある。** (1) i18n は 1094 キー・en/ja 完全一致・未使用キー実質ゼロで、型（`satisfies Record<keyof typeof en, …>`）と catalogue.test.ts の二重で整合が守られている。(2) terminal/ の 12 ファイルは相互参照が閉じており、未使用ファイル・未使用 export（テスト用の 2 つを除く）が無い。(3) routing/sectionRoute.ts は 62 行の純関数で、App.tsx の 3 つの `Record<Section, …>` が網羅を型で強制する。(4) 全 tsx コンポーネント 60 個中、本番でレンダリングされないものは PasswordPanel ただ 1 つで、アイコン 18 個も全て使われている。(5) `noUncheckedIndexedAccess` と `exactOptionalPropertyTypes` を有効にしている点、`ui/palette.test.ts` が「コメントの中にしか生きていないルールは朽ちる」と書いて配色規約を実行可能な検査にしている点は、この規模のプロジェクトとしては相当に真面目。

**結論として、直す順序を付けるならこうなる。** (a) PasswordPanel の廃止と eligibilityText の移設（dead code 330 行を一撃で消せる、影響範囲が最小）、(b) asRecord 系ガードと toProblem と issueAction を api/client.ts へ集約（機械的、リスクゼロ）、(c) ConnectionTree を connectionBrowser の index の上に載せ直す（08-12 の反転で発生した重複の解消）、(d) ui/form.tsx のクラス文字列を surface.tsx のコンポーネントへ寄せる（大きいが機械的）、(e) KeysScreen と ConnectionsPage の分割（JSX 上の切れ目は既に見えているので設計判断はほぼ不要）。App.tsx の 23 prop バッグは (e) より先に触ると衝突するので最後。

**確度の高い指摘**

- [dead-code] **PasswordPanel（330行）は本番で一度もレンダリングされない**
  - 場所: `web/src/diagnostics/PasswordPanel.tsx:43`
  - 根拠: `rg '<PasswordPanel'` の非テストヒット 0。全 tsx の export コンポーネントを走査して本番レンダリング箇所を数えた結果、未使用はこの 1 つだけ（他は全て到達可能）。このファイルから本番で import されるのは 5 行の eligibilityText（:39）だけで、唯一の消費者は ConnectionBasicForm.tsx:18,799,802。テストは PasswordPanel.test.tsx に 226 行残る。:16-18 のコメント「このパネルはホストエディタの内側でしかレンダリングされない」も事実と食い違う。実際のパスワード UI は ConnectionBasicForm.tsx へ移され、e2e/password.spec.ts:12 が region "Authentication" と "Stored password action" を叩いている
- [duplication] **実行時ガード asRecord/asArray/asString/asNumber/asBoolean が 4 モジュールに逐語コピーされている**
  - 場所: `web/src/api/config.ts:35-50, web/src/api/integrations.ts:143-176, web/src/keys/api.ts:99-124, web/src/remotekeys/api.ts:26-51`
  - 根拠: `rg 'function asRecord'` 4 件、asString 4 件、asArray 4 件、asNumber 3 件、asBoolean 3 件。本文はいずれもバイト単位で同一（throw new Error("invalid_response") まで）。4 ファイルとも直前に「生成された型は契約を記述するに過ぎない…」という同趣旨のコメントを別々の日本語で書いている
- [duplication] **toProblem が 4 ファイルに逐語コピーされている**
  - 場所: `web/src/connections/ConnectionsPage.tsx:77-81, web/src/explorer/ConfigExplorer.tsx:26-30, web/src/groups/GroupsPanel.tsx:24-28, web/src/history/HistoryPanel.tsx:9-13`
  - 根拠: `rg -A6 '^function toProblem'` の 4 ヒットが 5 行とも完全一致。ApiError の定義は api/client.ts:6、failureCode は同 :23 にあるのに、Problem への正規化だけが画面側に 4 つ散っている
- [duplication] **ConnectionTree が connectionBrowser の index 構築ロジックを丸ごと再実装している**
  - 場所: `web/src/connections/ConnectionTree.tsx:42,48-55,57-65,83-107 / web/src/connections/connectionBrowser.ts:52,69-78,55-63,97-120`
  - 根拠: identityKey（ConnectionTree.tsx:42 と connectionBrowser.ts:52）は同一式。nearestParent（:57-65）と nearestDeclaredParent（:55-63）は変数名以外同一。hostMatches（:48-55）と matches（:69-78）は alias/patterns/group/tags の 4 条件が同一。metadata 結合＋order→sourceOrder 二段ソート（:83-107）は buildConnectionBrowserIndex（:97-120）と同手順。ConnectionTree が connectionBrowser から取っているのは duplicateAliasesOf 1 つのみ（:6,90）。identityKey は OrphanPanel.tsx:12 に 3 つ目の定義があり、QuickConnectBrowser.tsx:48 と ConnectionTree.tsx:86 にインライン展開が 2 つ（計 5 か所）
- [layering] **生成された Request スキーマ 26 個を使わず、更新系ボディを手書き型とオブジェクトリテラルで送っている**
  - 場所: `web/src/keys/api.ts:43-76, web/src/remotekeys/api.ts:8-17`
  - 根拠: openapi.yaml は GenerateKeyRequest(1491) / HardwareCommandRequest(1516) / ChangePassphraseRequest(1533) / RelocateKeyRequest(2368) / RegisterKeyRequest(2405) / RemoteKeyPlanRequest(1698) / RemoteKeyRegisterRequest(1730) / KnownHostsAddRequest(1821) など 26 の request スキーマを定義するが、schema.d.ts の 156 スキーマ中クライアントがエイリアスしているのは 89 で、上記 26 は 1 つも参照されない。代わりに keys/api.ts:48 GenerateKeyInput / :58 HardwareCommandInput / :65 PassphraseInput / :73 RegisterAgentInput / :43 KeyLocationInput、remotekeys/api.ts:8 RemoteKeyInput が手書きされる。PassphraseInput・RegisterAgentInput・KeyLocationInput は生成型とフィールド完全一致の純粋な重複、GenerateKeyInput と HardwareCommandInput は group を必須にしており契約（optional）と食い違う。生成 request 型を使っているのは EditRequest / CreateConnectionRequest / UpdateConnectionRequest / OpenTerminalSessionRequest / SyncSettingsRequest の 5 だけで、api/ の古い部分だけが規約を守っている
- [platform-divergence] **2026-08-11 と 08-12 の計画が 1 日で逆転し、両方の産物が残っている**
  - 場所: `docs/superpowers/plans/2026-08-11-connection-browser-drilldown-implementation-plan.md:33-36, docs/superpowers/plans/2026-08-12-quick-connect-group-browser-implementation-plan.md:5-7, web/src/connections/connectionBrowser.ts:1, web/src/connections/ConnectionTree.tsx:1`
  - 根拠: 08-11 計画の File map に「Create web/src/connections/ConnectionBrowser.tsx」「Delete web/src/connections/ConnectionTree.tsx and its test after ConnectionsPage switches to the browser」。翌日 08-12 計画の Architecture に「接続画面には管理操作を持つ ConnectionTree を復元する」「Create: web/src/connections/ConnectionTree.tsx」。現状 ConnectionBrowser.tsx は存在せず、ConnectionTree.tsx は復活し、08-11 のために作られた connectionBrowser.ts だけが connections/ に取り残され overview/ から使われている。上記の ConnectionTree 重複はこの逆転の直接の結果
- [over-abstraction] **App.tsx の SectionViewProps が 23 prop の受け渡し袋になっている**
  - 場所: `web/src/App.tsx:844-873, :875-911, :958-1030`
  - 根拠: 型は 23 prop を宣言（844-873 のメンバー行を数えて 23）。SectionView は Connections 用に 12 を使い、残りを {...props} で PaddedSection へ丸投げ（:905）。PaddedSection が分割代入するのは 15 で、connectionDraft / onConnectionDraftChange / onNavigateForCreation / location / onNavigationBlockerChange / preferredConnectionKey / onPreferredConnectionKeyApplied / onOpenFile の 8 は素通りする。画面をまたぐハンドオフ（生成鍵→Connections:333、公開鍵→Remote Keys:338、作成ドラフト→Groups/Keys:181、ファイル位置→Config:328）を全て App の useState で持つのが原因

### docs/superpowers/plans（43本）・README.md・git log — 方針変遷の年表と、ピボットが残した痕跡

**責務** — この領域は「何を作るか」を決め、決めたことを後から覆した記録そのものである。2026-08-04 から 08-16 までの 13 日間に 43 本の実装計画が書かれ、735 コミットが積まれた。計画文書は単なる履歴ではなく、`2026-08-04-ssh-ui-roadmap.md` が「延期したもの・覆したもの・作らなかったもの」を追記し続ける生きた台帳として機能しており、README.md（411行）は各境界について「失ったもの／得たもの」を明記する事実上の設計正本になっている。ただしこの三者（計画・README・コード）は同期しておらず、ピボットのたびに片方だけが更新された箇所が残っている。

**所見** — ■ 全体の所見

43本の計画は「6サブシステムを順に積む」という 08-04 の roadmap から始まり、08-09 以降は roadmap の統制を離れて 1〜2 日ごとの独立した計画に切り替わっている。方針変更は隠されておらず、README と roadmap には「失ったもの／得たもの」がかなり誠実に書かれている。ソフトウェア設計が破綻しているというより、**判断の速度に対して、判断の痕跡を実装から取り除く速度が追いついていない**というのが実態である。735 コミット中 62 件（8.4%）が削除系の語を含み、`git revert` は 0 件——すべての反転は revert ではなく書き直しで行われた。したがって「旧方式の断片」は diff では追えず、grep でしか見つからない。

■ 年表：ピボットの系列

【系列1: 外部 ssh → プロセス内 SSH】
- 08-05 ssh-integrations が `platform.OutputRunner` を「すべての外部プロセスが通る単一の継ぎ目」として作る。
- 08-05 password-authentication が `SSH_ASKPASS` + ワンタイムトークンを作る（計画では Keychain 保存 → 実装中に暗号化ファイルへ反転）。
- 08-13 01:30 (d40bfe3) embedded-terminal が、CLI に閉じていた ssh 起動一式を `internal/platform/interactive.go` へ引き上げる（`InteractiveSSH` / `FreezeSSHConfig`）。**「二つ目の実装は作らない」と書かれた共通化である。**
- 08-13 17:54 (c165c78) / 18:43 (171b70e) config-resolver-authority が `ssh -G` の権威を `effective.Resolve` へ移し、`internal/effective/evaluate.go` を削除。
- 08-13 21:25 (2462d9b) retiring-the-external-ssh が `SSH_ASKPASS` 一式・`cmd/sshc/askpass.go`・そして **20時間前に作ったばかりの `internal/platform/interactive.go`** を削除。
- 08-13 (0d6bdb7) `ssh-add` と `ssh -Q key` も退け、`platform.Toolchain` は `KeyGen()` 1メソッドへ。
→ 生存確認: `InteractiveSSH` / `FreezeSSHConfig` / `AskpassCredential` / `platform.MinimalEnvironment` / `Toolchain.KeyScan` / `writeConfigSnapshot` / `diagnostics.HardeningOptions` はいずれも非 docs 参照 0 件。**死んでいる。**
→ 残骸: F1（OutputRunner 継ぎ目）、F4（SSHC_ASKPASS_* ポリシー）、F3（secret の TokenTTL / ErrUnknownToken）、F5（ParseValues）、F10（openapi の askpass 文言）。

【系列2: 常駐サービス（launchd/systemd）】
- 08-07 (3cc8ea7) `internal/platform/macos/loginitem.go` 誕生。
- 08-09 linux-support が systemd user unit を追加。
- 08-12 16:21 (a056a25) stable-install-and-service-rebind が `cmd/sshc/service.go` と `sshc service refresh|disable` を追加。
- 08-15 03:27 (3747db0) single-app Task 7 が **loginitem・service サブコマンド・`/api/v1/login-item`・設定画面のトグル・i18n の `settings.loginItem*` を丸ごと削除。** `cmd/sshc/service.go` の寿命は 2日11時間。
→ 生存確認: `LoginItem` / `ServiceSubcommand` / `serviceLoginItem` / `runService` / `settings.loginItem` の非 docs 参照は 0 件、Makefile に `service` 系ターゲットも無し（`install:` と `uninstall:` はバイナリの出し入れだけ）。**この系列は最もきれいに畳まれている。** README.md:91,374 が削除の事実と理由を明記。

【系列3: 複数アプリ構成 → single-app → エンジン所有権の再設計】
- 08-13 (20d4770) `cmd/sshc/engine.go` 誕生。
- 08-15 04:03 (d539e59) single-app Task 8 が `engine.go` / `detach_unix.go` / `internal/application/desktop.go` / `KeepRunning` を削除。「画面の無い機械でエンジンだけを動かす道は無くなる」と明記。2586df2 が `/cli/engine` も削除。
- 08-16 09:32 (4944849) explicit-engine-owners が **`cmd/sshc/engine.go` を別の意味で復活**（stdin を所有権チャンネルとする desktop 専用の入口）。同時に `sshc headless` を新設し、single-app が閉じた道を部分的に開け直した。空白期間 29時間30分。
→ 生存確認: `KeepRunning` は非 docs 参照 0 件。`internal/acceptance/documentation_test.go:38` が「README に `--own-engine`・`sshc engine start`・`make desktop-dist` が現れないこと」を検査しており、削除が検査で固定されている。**残骸なし。** ただし README 内の記述の新旧が混在（F20）。

【系列4: 外部端末アプリ起動 → 埋め込みターミナル】
- 08-05 (ee1c94d) `internal/platform/macos/terminal.go`（AppleScript・バンドル ID の表）誕生。
- 08-13 (d40bfe3) embedded-terminal が削除。計画の D8 に削除対象の表があり、`TerminalChoice`・`TerminalID`・`SetPreferredTerminal`・`PreferredTerminal`・`Metadata.Terminal`/`CustomTerminal`・`session.ActionTerminalLaunch`・`launchable`・`diagnostics.LaunchTerminal`/`TerminalOptions`・`GET|PUT /api/v1/terminal/preference`・`POST /api/v1/terminal/launch`・`TerminalPreferenceSection.tsx`・`injection_darwin_test.go` を列挙。
- 08-13 (fffa6ac) 続けて `POST /api/v1/terminal/command`（貼り付け用コマンド配布）も削除。
→ 生存確認: 上記識別子はすべて grep 0 件、`api/openapi.yaml` のルート一覧にも残っていない。web 側の `terminalPreference` も 0 件。**残骸なし。** `ErrMetadataTerminal` だけが名前として生き残るが、`embeddedTerminal` 設定（maxSessions/scrollbackBytes/fontSize）の検証エラーに再利用されており死んでいない。**削除表を書いた計画は完全に終わっている。**

【系列5: Connections の情報構造（3回書き換え）】
- 08-05 (b3dc434) `ConnectionTree.tsx` 誕生。
- 08-11 22:28 (79d5fae) connection-browser-drilldown が削除し、ドリルダウン式ブラウザへ（「旧 URL 互換を保たない」と明記）。
- 08-12 23:08 (0685bda) quick-connect-group-browser が **25時間後に復元**し、ドリルダウンは Home のクイック接続（`QuickConnectBrowser.tsx`）へ移した。純粋な index/projection である `connectionBrowser.ts` は両方で再利用。
- 08-12 (63e7c7f) `retire connection group routes` が `connectionRoute.ts` を 87行削って `/connections/servers` に一本化。
→ 生存確認: `ConnectionBrowser.tsx` は存在しない。`connectionRoute.ts` は 117行に縮み legacy な `tab=` 語彙も無い。i18n キー 1094個を全走査して未参照 0 件。web/src の全コンポーネントに import 元がある。**フロントエンドの反転は最も後始末がよい。**

【系列6: Keychain の廃止と再登場】
- 08-05 key-vault が macOS Keychain 登録を実装。
- 08-09 (4632469) linux-support が「Keychain 経路を廃止する」。98a0b99 が手動試験 M3/M6 の Keychain 部分を撤去。b45c43e が `KeyAgent` を共有パッケージへ移動。
- 08-19（spec `2026-08-19-biometric-unlock-design.md`、コミット 9daa862/7dd40d9）が **Keychain を「鍵ではなく番人」として再導入**（`internal/platform/macos/biometric_darwin.{go,h,m}`、`secret.Guardian`、`/api/v1/passwords/biometric`）。
→ 「同じ OS 機能を、違う役割で入れ直した」ケース。廃止時のコミットが機能名で書かれているため反転に見えるが、実際は別の設計。この経緯は README にも roadmap にも書かれていない（計画文書も存在せず spec のみ）。

【系列7: Android の後付け（08-16）】
- `mobile/`（gomobile bind）と `android/`。Global Constraints が「`handoff.Owner` に新しい値を足さない」「`HealthResponse` に platform フィールドを足さない」「`wiring_linux.go` のビルドタグを触らない」と、既存構造を壊さないことを明示的に縛っている。
- 43本中で唯一、実行後に自分の計画へ「実施状況」表を書き足し、変えた判断3点（Kotlin→Java、`StartFailureKind(err)`→`LastStartFailureKind()`、`platform.Toolchain` は nil）を自己申告した文書。
→ 残骸なし。むしろこの計画は「`platform.Toolchain` は interface だった。空の struct ではなく nil が正解で、`keys.CatalogueReader` は既にそれを機能の不在として扱っていた」と、既存の nil 許容設計を発見して利用している。皮肉なことに、その同じ nil 許容が F3（Windows Toolchain 未配線）を静かなバグにしている。

■ structure に入り切らなかった5本の要約

- `2026-08-10-connection-creation-modal-implementation-plan.md`(179行) — alias だけの作成器を、使える接続を即作るモーダルへ。設定と vault を1トランザクションで commit。
- `2026-08-10-navigation-usability-implementation-plan.md`(119行) — 前提作業のための脱線を再開可能にし、ad hoc 診断と接続診断を区別、no-op な操作を除去。
- `2026-08-13-connection-key-passphrase-disclosure-implementation-plan.md`(77行) — 鍵パスフレーズ欄を保存状態に応じて初期開閉する native `<details>` に。
- `2026-08-13-i18n-language-catalogues-implementation-plan.md`(87行) — 英語を正本にカタログをファイル分割し、キー差分検査 `check:i18n` を追加。
- `2026-08-13-basic-actions-and-quick-connect-icon-implementation-plan.md`(61行) — 保存操作列の sticky 解除と `…` の SVG アイコン化。

■ 削除系コミット一覧（62件、日付順の主要なもの）

08-05: af8af5f 鍵の soft delete/purge、7f10a7a 設定ファイルの rename/delete と Include 行、1c02db9 validator が password vault を ssh_config として解析するのを停止
08-06: 8c862cd Config Explorer からのディレクトリ作成・削除、0cb95c9 空グループを異常として報告するのを停止
08-07: **3488374 Stop replacing this binary from the network（自己更新の削除）**
08-08: 57a45d1 リリース workflow の偽コメント削除
08-09: **4632469 Keychain 経路の廃止**、ddc2558 その設計、98a0b99 手動試験 M3/M6 の Keychain 部分、db673b4 二つ目のプロセス継ぎ目を OutputRunner に畳む、ff9351d 呼び出し元のないエクスポート2つ、1ecb072 設計書・README・手動試験表から消えた実装の記述、ef4c8f9 NewKeyAgent の説明から macOS、49c3a50 端末パス検査のプラットフォーム分割の廃止
08-12: **63e7c7f connection group routes の retire**、0670c0d obsolete browser copy
08-13: **2462d9b SSH_ASKPASS 一式**、48d2bdb askpass のあとに余った継ぎ目、**0d6bdb7 ssh-add と ssh -Q key**、**fffa6ac 端末へ貼るためのコマンド配布機能**、c165c78 画面から「近似」を外す、3bc2e42 外した自己更新のダウンロード関数、cdf006c Playwright の化石、11fdb61 objectstore タイムアウト試験の競合2つ、2ad52fa/db295ec 上記2件の設計文書
08-14: **f36e5eb ブラウザを完全に外す（platform.BrowserLauncher）**
08-15: **3747db0 ログイン項目を書く仕組み**、31d1968 その消し残し、**d539e59 engine サブコマンド・detach・KeepRunning**、2586df2 `/cli/engine`、9a83e4b 親が居なくなったエンジンの自己終了
08-16〜19: 主に fix 系（stop the … from …）。機能削除は 91f05f9（merge が戻したバンドル）程度で、18b61d6「keep what a pull removes, and ask before it removes anything」のように**削除しない**方向の変更が増える

■ 後続フェーズへの引き継ぎ（最重要）

grep で確認した結果、**きれいに畳まれた系列**（系列2 サービス／系列4 外部端末／系列5 Connections UI／系列3 engine サブコマンド）と、**残骸が生きている系列**（系列1 外部 ssh・askpass、設定解決器、Windows Toolchain）がはっきり分かれる。前者は計画文書に「Delete:」の明示的な一覧（embedded-terminal の D8、single-app の Task 7/8）を持ち、後者は「並走させてから寄せる」「残す理由を書く」という漸進的な方針を採った計画である。

**削除を一覧で書いた計画は完了し、条件付きで残すと書いた計画は残ったまま条件だけが消えた**——これがこの13日間から読み取れる最も再利用可能な教訓である。実際 `docs/superpowers/specs/2026-08-13-guards-after-askpass-design.md:57-71` は「条件を書かずに『まだ要る』とだけ言うと、要らなくなった日に誰も気づかない」と正しく予見しているが、その同じ文書が `platform.OutputRunner` を4つの理由付きで残し、その4つが全部消えた日に誰も気づかなかった（F1）。同様に config-resolver-authority の Task 6「呼び出し側を Resolve へ寄せる」は `ComputeEffective` だけで止まり、`Project` の5系統が残った（F2）。

したがって次フェーズの優先順位は、(1) F1・F3・F4・F5 の askpass/外部プロセス残骸の一括除去（互換性の懸念がなく、影響範囲が閉じている）、(2) F3 の Windows Toolchain 配線（唯一の実害があるバグ）、(3) F2・F13・F14 の解決器二重化の決着（設計判断が要る）、の順が妥当である。README と roadmap の陳腐化（F6/F8/F9/F10/F11/F19/F20）は、上の (1)(2) を直すついでに同じコミットで直せる。

**確度の高い指摘**

- [dead-code] **platform.OutputRunner の継ぎ目一式が、本番の呼び出し元ゼロのまま残っている**
  - 場所: `internal/platform/command.go:11,16,19,22,38,50, internal/platform/process/command.go:15-117`
  - 根拠: 確認済み。`rg -n '\.RunOutput\(' --type go` の8件はすべて internal/platform/process/command_test.go。`NewOutputRunner` は定義（command.go:26）と同パッケージのテスト4件のみ。docs/superpowers/specs/2026-08-13-retiring-the-toolchain-design.md:69 が「platform.OutputRunner は残る。ブラウザの起動、launchctl、systemctl、ssh-keygen が使う」と残す理由を4つ挙げたが、ブラウザは f36e5eb（08-14）で、launchctl/systemctl は 3747db0（08-15）で消え、ssh-keygen は実行されない（README.md:326）。4つとも消滅したのに継ぎ目だけ残った。internal/acceptance/programs_test.go:107-111 は両ファイルを名指しで検査対象から除外しており、コード側もこれが使われていないことを知っている
- [over-abstraction] **設定解決のエンジンが2本並走したまま統合が終わっていない**
  - 場所: `internal/effective/resolve.go:95, internal/effective/provenance.go:81,106, internal/application/effective.go:56-67`
  - 根拠: 確認済み。08-13 config-resolver-authority の Task 6「呼び出し側を Resolve へ寄せる」は application.ComputeEffective については完了（effective.go:58-61 のコメントが「以前はここに二つ目の走査があり…同じ問いに答えるものが二つあってはならない」と書く）。しかし effective.Project は本番から5系統が生きている: cmd/sshc/tui.go:51、internal/diagnostics/service.go:113,170,187、internal/application/passwordeligibility.go:82、internal/application/connectionupdate.go:406,442、internal/effective/jump.go:170,209。Projection.Value("hostname"/"user"/"port") は Resolve と同じ問いに答える。08-04 roadmap:87 の「Two engines answering one question will diverge」が、実装が入れ替わったあとも同じ形で残っている
- [dead-code] **実装済みの Windows Toolchain が配線されず、コメントも「これから入る」のまま**
  - 場所: `cmd/sshc/wiring_windows.go:13-23, internal/platform/windows/toolchain.go:31,36,43`
  - 根拠: 確認済み。internal/platform/windows.Toolchain と NewToolchain は実装済みでテスト（toolchain_test.go:15 に `var _ platform.Toolchain = windows.Toolchain{}`）もあるが、`rg -n 'windows.NewToolchain'` の5件はすべて toolchain_test.go。cmd/sshc/wiring_windows.go:23 は `platformParts{KeyAgent: ...}` だけを返し Toolchain を nil のままにし、同ファイル14-17行のコメントは「本物の Windows toolchain はその task が入れる」と未実装であるかのように書いている。結果として Windows では %WINDIR%\System32\OpenSSH\ssh-keygen.exe があってもハードウェア鍵の項目が出ない（keys/catalogue.go:77 が KeyGen() の可否で判定）
- [dead-code] **削除された SSH_ASKPASS 機構のための定数が internal/secret に残っている**
  - 場所: `internal/secret/service.go:26-28, internal/secret/service.go:75-80, internal/secret/service.go:92-96`
  - 根拠: 確認済み。`ErrUnknownToken`（service.go:28、コメントは「askpass トークンを報告する」）と `TokenTTL`（service.go:80、コメントは「ユーザーがボタンを押してから OpenSSH がパスワードのプロンプトに到達するまでの間隔」）はどちらも参照0件。`rg -n 'ErrUnknownToken' --type go` のヒットは別パッケージの effective.ErrUnknownToken とこの定義行だけ。Service 型の doc（service.go:92-96）も「外へ出るのはパスワードひとつだけであり、それも、このサービスが発行したトークンをひとつ持つ askpass リクエストひとつに対してである」と、2462d9b（08-13 21:25）で消えた経路を説明し続けている
- [dead-code] **存在しない環境変数を守るための資格情報ポリシーが生きて動いている**
  - 場所: `internal/application/passwordeligibility.go:125-129, :195, :296-301, :320-322`
  - 根拠: 確認済み。credentialEnvironmentUnsafe は passwordeligibility.go:195 から実際に呼ばれ、SendEnv パターンが SSHC_ASKPASS_TOKEN / _URL / _ALIAS / _KIND / _KEY_PATH に一致すると保存済みパスワードを拒否する。しかしこの5変数を設定する場所はリポジトリ全体に存在しない（`rg -niI 'askpass' --type go` の非テストヒットはこのファイルのコメントと名前リスト、そして既に死んだことを述べる他ファイルのコメントのみ）。08-13 retiring-the-external-ssh の Task 6 が「internal/platform/interactive.go の5つの変数を削除」と書いたその5変数であり、変数は消えたが守りだけが残った

---

## 調査方法

- 15 エージェント（サブシステム調査 9 → 横断分析 5 → 統合 1）。所要 61 分、ツール実行 1170 回。
- 調査 9 本: 設定ドメイン / HTTP 層とプロセス構成 / 保管庫と鍵 / 同期とオブジェクトストア / SSH とターミナル / プラットフォーム抽象 / CLI とエントリポイント / フロントエンド / 方針変遷の追跡。
- 横断 5 本: dead code / 過剰な抽象化 / 抽象化の不足と重複 / 層構造と依存方向 / 4 OS の分岐設計。各観点は調査結果を鵜呑みにせず自分で裏を取る指示を与えている。
- 各指摘には confidence（confirmed / likely / speculative）が付いている。本文に採ったのは主に confirmed。
- 調査後に Go 1.26.6 / Node 22.19.0 / npm 11.7.0 を `~/.local/toolchains/` へ導入し、ビルド・テスト・`deadcode`・`staticcheck` で主要な主張を検証した（§0.5）。
- **なお未検証**: 実機での挙動確認（macOS / Windows / Android 実機、S3 同期、PTY を伴う対話）。§0.5 に挙がっていない指摘は静的追跡のみの根拠である。
- ここに挙げた e2e は、その後 §0.11 で走らせた（95 通過 / 1 失敗、失敗は表示装置が無いためである）。**この節は監査を書いた時点の記録であり、後から書き換えていない**——何を確かめずに書いたかは、確かめたことと同じくらい読む価値がある。
