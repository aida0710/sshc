# Web セクションのチャンク分割

## 目的

Vite の production build が出す 500 kB 超過警告を、警告上限の引き上げではなく初期 JavaScript の削減で解消する。

現在は `App.tsx` がすべてのセクション画面を静的 import しているため、利用者が開かない画面も 569.91 kB の単一初期チャンクへ入る。sshc は localhost から配信されるので転送待ちは小さいが、JavaScript の解析と初期実行はローカルでも発生する。本変更はその初期作業を減らし、画面遷移へ不要な演出は加えない。

## 採用する構成

React の `lazy` と `Suspense` を使い、セクション画面を動的 import する。

- シェル、ルーティング、テーマ、言語、セッション開始処理は初期チャンクへ残す。
- `LockScreen` はアプリケーションを開く前に必要なので初期チャンクへ残す。
- `OverviewPanel` は既定の Home 画面なので初期チャンクへ残す。
- Connections、Config、Groups、Keys、Known Hosts、Remote Keys、Diagnostics、Secrets、Settings、Sync、History をセクション単位で遅延読み込みする。
- Vite に共有依存の抽出とチャンク名を任せる。`manualChunks` や警告上限の変更は行わない。

`App.tsx` には各画面の loader を一度だけ宣言し、その loader を `lazy` へ渡す。named export は `default` へ変換する。画面の props と型は現在の契約を保ち、ルートや API 呼び出しの所有関係は変えない。

## 表示と操作

`SectionView` を `Suspense` で囲み、fallback は `null` にする。遅延チャンクを取得する短い間もヘッダー、主ナビゲーション、現在のセクション表示は残り、中央の画面だけが未描画になる。

専用のスピナー、スケルトン、読み込み文言は追加しない。localhost では待ち時間が短く、それらが一瞬だけ点滅する方が操作感を悪くするためである。hover／focus による事前ロードも追加しない。実際に使わない画面を読み込むと、今回の初期削減を別のタイミングで打ち消すためである。

同じセクションを再度開く場合、ブラウザーの module cache と `lazy` の解決済み Promise が使われる。画面の初回読み込み後に毎回待ち直すことはない。

## エラー境界

遅延チャンク専用の再試行 UI は追加しない。sshc の HTML とハッシュ付き asset は同じ Go バイナリへ埋め込まれ、外部デプロイ中に HTML とチャンクの世代がずれる構成ではない。チャンク取得に失敗する状況は、localhost の sshc 本体が停止するなどアプリケーション全体を利用できない状況と同じである。

既存の bootstrap、vault、API エラー表示は変更しない。

## テストと完了条件

- App の既存ルーティング、handoff、未保存変更ガードのテストを維持する。
- Home とロック画面が初期表示でき、遅延対象の各セクションが直接 URL、主ナビゲーション、Back／Forward から表示できることを確認する。
- `npm test --prefix web` と `npm run typecheck --prefix web` を通す。
- `npm run build --prefix web` を通し、500 kB 超過警告が消えることを確認する。
- 初期 JavaScript チャンクは 350 kB 未満を目標とする。各遅延チャンクも 500 kB 未満でなければならない。
- 全 Playwright を実バイナリで通し、直接 URL と各セクションの初回読み込みを検証する。
- API 生成物、Go normal／race、Docker-backed SeaweedFS／OpenSSH integration を回帰確認する。
- production build の `internal/ui/dist` を更新する。
- 依存パッケージと依存バージョンは変更しない。

## 非目標

- locale message の分割
- React／React DOM の vendor chunk を手動指定すること
- `chunkSizeWarningLimit` の引き上げ
- 画面内コンポーネントの細分化
- hover、focus、idle 時のチャンク事前取得
- スピナー、スケルトン、進捗表示の追加
- ルーティング、API、vault、秘密情報モデルの変更
