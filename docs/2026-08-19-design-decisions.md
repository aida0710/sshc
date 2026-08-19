# 設計上の決定 — 2026-08-19

`docs/2026-08-19-design-audit.md` の §6 が「作者の判断が要る」として挙げた 9 件に答える。
決めたことと、**決めなかったこと**の両方を書く。

---

## 1. Android は linux build tag に相乗りさせ続ける

**決定: 現状維持。`android` build tag は導入しない。**

`GOOS=android` は `linux` タグを満たすので、`internal/platform/linux` も `cmd/sshc` も
Android 向けのコンパイルを通る。APK に CLI は入らないので実害は無い。

android タグを入れれば「Android では別のものが選ばれる」を型で言えるようになるが、
同時に `make test` の `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` が
見ているもの——**タグの食い違い**——の範囲が狭くなる。いま全パッケージが Android
向けに通ることを確かめられているのは、相乗りしているからである。

代わりに、不変条件はテストで言う。`mobile/dependencies_test.go` の
`TestEveryDependencyOfTheAndroidEngineIsADecision` が `app.Dependencies` の全項目に
「配線する／意図して空にする／既定に落とす」の分類を要求し、項目が増えれば赤くなる。
**Biometric が黙って落ちたのは、それが無かったからである。**

## 2. `internal/api` の生成モデルを通信の正本にする

**決定: (A)。生成型を契約の正本とし、手書きの双子は消していく。**

group の rename / delete と履歴の restore / recover は生成型を直に受ける形にした。
契約から外れれば、そこはコンパイルが止まる。

残りは、ドメインの値をそのまま `c.JSON` へ渡すエンドポイントである。詰め替えの関数を
32 型分置く価値は無いと判断した——代わりに
`internal/acceptance/contract_drift_test.go` が、生成型と実際に返している型の JSON
フィールド名を突き合わせる。`make verify-generated` が見るのは「生成物が仕様と一致
するか」だけで、**実際に返る JSON は誰も見ていなかった。**

`models.gen.go` を捨てる案 (B) は採らない。捨てれば `openapi.yaml` を変えたときに
Go が壊れるという唯一の契約保証を失う。

## 3. Electron 外殻は維持する

**決定: 維持。ただし外殻 2 つの共通化はしない。**

Electron（デスクトップ）と Android の WebView は、同じ役割——エンジンを起こし、
所有し、入口 URL を渡す——を別々に解いている。共通化の候補は 3 点あったが、いずれも
言語をまたぐ（JavaScript と Java）ので、共通化の実体は「仕様を文書で揃える」以上に
ならない。**実装が 2 つあること自体は、走る場所が 2 つある以上避けられない。**

到達不能になっていた `--hidden` 起動モードは削除した（Go 側の呼び出し元が消えていた）。

**残る不揃い（未着手）**: 文言のポリシーが 3 つある。Electron は日本語直書き、Android は
英語の `strings.xml` のみ、Web は en/ja の 1065 キー。揃えるなら Electron の文言を
`web/src/i18n` から生成することになるが、外殻が起きる時点で engine はまだ無く、
どの言語を選ぶかを誰も知らない——**先に「外殻は誰の言語で話すか」を決める必要がある。**

## 4. Windows のデスクトップ実体登録は NSIS が持つ

**決定: NSIS。Go 側の登録 API は公開しない。**

実際にレジストリへ書いているのは `desktop/build/installer.nsh` である。Go 側の
`RegisterDesktopExecutable` / `RemoveDesktopExecutable` は呼び出し元が一人も現れな
かったので削除した。**小文字版とその検査は残してある** ——方針を反転させるなら、
再公開は一行で済む。

Go 側が読む（`ReadDesktopExecutable`）のは維持する。書き手と読み手が別なのは
Linux の `desktop.json`（書き手 Electron / 読み手 Go）と同じ形であり、**3 OS で
3 つの記録方式になっているのはそれぞれの OS がそう決めているからである。**

## 5. `SSHC_ASKPASS_*` の防御は消す。規則の残りは名前と理由を書き換える

**決定: 消す。**

