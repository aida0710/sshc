# i18n Language Catalogues Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 英語を正本として言語カタログをファイル分割し、非英語カタログの不足・余分なキーを明示的に検査できるようにする。

**Architecture:** 英語カタログから `MessageKey` を導出し、日本語をその完全な `Record` として型付けする。既存の `messages.ts` は公開APIを保つbarrelにし、純粋なキー比較関数をVitestから実行する。

**Tech Stack:** TypeScript 5.9、Vitest 4、React 19、Vite 8

## Global Constraints

- 翻訳文と既存キーを変更しない。
- 既存の `./i18n/messages` importを変更しない。
- 新しい依存を追加しない。
- 英語を唯一のキー正本にする。

---

### Task 1: カタログ差分検査

**Files:**
- Create: `web/src/i18n/catalogue.ts`
- Create: `web/src/i18n/catalogue.test.ts`
- Modify: `web/package.json`
- Modify: `README.md`

**Interfaces:**
- Produces: `catalogueDifference(master, candidate): { missing: string[]; extra: string[] }`。
- Consumes: `messages` に登録された英語以外のカタログ。

- [x] **Step 1: 失敗するテストを書く**

`{ alpha, beta }` と `{ beta, gamma }` を比較して `{ missing: ["alpha"], extra: ["gamma"] }` を返すこと、および登録済みの各カタログの差分一覧が空であることを表明する。

- [x] **Step 2: REDを確認する**

Run: `npm test --prefix web -- --run src/i18n/catalogue.test.ts`

Expected: `catalogue.ts` が存在しないため失敗する。

- [x] **Step 3: 最小実装と専用scriptを追加する**

両方の `Object.keys` を集合比較し、結果をlocaleごとにまとめるテストを実装する。`check:i18n` は `vitest run src/i18n/catalogue.test.ts --reporter=verbose` を実行し、READMEの開発コマンドに記載する。

- [x] **Step 4: GREENを確認する**

Run: `npm run check:i18n --prefix web`

Expected: 2件の検査が成功する。

### Task 2: 言語ファイル分割

**Files:**
- Create: `web/src/i18n/messages/en.ts`
- Create: `web/src/i18n/messages/ja.ts`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Produces: 従来と同じ `en`、`ja`、`messages`、`MessageKey` export。
- Consumes: `MessageKey = keyof typeof en` と `ja satisfies Record<MessageKey, string>`。

- [x] **Step 1: カタログを機械的に分割する**

英語オブジェクトを `en.ts`、日本語オブジェクトを `ja.ts` へ一文字も変えず移動する。`messages.ts` はimport、re-export、登録だけを行う。

- [x] **Step 2: 元カタログとの同一性を確認する**

Run: `git show HEAD:web/src/i18n/messages.ts` から抽出した各オブジェクトと、新ファイルの各オブジェクトを `diff` する。

Expected: 差分なし。

- [x] **Step 3: 型検査と専用検査を実行する**

Run: `npm run typecheck --prefix web && npm run check:i18n --prefix web`

Expected: 欠落・余分なキーなしで成功する。

- [x] **Step 4: 全体検証する**

Run: `npm test --prefix web && make e2e && git diff --check`

Expected: Vitest、production build、Playwright、差分検査がすべて成功する。

- [x] **Step 5: コミットしてpushする**

Run: `git add docs/superpowers web/src/i18n web/package.json internal/ui/dist && git commit -m 'i18nカタログを言語ごとに分割する' && git push origin main`
