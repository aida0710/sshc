# Basic Actions and Quick Connect Icon Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基本設定の保存操作列を通常配置に戻し、クイック接続の文字省略記号をSVGアイコンへ置き換える。

**Architecture:** `ConnectionBasicForm` の操作列から位置固定クラスだけを除去する。既存のインラインSVGスプライトへ `moreHorizontal` を追加し、`ConnectionActions` が共通 `Icon` を使う。

**Tech Stack:** React 19、TypeScript、Tailwind CSS、Vitest、Testing Library、Playwright

## Global Constraints

- 新しい依存を追加しない。
- 保存・破棄・メニュー・接続の動作を変更しない。
- アイコンだけのボタンには既存の accessible name を維持する。

---

### Task 1: 追従解除と横三点アイコン

**Files:**
- Modify: `web/src/connections/ConnectionBasicForm.tsx`
- Modify: `web/e2e/connections.spec.ts`
- Modify: `web/src/overview/ConnectionActions.tsx`
- Modify: `web/src/overview/ConnectionActions.test.tsx`
- Modify: `web/src/ui/icons.tsx`
- Modify: `web/src/ui/icons.test.tsx`

**Interfaces:**
- Produces: `IconName` の `moreHorizontal`。
- Consumes: 既存の `Icon`、基本設定フォームの保存操作列、クイック接続の操作メニュー。

- [x] **Step 1: 失敗するテストを書く**

接続詳細を開いた直後は保存ボタンがviewport外にあり、フォーム末尾へスクロールすると表示されることを検証する。操作メニューボタン内の `use` が `#icon-moreHorizontal` を参照し、文字 `…` を持たないことも検証する。

- [x] **Step 2: REDを確認する**

Run: `npm test --prefix web -- --run src/overview/ConnectionActions.test.tsx src/ui/icons.test.tsx` と対象E2E。

Expected: 保存操作列がviewport内へ固定され、SVG参照もないため失敗する。

- [x] **Step 3: 最小実装を行う**

`ConnectionBasicForm` から `sticky bottom-0 z-10` を除去する。`iconNames` と `shapes` へ `moreHorizontal` を追加し、`ConnectionActions` の文字spanを `<Icon name="moreHorizontal" />` へ置き換える。

- [x] **Step 4: GREENを確認する**

Run: `npm test --prefix web -- --run src/overview/ConnectionActions.test.tsx src/ui/icons.test.tsx` と対象E2E。

Expected: PASS。

- [x] **Step 5: 全体検証する**

Run: `npm test --prefix web && npm run typecheck --prefix web && make e2e && git diff --check`

Expected: Vitest、型検査、build、Playwright、差分検査がすべて成功する。

- [x] **Step 6: コミットしてpushする**

Run: `git add docs/superpowers web/src internal/ui/dist && git commit -m '基本設定の保存操作を通常配置に戻す' && git push origin main`
