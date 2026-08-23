# 2026-08-19 の設計判断

`docs/2026-08-19-design-audit.md` §6 に挙げた 9 件について、当時の判断と保留事項を記録します。

---

## 1. Android では linux build tag を継続して使用する

決定: 現状を維持し、`android` build tag は導入しません。

`GOOS=android` は `linux` タグを満たすので、`internal/platform/linux` も `cmd/sshc` も
Android 向けにコンパイルできます。APK に CLI は含まれないため、製品動作への影響はありません。

android タグを導入すると Android 固有の実装をビルド時に区別できますが、
同時に `make test` の `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` が
検出できる build tag の不整合範囲が狭くなります。現在は linux タグを共有することで、
全パッケージが Android 向けにビルドできることを確認しています。

代わりに、不変条件はテストで検査します。`mobile/dependencies_test.go` の
`TestEveryDependencyOfTheAndroidEngineIsADecision` が `app.Dependencies` の全項目に
「設定する／意図的に未設定にする／既定値を使う」の分類を要求し、項目追加時に未分類であれば失敗します。
この検査を追加する前は、Biometric の未設定を検出できませんでした。

## 2. `internal/api` の生成モデルを API 契約の基準にする

当初の決定: (A)。生成型を API 契約の基準とし、重複する手書き型を削減します。

group の rename / delete と履歴の restore / recover は生成型を直接受け取ります。
契約と不一致になった場合はコンパイルエラーになります。

残りは、ドメイン値をそのまま `c.JSON` に渡すエンドポイントです。32 型分の変換関数は追加せず、
`internal/acceptance/contract_drift_test.go` が、生成型と実際に返している型の JSON
フィールド名を照合します。`make verify-generated` が検査するのは「生成物が仕様と一致
するか」だけであり、実際の応答 JSON との一致は検査していませんでした。

`models.gen.go` を削除する案 (B) は採用しません。`openapi.yaml` を変更したときに
Go コードとの不一致をコンパイル時に検出する仕組みを失うためです。

### 追記: 実測後の変更

上記は当初の判断として残します。実装前の調査により、(A) と (B) のどちらも適切でないことが分かりました。

同名 36 対のうち 27 対は Go の型が異なり、その差には一定の規則がありました。生成側は OpenAPI の
省略可能を `*T` で表し、`application` 側は値と `omitempty` で表します。さらに
`application` は `DiffOp` や `EditAction` のような名前付きの型を持ち、生成側はそれを
`string` にします。(A) を適用するとドメイン型がポインタと `string` に置き換わり、
ドメイン側の型制約が弱くなります。

最終的に、型を統合せず、不要な型を生成対象から除外しました。`oapi-codegen` の
`exclude-schemas` に 20 スキーマを指定して重複定義を削除しました。組は 36 から 18 に減り、
`models.gen.go` は 1524 行から 1339 行になりました。残る 18 組は、使用中の
request/response に入れ子で含まれるため生成が必要です。

API 契約の検査は維持しています。生成物との比較は `openapi.yaml` 自体との比較に変更しました。
`contract_drift_test.go` が `yaml.v3` で仕様を読み、`application` の型と比較します。加えて `httpserver` 側の
「返る本文に知らない項目が無い」検査を、生成型ではなく実際に `c.JSON` へ渡している型へ
向けました。2 つの検査により「応答本文 ⊆ application の型 = API 契約」を確認します。

## 3. Electron ラッパーを維持する

当時の決定: 維持します。ただし Electron と Android の実装は共通化しません。

Electron（デスクトップ）と Android WebView は、エンジンの起動と所有、アクセス URL の受け渡しを
それぞれ実装していました。共通化候補はいずれも JavaScript と Java をまたぐため、コード共有ではなく
仕様の共通化にとどまります。実行環境が異なるため、実装は 2 つ必要です。

到達不能になっていた `--hidden` 起動モードは、Go 側の呼び出し元が削除済みだったため削除しました。

当時の保留事項: 文言の管理方法が 3 種類あります。Electron は日本語直書き、Android は
英語の `strings.xml` のみ、Web は en/ja の 1065 キー。揃えるなら Electron の文言を
`web/src/i18n` から生成する案がありますが、デスクトップラッパーの起動時点では engine がまだなく、
どの言語を選択するか決められません。共通化の前に、ラッパーの言語選択規則を決める必要があります。

## 4. Windows のデスクトップ実行ファイル登録は NSIS が担当する

決定: NSIS が担当し、Go 側の登録 API は公開しません。

