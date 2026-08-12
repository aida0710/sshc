# Quick Connect Group Browser Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 接続画面を常時展開の管理ツリーへ戻し、サーバー／グループのドリルダウンをホームのクイック接続へ移す。

**Architecture:** `connectionBrowser.ts` の副作用を持たない index／projection はホームで再利用し、接続画面には管理操作を持つ `ConnectionTree` を復元する。接続 URL は `/connections/servers` に一本化し、クイック接続のブラウザー位置は `OverviewPanel` の一時 state に限定する。

**Tech Stack:** React 19、TypeScript、Tailwind CSS、Vitest、Testing Library、Playwright、Vite、Go embedded UI

## Global Constraints

- クイック接続の既定表示はサーバー一覧とする。
- ホームの閲覧だけでは Terminal、SSH、診断、`ssh -G` を開始しない。
- 接続画面のグループ／ファイル表示、パターンルール、ドラッグ移動を復元する。
- クイック接続にドラッグ移動を持ち込まない。
- 接続 identity URL は `/connections/servers` に一本化し、API、SSH config、vault、秘密情報モデルは変更しない。
- クイック接続の表示状態を URL、localStorage、metadata へ保存しない。
- 新しい依存パッケージを追加しない。

---

### Task 1: 接続管理ツリーを復元する

**Files:**
- Create: `web/src/connections/ConnectionTree.tsx`
- Create: `web/src/connections/ConnectionTree.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Consumes: `Overview`, `HostEntry`, `DragPayload`, `canDrop`, `Segmented`
- Produces: `ConnectionTree({ overview, selected, onSelect, onOpenPatternRule, onDrop, movesDisabled })`

- [ ] **Step 1: 失敗する管理ツリーの component test を復元する**

  接続をグループ階層とファイルで表示し、パターンルールをファイルへ渡し、保存中はドラッグを無効にする期待を `ConnectionTree.test.tsx` に置く。

- [ ] **Step 2: RED を確認する**

  Run: `npm test --prefix web -- ConnectionTree.test.tsx`

  Expected: `ConnectionTree` module が存在しないため FAIL。

- [ ] **Step 3: 変更前の管理ツリーを現在の型へ復元する**

  `ConnectionTree.tsx` にグループ／ファイル切替、検索、お気に入り、展開階層、パターンルール、ドラッグ移動を実装する。グループの正本には `overview.groups` を使い、表示属性は `overview.metadata.groups` から identity で重ねる。

- [ ] **Step 4: GREEN を確認する**

  Run: `npm test --prefix web -- ConnectionTree.test.tsx`

  Expected: 全件 PASS。

- [ ] **Step 5: Commit**

  ```bash
  git add web/src/connections/ConnectionTree.tsx web/src/connections/ConnectionTree.test.tsx web/src/i18n/messages.ts
  git commit -m "feat: restore connection management tree"
  ```

### Task 2: 接続ページを固定の管理ツリーへ戻す

**Files:**
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`
- Modify: `web/src/App.tsx`

**Interfaces:**
- Consumes: Task 1 の `ConnectionTree`
- Produces: 接続 URL の browser state と無関係に、常時展開ツリーを描く `ConnectionsPage`

- [ ] **Step 1: 失敗するページテストを書く**

  `/connections/servers` で「サーバー／グループ」切替が存在せず、`Arrange connections by` の「Groups／Files」が存在すること、パターンルールが `onOpenFile(path, line)` を呼ぶことを期待する。

- [ ] **Step 2: RED を確認する**

  Run: `npm test --prefix web -- ConnectionsPage.test.tsx`

  Expected: 現在の `ConnectionBrowser` が「Servers／Groups」を表示するため FAIL。

- [ ] **Step 3: ConnectionsPage を管理ツリーへ接続する**

  `ConnectionBrowser` を `ConnectionTree` に置換し、`onOpenFile` を props と App から再接続する。URL parser／formatter と選択 detail は維持するが、左ペインの `browser` location は描画へ渡さない。

- [ ] **Step 4: obsolete なブラウザー移動テストを管理ツリー契約へ置き換える**

  同一 identity の draft guard は `/connections/servers` の panel 変更で維持し、グループブラウザー URL をクリックするテストは削除する。

- [ ] **Step 5: GREEN を確認する**

  Run: `npm test --prefix web -- ConnectionsPage.test.tsx App.test.tsx`

  Expected: 全件 PASS。

- [ ] **Step 6: Commit**

  ```bash
  git add web/src/connections/ConnectionsPage.tsx web/src/connections/ConnectionsPage.test.tsx web/src/App.tsx
  git commit -m "feat: restore connection management navigation"
  ```

### Task 3: クイック接続へ閲覧ブラウザーを移す

