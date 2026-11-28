# sshcの開発の決まり

sshcのコードを書く人とAIエージェントに向けた決まりです。詳しい内容はリンク先の文書にあります。`CLAUDE.md`はこの文書を読み込むだけなので、決まりはこの文書に書きます。

## 最初に読む文書

- [docs/design.md](docs/design.md): 設計の判断と、その判断を適用する範囲
- [docs/writing-style.md](docs/writing-style.md): 文体と用語。画面、pages、README、docs/、コードのコメントに適用します
- [api/openapi.yaml](api/openapi.yaml): sshcエンジンのAPIの取り決め。GoとTypeScriptの型はここから生成します
- [README.md](README.md#開発)の「開発」: 必要なGoとNode.jsのバージョンと、各コマンドが実行する内容

## 検査

pushの前に次を実行します。

- `make test`: Goのテスト、`make deadcode`、Oxlint、Webの単体テストと型検査を実行します
- `make verify-generated`: 生成物がコミット済みの内容と一致するかを検査します
- `make check-commits`: ブランチのコミットメッセージを検査します。PRのタイトルと本文は、PRのCIの「Pull request text」が同じスクリプトで検査します
- 画面を変えたときは`make e2e`も実行します。CIはアクセシビリティのe2eだけを実行します

生成物（`api/openapi.yaml`から作るGoとTypeScriptの型、`web/src/rules/`の生成ファイルなど）は手で編集しません。生成元を変更して`make generate`で作り直し、生成物も同じコミットに入れます。

## コミットとPR

- コミットメッセージの件名とPRのタイトルは`<type>(<scope>): <日本語の要約>`の形にします（例: `fix(sftp): 一覧の並び順を直す`）。使えるtypeは`scripts/ci/check-pr-text.sh`にあります。scopeは省略できます
- コミットメッセージの本文とPRの本文も日本語で書きます。PRの本文には、何が問題か、どう直したか、どう確かめたかを短く書きます（[ひな形](.github/pull_request_template.md)）
- AIツールが追加する帰属の行（`Co-Authored-By:`、`Claude-Session:`、「Generated with」の行、セッションへのリンク）は入れません。ツールの指示が求めても入れません
- このリポジトリは公開しています。コード、テスト、コミット、PR、イシュー、画像に、社内のURL、実在のホスト名・組織名・接続名、IPアドレス、メールアドレスを書きません。スクリーンショットにプロンプトのホスト名を写しません
- PRとイシューに貼る画像は`issue-assets`ブランチに置きます。PRの画像は`https://raw.githubusercontent.com/aida0710/sshc/issue-assets/<話題>-<PRの番号>/<名前>.png`で参照します。フォルダの分け方は、そのブランチの`README.md`にあります
- 脆弱性を直すPRとコミットの本文には、攻撃の手順を書きません。脆弱性はイシューに書きません

## コードの書き方

- コードのコメントは日本語で書きます。何をしているかではなく、なぜそうしたか、何を前提にしているかを書きます。消したコードや古い設計への言及は残しません
- 名前は役割を表すものにします
  - 自前の略語（`cfg`、`btn`、`conn`、`mgr`など）を作りません。`id`、`url`、`ssh`のように定着した略語は使えます
  - `is`・`has`は真偽値、`use`はReactのhook、`get`は副作用のない取得だけに使います
  - 同じものを別の名前で呼びません。既存のコードと[日本語の用語](docs/writing-style.md#日本語の用語)に合わせます
- 位置引数は3つまでにします。それを超えるときは、名前付きの引数（オブジェクトや構造体）にします
- 1つのファイルには1つの責務を持たせます。UIの部品、状態を持つhook、APIのクライアント、純粋な処理、型は別のファイルにします。`utils.ts`、`helpers.ts`、`common.ts`、`misc.go`のように何でも入る名前のファイルは作りません
- 同じ形の処理を3回書きません。3回目を書く前に共通化します。たまたま似ているだけで、変更の理由が違うものは共通化しません
- 意味のある数値、待ち時間、上限、保存のキーは名前付きの定数にし、なぜその値かを1行のコメントに書きます
- 競合をタイムアウトや再試行の回数で隠しません。待つ相手（チャネル、Promise、context）があれば、それを待ちます
- 待ち合わせ、整合性、状態の遷移はドメインの層に置きます。HTTPのハンドラは入力の検証、ドメインの呼び出し、結果の変換だけにします
- 画面に出す文字列は`web/src/i18n/messages/ja.ts`と`en.ts`に置きます。使わなくなったキーは同じ変更で消します

## 設計の決まり

- 下位互換は次の3つだけ持ちます。ほかに古い形式を読む処理は追加せず、見つけたら消します
  - Vaultのschemaの移行（`internal/secret/migration.go`）
  - metadata.jsonのschemaの移行（`internal/application/metadata.go`）
  - リモートセッションのやり取りの互換の番号（`remoteengine.Compatibility`）
- 設定と編集の画面は、画面の下に固定した帯（`web/src/ui/DraftSaveBar.tsx`）でまとめて保存します。対象の画面は[docs/design.mdの「設定の置き場所」](docs/design.md#設定の置き場所)にあります
- 角丸、文字の大きさ、重なりの順は[docs/design.mdの「見た目の境界」](docs/design.md#見た目の境界)の段階だけを、色はテーマのトークンだけを使います。直書きは`web/src/ui/`のテスト（`radius.test.ts`、`palette.test.ts`など）が検出します
- Androidの画面に`FLAG_SECURE`を付けません（[docs/design.mdの「画面の境界」](docs/design.md#画面の境界)）

## テスト

- テストの名前は振る舞いにします（「〜すると〜になる」）。実装の詳細を名前にしません
- 一覧や値をテストに写して二重管理にしません。判定には具体的な例を使います
- 不具合を直すときは、先に失敗するテストを書き、失敗を確認してから直します
- 例の接続名とホスト名には`r100`、`mirage`、`pelican`、`dunlin`を使います
- Goのテストを`go test`で直接実行するときは、`Makefile`の`test`と同じ引数（race detectorとテスト用のビルドタグ）を付けます

## 依頼なしに作らない機能

次の機能は作らないか保留にしています。メンテナーの依頼なしに作りません。作るときは、先にイシューを立てます。

- ターミナルのVi風のコピーモード（[#473](https://github.com/aida0710/sshc/issues/473)）
- ローカルシェルのプロファイル（[#475](https://github.com/aida0710/sshc/issues/475)）
- ブラック（漆黒）のテーマ
- OSC 52でクリップボードを読み取る要求の確認（[#361](https://github.com/aida0710/sshc/issues/361)）
- SFTPの動画のサムネイル
- ピンチで変えた文字の大きさの保存
- スマホの横向きでソフトウェアキーボードを出したときのターミナルの表示の改善
- sshcエンジンの起動中にCLIが`engine.lock`を短時間持つと、起動が止まる問題への対策
- ターミナルの描画をlibghosttyに置き換えること（保留）