`SendEnv` が `SSHC_ASKPASS_*` を拾わないか見ていた照合は、その 5 変数を設定する場所が
リポジトリに存在しないので発火しない。`internal/secret` の `ErrUnknownToken` と
`TokenTTL` も参照 0 だった。

規則の残り（Match、条件付き Include、CanonicalizeHostname、実行を伴うディレクティブ）は
残した。ただし理由は置き換わっている——環境変数で capability を渡していたからではなく、
**この alias が使う鍵を実行も DNS も伴わずに一意に決められるか**である。名前も
`credentialEnvironmentUnsafe` から `credentialUnstaticConfiguration` へ改めた。

**未決**: `ProxyJump` がその一覧に残っているのは、いまは一貫していない。連鎖は
`internal/sshclient` がプロセス内で辿るようになり、アカウントパスワードの側は連鎖に
現れる alias のぶんを渡している。パスフレーズだけを断る理由は、外部プログラムが
消えた時点で無くなっている。**挙動を変える判断なので手を付けていない。**

## 6. `internal/application` は「設定のユースケース」と明記する

**決定: (a)。改名はしない。パッケージ doc で範囲を宣言する。**

保管庫と同期にまたがるユースケース——マスターパスワードの変更が local rekey と
remote reseal を 1 つの順序として扱うところ——は `internal/httpserver` の
`vaultOperations` にある。監査は「CLI から同じ順序を再現する保証が無い」と書いたが、
**実際には本番の配線が同じ 1 つを両方の transport へ渡している**（`server.go` で組んだ
`vault` が HTTP のハンドラにも CLI のルートにも入る）。`nil` はテスト用の既定である。

分担は意図的なので、揃えるのではなく `doc.go` に書いた。

## 7. `Run` と `Stream` の差は意図である

**決定: 揃えない。理由を doc に書く。**

`Run`（無人実行）は keepalive を張らず、設定の `SetEnv` を送らない。どちらも根拠が
ある——前者は上限時間の中で終わる短い操作なので相手の生存確認が要らず、後者は
**出力を読むから**である。定型のプログラムを走らせて標準出力を答えとして解析するので、
利用者の設定が `LANG` を変えれば読む相手の言葉が変わる。

`Stream`（`sshc run`）は出力を人が読むので、利用者の設定がそのまま効くのが正しい。

## 8. `ui/form` のクラス文字列は `ui/surface` の `Button` へ寄せる

**決定: 寄せた。**

実測すると、対象は 110 箇所ではなく **生の `<button>` 69 箇所**だった（残りは import 行）。
`<a>` にクラスを当てている 2 箇所はボタンではないので、そのまま残した。

寄せた理由は見た目の統一ではない。`surface.tsx` の `Button` が `type="button"` を
既定にする理由を「フォーム送信はどこにもなく」と書いていたが、**それは偽である**
——`<form onSubmit>` が 6 箇所ある。form の中の `<button>` は、書かなければ submit に
なり、押した瞬間にページがリロードされてセッションが失われる。**その間それが
起きずに済んでいたのは、書く人が毎回 `type="button"` を付けていたからである。**

## 9. テスト専用パッケージの命名

**決定: `internal/sshintegration` を `internal/sshdconformance` へ改めた。**

`integration/`（本物の sshc プロセスを起こす。外の何も要らないので `go test ./...` が
回す）と名前が近すぎ、Makefile が 4 行かけて別物だと説明していた。相手を名指しすれば
説明は要らない。

`internal/buildcontract` も割った。あそこは 2 つのものを抱えていた——ネイティブ
ビルドの CLI と機種を見る道具（本番のコード）と、Makefile・`.github`・`scripts` を
読んで取り決めを表明するメタテスト約 830 行である。**名前が合っているのは後者**
なので、道具を `internal/nativebuild` へ出した。`buildcontract` に残るのは検査だけで、
本番のコードはひとつも無い。

`internal/acceptance` へは寄せなかった。あちらが検査するのは**走っているアプリ**の
HTTP 面とハードニングで、こちらが検査するのは**リポジトリのビルドファイル**である。
主題が違う。