実際にレジストリへ書き込むのは `desktop/build/installer.nsh` です。Go 側の
`RegisterDesktopExecutable` / `RemoveDesktopExecutable` は呼び出し元が一人も現れな
かったため削除しました。非公開版とその検査は残してあり、方針変更時には 1 行で再公開できます。

Go 側の読み取り処理（`ReadDesktopExecutable`）は維持します。書き込みと読み取りを別の実装が担当する点は、
Linux の `desktop.json`（書き込みは Electron、読み取りは Go）と同じ構成です。OS ごとの標準方式に従うため、3 OS で記録方式が異なります。

## 5. `SSHC_ASKPASS_*` の防御は消す。規則の残りは名前と理由を書き換える

決定: 削除します。

`SendEnv` が `SSHC_ASKPASS_*` を拾わないか見ていた照合は、その 5 変数を設定する場所が
リポジトリに存在しないため機能していませんでした。`internal/secret` の `ErrUnknownToken` と
`TokenTTL` も参照数は 0 でした。

規則の残り（Match、条件付き Include、CanonicalizeHostname、実行を伴うディレクティブ）は
残しました。ただし目的は、環境変数による capability の受け渡しではなく、
対象 alias が使用する鍵を外部コマンド実行や DNS なしで一意に決定できるかの確認です。名前も
`credentialEnvironmentUnsafe` から `credentialUnstaticConfiguration` へ変更しました。

当時の保留事項: `ProxyJump` がその一覧に残っている点は一貫していません。接続チェーンは
`internal/sshclient` がプロセス内で辿るようになり、アカウントパスワードの側は連鎖に
含まれる alias の資格情報を渡しています。パスフレーズだけを拒否する理由は、外部プログラムが
消えた時点でなくなっています。動作変更を伴うため、この時点では変更していません。

## 6. `internal/application` は「設定のユースケース」と明記する

決定: (a)。改名せず、package documentation に対象範囲を記載します。

保管庫と同期にまたがるユースケース、すなわちマスターパスワード変更時に local rekey と
remote reseal を一連の処理として扱う箇所は、`internal/httpserver` の
`vaultOperations` にあります。監査では「CLI から同じ順序を再現する保証がない」と記載しましたが、
実装では同じ `vault` を両方の transport に渡しています（`server.go` で構築した
`vault` を HTTP handler と CLI route の両方に渡します）。`nil` はテスト用の既定値です。

この分担は意図した設計であるため、統合せず `doc.go` に記載しました。

## 7. `Run` と `Stream` の差は意図である

決定: 統一せず、理由を package documentation に記載します。

`Run`（無人実行）は keepalive を設定せず、設定の `SetEnv` も送信しません。短時間かつ上限時間内に
終了する処理では keepalive が不要です。また、定型プログラムの標準出力を機械的に解析するため、
利用者の設定が `LANG` を変えれば読む相手の言葉が変わる。

`Stream`（`sshc run`）は利用者が出力を読むため、利用者の環境設定を適用します。

## 8. `ui/form` のクラス文字列を `ui/surface` の `Button` に統一する

決定: `ui/surface` の `Button` に統一しました。

調査対象は 110 箇所ではなく、生の `<button>` 69 箇所でした（残りは import 行）。
`<a>` にクラスを指定する 2 箇所はボタンではないため、そのまま残しました。

統一の目的は見た目ではなく、`type="button"` の既定値を一元化することです。
`<form onSubmit>` は 6 箇所あり、form 内の `<button>` は type を省略すると submit になります。
それまでは各箇所で `type="button"` を指定してページ再読み込みとセッション消失を防いでいました。

## 9. テスト専用パッケージの命名

決定: `internal/sshintegration` を `internal/sshdconformance` に改名しました。

`integration/`（実際の sshc プロセスを起動し、外部依存なしで `go test ./...` が実行する）と
名前が近く、Makefile に 4 行の区別説明が必要でした。`sshdconformance` は検査対象を明示します。

`internal/buildcontract` も分割しました。ここには、ネイティブビルド用 CLI とアーキテクチャ検査を行う製品コード、
および Makefile・`.github`・`scripts` の契約を検査する約 830 行のメタテストが含まれていました。
前者を `internal/nativebuild` に移し、`buildcontract` にはテストだけを残しました。

`internal/acceptance` には統合していません。`acceptance` は実行中アプリケーションの HTTP API と
セキュリティ設定を検査し、`buildcontract` はリポジトリのビルドファイルを検査するためです。