**Files:**
- Create: `web/src/overview/QuickConnectBrowser.tsx`
- Create: `web/src/overview/QuickConnectBrowser.test.tsx`
- Modify: `web/src/overview/OverviewPanel.tsx`
- Modify: `web/src/overview/OverviewPanel.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Consumes: `buildConnectionBrowserIndex(overview)`, `projectConnectionBrowser(index, browser, query, favouritesOnly)`
- Produces: `QuickConnectBrowser({ overview, launching, onConnect, onOpenSettings })`

- [ ] **Step 1: 失敗するクイック接続テストを書く**

  次の利用者向け動作を literal な期待で固定する。

  - 初期状態は `Servers` が選択され、全カードを表示する。
  - `Groups` へ切り替えるとトップレベルだけを表示する。
  - グループを選ぶと子グループと直下サーバーだけを表示する。
  - パンくずで戻れる。
  - 閲覧だけでは `onConnect` を呼ばない。
  - サーバーカードの `…` から設定と接続を別々に実行する。

- [ ] **Step 2: RED を確認する**

  Run: `npm test --prefix web -- QuickConnectBrowser.test.tsx OverviewPanel.test.tsx`

  Expected: `QuickConnectBrowser` が存在せず、現行 OverviewPanel に切替がないため FAIL。

- [ ] **Step 3: QuickConnectBrowser を実装する**

  browser state を `{ view: "servers" }` で初期化し、`connectionBrowser.ts` の projection をカード表示へ変換する。ドラッグ属性と drop handler は実装しない。検索、お気に入り、パンくず、missing／empty state を描く。

- [ ] **Step 4: OverviewPanel の既存カードループを置換する**

  overview 読み込み、集計、同期、エラー、Terminal 起動は `OverviewPanel` に残す。クイック接続の表示だけを `QuickConnectBrowser` へ渡す。

- [ ] **Step 5: GREEN を確認する**

  Run: `npm test --prefix web -- QuickConnectBrowser.test.tsx OverviewPanel.test.tsx ConnectionActions.test.tsx`

  Expected: 全件 PASS。

- [ ] **Step 6: Commit**

  ```bash
  git add web/src/overview/QuickConnectBrowser.tsx web/src/overview/QuickConnectBrowser.test.tsx web/src/overview/OverviewPanel.tsx web/src/overview/OverviewPanel.test.tsx web/src/i18n/messages.ts
  git commit -m "feat: browse groups from quick connect"
  ```

### Task 4: E2E 契約を新しい画面責務へ移す

**Files:**
- Modify: `web/e2e/home.spec.ts`
- Modify: `web/e2e/connections.spec.ts`
- Modify: `web/e2e/groups.spec.ts`
- Modify: `web/e2e/routing.spec.ts`

**Interfaces:**
- Consumes: Task 2、3 の画面動作
- Produces: 実バイナリを通る新導線の回帰契約

- [ ] **Step 1: 失敗するホーム E2E を書く**

  Home の `Quick connect` 内で `Groups` → `work` と辿り、カードのメニューを開くまで Terminal launch endpoint が呼ばれないことを確認する。

- [ ] **Step 2: RED を確認する**

  Run: `npm run e2e --prefix web -- home.spec.ts`

  Expected: Home にグループ切替が無いため FAIL。

- [ ] **Step 3: 接続画面のドリルダウン期待を展開ツリーへ置き換える**

  `/connections/groups/...` をクリックで生成するテストを削り、接続画面でグループ階層が同時表示されることと、ドラッグ移動後に URL identity が更新されることを維持する。

- [ ] **Step 4: 対象 E2E を通す**

  Run: `npm run e2e --prefix web -- home.spec.ts connections.spec.ts groups.spec.ts routing.spec.ts`

  Expected: 対象全件 PASS。

- [ ] **Step 5: Commit**

  ```bash
  git add web/e2e/home.spec.ts web/e2e/connections.spec.ts web/e2e/groups.spec.ts web/e2e/routing.spec.ts
  git commit -m "test: cover quick connect group browsing"
  ```

### Task 5: 全回帰検証と出荷

**Files:**
- Modify generated UI only through `make build`

**Interfaces:**
- Consumes: Tasks 1–4 の最終 tree
- Produces: 検証済みの `main` commit と `origin/main`

- [ ] **Step 1: 生成物と静的検査を確認する**

  Run: `make verify-generated && git diff --check && go vet ./...`

- [ ] **Step 2: 全テストを実行する**

  Run: `make test`

- [ ] **Step 3: production build と全 E2E を実行する**

  Run: `make e2e`

  Expected: Vite chunk warning なし、Playwright failure なし。

- [ ] **Step 4: Docker 統合を実行して片付ける**

  Run: `make integration-up && make integration; result=$?; make integration-down; exit $result`

- [ ] **Step 5: 差分を自己レビューする**

  `git diff origin/main...HEAD` を読み、クイック接続に設定変更、秘密、不要な URL state が入っていないことを確認する。

- [ ] **Step 6: push して一致を確認する**

  ```bash
  git push origin main
  git fetch origin main
  test "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)"
  test -z "$(git status --porcelain)"
  ```
