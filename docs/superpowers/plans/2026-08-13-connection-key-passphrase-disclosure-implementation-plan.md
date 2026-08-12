# Connection Key Passphrase Disclosure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 接続の鍵パスフレーズ編集欄を、保存状態に応じて初期開閉する折りたたみセクションにする。

**Architecture:** 既存の資格情報読込・保存処理は維持し、表示境界だけを native `<details>` に置き換える。選択鍵と保存状態が変わったときにReact stateを再初期化し、ユーザー操作は同じ対象の再描画をまたいで保持する。

**Tech Stack:** React 19、TypeScript、Testing Library、Vitest、Playwright

## Global Constraints

- 新しい依存を追加しない。
- 秘密値をDOMへ再表示・永続化しない。
- 保存APIと資格情報の意味を変更しない。

---

### Task 1: 保存状態に応じる折りたたみ

**Files:**
- Modify: `web/src/connections/ConnectionBasicForm.tsx`
- Modify: `web/src/connections/ConnectionBasicForm.test.tsx`
- Modify: `web/src/i18n/messages.ts`
- Modify: `web/e2e/connections.spec.ts`

**Interfaces:**
- Consumes: `dedicatedKeyPassphrase: boolean` と `namedKeyPassphrase: Credential | undefined`
- Produces: `conn.basicManageKeyPassphrase` を見出しに持つ native `<details>`

- [x] **Step 1: 失敗するコンポーネントテストを書く**

  未保存の鍵では `details.open === true`、共有・専用の保存済み鍵では `details.open === false` を直接検証し、閉じた見出しをクリックすると入力欄が操作可能になることを確認する。

- [x] **Step 2: REDを確認する**

  Run: `npm test --prefix web -- --run src/connections/ConnectionBasicForm.test.tsx`

  Expected: 新しい見出しまたは `<details>` が存在せず失敗する。

- [x] **Step 3: 最小実装を行う**

  `hasStoredKeyPassphrase = dedicatedKeyPassphrase || namedKeyPassphrase !== undefined` を導出し、既存ブロックを次の構造へ置き換える。

  ```tsx
  <details open={keyPassphraseOpen} onToggle={(event) => setKeyPassphraseOpen(event.currentTarget.open)}>
    <summary>{t("conn.basicManageKeyPassphrase")}</summary>
    <div>{/* 既存の状態説明・入力欄・エラー */}</div>
  </details>
  ```

- [x] **Step 4: GREENを確認する**

  Run: `npm test --prefix web -- --run src/connections/ConnectionBasicForm.test.tsx`

  Expected: PASS。

- [x] **Step 5: E2Eの利用手順を更新する**

  保存済み共有値から専用値へ切り替える箇所で、見出しを開いてから入力する。保存後に閉じることも確認する。

- [x] **Step 6: 全体検証を実行する**

  Run: `npm test --prefix web && npm run typecheck --prefix web && make e2e && git diff --check`

  Expected: Vitest、型検査、Playwright、差分検査がすべて成功する。

- [x] **Step 7: コミットしてpushする**

  ```bash
  git add docs/superpowers/specs/2026-08-13-connection-key-passphrase-disclosure-design.md \
    docs/superpowers/plans/2026-08-13-connection-key-passphrase-disclosure-implementation-plan.md \
    web/src/connections/ConnectionBasicForm.tsx \
    web/src/connections/ConnectionBasicForm.test.tsx web/src/i18n/messages.ts web/e2e/connections.spec.ts
  git commit -m '鍵パスフレーズ編集欄を折りたたむ'
  git push origin main
  ```
