# レスポンシブ UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 幅 360px の画面で、この UI のどの面も横スクロールせずに使えるようにする。

**Architecture:** コンポーネントツリーは 1 本のまま、Tailwind の breakpoint だけで配置を切り替える。JS の媒体クエリは 1 つも作らない——一覧と詳細の出し分けは幅ではなく「何かが選択されているか」で決まり、それは既にルートが持っている。

**Tech Stack:** React 19.2.8 / Tailwind CSS 4.3.3（既定の breakpoint）/ Playwright 1.62.1 / xterm.js 6.0.0

**Spec:** `docs/superpowers/specs/2026-08-16-android-responsive-ui-design.md`

**Worktree:** `.worktrees/android-responsive-ui`（ブランチ `android-responsive-ui`）

この計画がサブプロジェクト 1 より短いのは、作業が機械的だからである。**新しい判断はほとんど無く、既存のクラス文字列に breakpoint を足す。** 判断が要るのはキーバー（Task 5）だけで、そこだけコンポーネントを新設する。

## Global Constraints

- **Tailwind の設定ファイルに手を入れない。** 既定の `md` (768) と `lg` (1024) をそのまま使う。
- **`useIsCompact()` のようなフックを作らない。** `matchMedia` も `ResizeObserver` も使わない。
- **`HealthResponse` に `platform` や capability のフィールドを足さない。**
- **クラス名の文字列を表明するテストを書かない。** リファクタで即座に壊れる割に何も守らない。守るのは挙動であり、それを見られるのは Playwright だけである。
- `aria-label` は消さない。見える文字を狭い画面で落とすことと、アクセシブルネームを落とすことは別である。
- `ConfigExplorer.tsx:257` は既に `lg:` で分岐済み。**触らない。**
- コメントは日本語。既存ファイルの文体に合わせる。
- 各タスクの最後に `npm run typecheck --prefix web` と `npm test --prefix web` が通ること。

## ファイル構成

| ファイル | 変更 |
|---|---|
| `web/index.html` | viewport meta に `interactive-widget=resizes-content` |
| `web/src/App.tsx` | シェルのグリッド、ヘッダーの畳み方、ナビのドロワー化、リンクのタッチ標的 |
| `web/src/ui/Inspector.tsx` | `InspectorPane` を `< lg` でシートに |
| `web/src/connections/ConnectionsPage.tsx` | 一覧と詳細を `< md` で置き換えに |
| `web/src/terminal/KeyBar.tsx` | 新設。Esc / Tab / Ctrl / Alt / 矢印 / 記号 |
| `web/src/terminal/KeyBar.test.tsx` | 新設。sticky 修飾のロジック |
| `web/src/terminal/TerminalView.tsx` | `< md` でキーバーを出す |
| `web/src/i18n/messages/{en,ja}.ts` | 端末を名指ししない文へ |
| `web/playwright.config.ts` | 360×800 の project |
| `web/e2e/narrow.spec.ts` | 新設。狭い幅で守る 4 つ |

---

### Task 1: シェルを 1 ペインから積み上げる

**Files:**
- Modify: `web/index.html`
- Modify: `web/src/App.tsx:462-533`（ヘッダーとグリッド）、`:542-628`（ナビ）、`:330-344`（リンク）

**Interfaces:**
- Consumes: なし
- Produces: `navigationOpen` / `setNavigationOpen`（App のローカル state）。Task 2 以降は触らない。

- [ ] **Step 1: viewport meta を直す**

`web/index.html`:

```html
    <meta name="viewport" content="width=device-width, initial-scale=1.0, interactive-widget=resizes-content" />
```

これでキーボードが出たときにレイアウトビューポートが縮む。既に入っている
`@xterm/addon-fit` がそのまま追従するので、Task 5 で仮想キーボードのために
書くコードは無くなる。

- [ ] **Step 2: グリッドを 3 段にする**

`App.tsx:531-534` の `className` を差し替える。

```tsx
        className={`grid min-h-0 flex-1 grid-rows-[minmax(0,1fr)] grid-cols-1 md:grid-cols-[15rem_minmax(0,1fr)] ${
          // minmax(0,…) on the middle track for the same reason min-h-0 is on
          // the row: a bare 1fr is minmax(auto,1fr), so the column refuses to
          // shrink below its content and the panel runs out under the
          // inspector instead of narrowing to make room for it.
          //
          // grid-cols-1 が既定である。**狭い画面では 1 ペインしか無い** ——
          // ナビはドロワーとしてこの格子の外に出て、inspector はシートになる。
          // 列が増えるのは幅が増えたときだけである。
          inspector !== null && inspectorOpen
            ? "lg:grid-cols-[15rem_minmax(0,1fr)_17rem]"
            : ""
        }`}
```

- [ ] **Step 3: ナビをドロワーにする**

`navigationOpen` の state を App に足す。

```tsx
  const [navigationOpen, setNavigationOpen] = useState(false);
```

`<nav>` の className を差し替える。

```tsx
          className={`relative z-30 flex min-h-0 flex-col overflow-hidden border-r border-line bg-sidebar p-2
            fixed inset-y-0 left-0 w-72 transition-transform
            md:static md:z-auto md:w-auto md:translate-x-0
            ${navigationOpen ? "translate-x-0" : "-translate-x-full"}`}
```

`<nav>` の直前にオーバーレイを置く。**`md:hidden` である** ——広い画面には
閉じるべきものが無い。

```tsx
        {navigationOpen ? (
          <div
            aria-hidden="true"
            onClick={() => setNavigationOpen(false)}
            className="fixed inset-0 z-20 bg-canvas/70 md:hidden"
          />
        ) : null}
```

`navigationLink` の `onClick` に `setNavigationOpen(false)` を足す。**遷移したら
閉じる。** 開いたままだと、選んだ先が自分の後ろに隠れる。

Esc で閉じるのは `<nav>` の `onKeyDown` ではなく、シェル全体の `useEffect` で
`document` に付ける——ドロワーの中にフォーカスが無くても閉じられるようにする。

- [ ] **Step 4: ハンバーガーをヘッダーに足し、狭い画面で文字を落とす**

`<header>` の先頭に置く。`md:hidden`。

```tsx
        <button
          type="button"
          aria-label={t("shell.primaryNavigation")}
          aria-expanded={navigationOpen}
          onClick={() => setNavigationOpen((open) => !open)}
          className="shrink-0 rounded-md border border-control-line bg-card p-2 md:hidden"
        >
          <Icon name="menu" className="h-4 w-4" />
        </button>
```

`web/src/ui/icons.tsx` に `menu` が無ければ足す（3 本線の path 1 つ）。

同じ header の中で落とすもの:

| 要素 | 追加するクラス |
|---|---|
| `<h1>{t("shell.title")}</h1>` | `hidden md:block` |
| 区切りの `<span>/</span>` | `hidden md:inline` |
| 稼働状態の `<p role="status">` | `hidden sm:flex` |
| 外観の `<label>` | `hidden md:inline` |
| 言語の `<label>` | `hidden md:inline` |

`px-6` を `px-3 md:px-6` にする。360px でヘッダーに 48px の余白は高い。

- [ ] **Step 5: リンクのタッチ標的を広げる**

`navigationLink` の `px-2 py-1.5` を `px-3 py-2.5` にする。**`md:` を付けない** ——
24px のクリック標的はデスクトップでも狭く、条件付きにする理由がない。

- [ ] **Step 6: 検査する**

```sh
npm run typecheck --prefix web && npm test --prefix web
```

Expected: PASS。既存の vitest はレイアウトを見ていないので、ここで落ちるものは
本当の壊れである。

- [ ] **Step 7: Commit**

```sh
git add web/index.html web/src/App.tsx web/src/ui/icons.tsx
git commit -m "feat: let the shell stand up in one column"
```

---

### Task 2: Inspector を狭い画面でシートにする

**Files:**
- Modify: `web/src/ui/Inspector.tsx:59-70`

**Interfaces:**
- Consumes: なし
- Produces: なし（`InspectorPane` の見た目だけが変わる）

- [ ] **Step 1: `InspectorPane` の className を差し替える**

```tsx
export function InspectorPane({ label, children }: { label: string; children: ReactNode }) {
  return (
    <aside
      id={inspectorId}
      aria-label={label}
      // **狭い画面では面を奪う。** 17rem の柱を 360px の脇に立てると、
      // 残るのは 5rem である。どちらも読めない 2 つより、読める 1 つを出す。
      // 閉じるのは header のトグルであり、それは面の上に残る。
      className="fixed inset-0 z-10 overflow-y-auto border-line bg-sidebar p-3
        lg:relative lg:z-auto lg:border-l"
    >
      {children}
    </aside>
  );
}
```

`fixed inset-0` は header も覆う。**それでよい** ——閉じるトグルが隠れるので、
`z-10` にして header（`z-30` のドロワーより下、面より上）より下に置く。header
に `relative z-20` を足して、トグルが必ず上に来るようにする。

- [ ] **Step 2: 検査して commit**

```sh
npm run typecheck --prefix web && npm test --prefix web
git add web/src/ui/Inspector.tsx web/src/App.tsx
git commit -m "feat: give the inspector the whole face when there is only one"
```

---

### Task 3: 接続の一覧と詳細を、狭い画面で置き換えにする

**Files:**
- Modify: `web/src/connections/ConnectionsPage.tsx:834-836`（グリッド）、`:877`（詳細ペイン）

**Interfaces:**
- Consumes: `selection`（既存の `HostSelection | null`）
- Produces: なし

- [ ] **Step 1: グリッドと 2 つのペインに breakpoint を足す**

```tsx
    {/* ウィンドウの端まで届く二つのペイン。detail の minmax(0,…) は、
        inspector が開いたときにも内容幅を保たず縮められるようにする。

        **狭い画面では二つではなく一つである。** どちらを出すかは幅ではなく
        「何かが選ばれているか」で決まり、それは既にルートが持っている。
        だから matchMedia は要らない——選択が変わればクラスが変わる。 */}
    <div className="grid h-full grid-cols-1 grid-rows-[minmax(0,1fr)] md:grid-cols-[19rem_minmax(0,1fr)]">
      <div className={`min-h-0 flex-col border-r border-line bg-tree md:flex ${
        selection === null ? "flex" : "hidden"
      }`}>
```

詳細側:

```tsx
      <div className={`min-h-0 flex-col gap-4 overflow-y-auto p-4 md:flex md:p-6 ${
        selection === null ? "hidden" : "flex"
      }`}>
```

- [ ] **Step 2: 詳細から一覧へ戻る道を出す**

詳細ペインの先頭に置く。`md:hidden`——広い画面では一覧が隣にあるので戻る先が
既に見えている。

```tsx
        <Button
          className="w-fit md:hidden"
          onClick={() => onSelect(null)}
        >
          {t("conn.backToList")}
        </Button>
```

`onSelect(null)` が既に選択解除として通る形かを確認すること。通らなければ、
選択を消す既存の経路（`emitLocation(connectionLocation(null))`）を使う。

`conn.backToList` を `en.ts` と `ja.ts` に足す。

- [ ] **Step 3: 検査して commit**

```sh
npm run typecheck --prefix web && npm test --prefix web
npm run check:i18n --prefix web
git add web/src/connections/ConnectionsPage.tsx web/src/i18n/messages/
git commit -m "feat: let the detail replace the list where both do not fit"
```

---

### Task 4: 端末を名指ししない文へ

**Files:**
- Modify: `web/src/i18n/messages/en.ts:64`
- Modify: `web/src/i18n/messages/ja.ts`（同じキー）

- [ ] **Step 1: `terminal.proxyCommandRefused` を書き換える**

いま:

> ProxyCommand starts a program, and sshc starts nothing to connect. Use ProxyJump, or connect from a terminal with ssh.

Android には打てる端末が無い。後半を落とし、このアプリが何をするかだけを言う。

```ts
  "terminal.proxyCommandRefused":
    "ProxyCommand starts a program, and sshc starts nothing to connect. Use ProxyJump instead.",
```

`ja.ts` の同じキーも同様に、端末を名指ししない文へ。

**他の文言は触らない。** ssh-agent とハードウェア鍵の文は、道具が無ければ
画面ごと出ないので、Android で読まれることがない。

- [ ] **Step 2: 検査して commit**

```sh
npm run check:i18n --prefix web && npm test --prefix web
git add web/src/i18n/messages/
git commit -m "fix: stop pointing at a terminal that may not exist"
```

---

### Task 5: 端末のキーバー

**このタスクだけがコンポーネントの新設である。** 物理キーボードの無い端末に
Esc も Ctrl も Tab も無く、それが無い端末では `ls` しか打てない。

**Files:**
- Create: `web/src/terminal/KeyBar.tsx`
- Create: `web/src/terminal/KeyBar.test.tsx`
- Modify: `web/src/terminal/TerminalView.tsx:248-307`

**Interfaces:**
- Consumes: なし
- Produces: `<KeyBar onSend={(data: string) => void} />`、`encodeKey(label: string, ctrl: boolean, alt: boolean): string`

- [ ] **Step 1: 失敗するテストを書く**

`web/src/terminal/KeyBar.test.tsx`:

```tsx
import { describe, expect, it } from "vitest";
import { encodeKey } from "./KeyBar";

describe("encodeKey", () => {
  // 修飾なしの特殊キーは、そのままの制御文字である。
  it("sends the control characters the keys stand for", () => {
    expect(encodeKey("Esc", false, false)).toBe("\x1b");
    expect(encodeKey("Tab", false, false)).toBe("\t");
    expect(encodeKey("↑", false, false)).toBe("\x1b[A");
    expect(encodeKey("↓", false, false)).toBe("\x1b[B");
    expect(encodeKey("→", false, false)).toBe("\x1b[C");
    expect(encodeKey("←", false, false)).toBe("\x1b[D");
  });

  // 記号はそのまま通る。ソフトキーボードで遠いものだけを並べている。
  it("passes literal characters through", () => {
    expect(encodeKey("|", false, false)).toBe("|");
    expect(encodeKey("~", false, false)).toBe("~");
  });

  // **Ctrl は sticky である。** 一度押してから次の 1 打鍵に乗る。
  // Ctrl+C が 0x03 にならなければ、走っているものを止められない。
  it("folds ctrl into the control range", () => {
    expect(encodeKey("c", true, false)).toBe("\x03");
    expect(encodeKey("C", true, false)).toBe("\x03");
    expect(encodeKey("d", true, false)).toBe("\x04");
  });

  // Alt は ESC を前置する。端末がそう約束している。
  it("prefixes alt with escape", () => {
    expect(encodeKey("b", false, true)).toBe("\x1bb");
  });

  // 両方立っているときは、Alt の ESC が Ctrl の制御文字の前に来る。
  it("puts the escape before the control character", () => {
    expect(encodeKey("c", true, true)).toBe("\x1b\x03");
  });

  // Ctrl は制御文字を持つ範囲にしか効かない。効かないものはそのまま送る
  // ——**押しても何も起きないより、押した文字が出る方がよい。**
  it("sends the plain character when ctrl has no meaning for it", () => {
    expect(encodeKey("|", true, false)).toBe("|");
  });
});
```

- [ ] **Step 2: テストが失敗することを確かめる**

```sh
npm test --prefix web -- KeyBar
```

Expected: FAIL — `encodeKey` が無い

- [ ] **Step 3: `KeyBar.tsx` を書く**

```tsx
import { useState } from "react";
import { useTranslate } from "../i18n/context";

// 特殊キーが立てる制御列。ここに無いラベルは、その文字そのものを送る。
const sequences: Record<string, string> = {
  Esc: "\x1b",
  Tab: "\t",
  "↑": "\x1b[A",
  "↓": "\x1b[B",
  "→": "\x1b[C",
  "←": "\x1b[D",
};

/**
 * encodeKey は、押されたキーと修飾から、端末へ送るバイト列を組み立てる。
 *
 * **Ctrl が効かない文字はそのまま送る。** 制御文字を持たない文字に Ctrl を
 * 乗せて何も送らないより、押した文字が出る方がよい——ソフトキーボードでは、
 * 何も起きないことと修飾が外れていないことが見分けられない。
 */
export function encodeKey(label: string, ctrl: boolean, alt: boolean): string {
  const sequence = sequences[label];
  if (sequence !== undefined) return alt ? "\x1b" + sequence : sequence;

  let body = label;
  if (ctrl) {
    const code = label.toLowerCase().charCodeAt(0);
    // 制御文字を持つのは a–z と、@ から _ までの記号である。
    if (code >= 97 && code <= 122) body = String.fromCharCode(code - 96);
  }
  return alt ? "\x1b" + body : body;
}

const keys = ["Esc", "Tab", "↑", "↓", "←", "→", "|", "-", "~", "/"];

/**
 * KeyBar は、物理キーボードの無い端末に Esc と Ctrl を与える。
 *
 * **狭い画面にだけ出す。** 物理キーボードがある画面では場所を取るだけである。
 */
export function KeyBar({ onSend }: { onSend: (data: string) => void }) {
  const t = useTranslate();
  const [ctrl, setCtrl] = useState(false);
  const [alt, setAlt] = useState(false);

  // **修飾は 1 打鍵で降りる。** 押しっぱなしになる修飾は、次に打った
  // 一文字が何になるか分からない端末を作る。
  function send(label: string) {
    onSend(encodeKey(label, ctrl, alt));
    setCtrl(false);
    setAlt(false);
  }

  const key = "min-h-11 min-w-11 rounded-md border border-control-line bg-card px-3 text-sm text-ink";
  return (
    <div
      aria-label={t("terminal.keyBar")}
      className="flex shrink-0 gap-1 overflow-x-auto border-t border-line bg-toolbar p-1 md:hidden"
    >
      <button type="button" aria-pressed={ctrl} onClick={() => setCtrl((on) => !on)}
        className={`${key} ${ctrl ? "bg-select-fill" : ""}`}>Ctrl</button>
      <button type="button" aria-pressed={alt} onClick={() => setAlt((on) => !on)}
        className={`${key} ${alt ? "bg-select-fill" : ""}`}>Alt</button>
      {keys.map((label) => (
        <button key={label} type="button" onClick={() => send(label)} className={key}>
          {label}
        </button>
      ))}
    </div>
  );
}
```

`terminal.keyBar` を `en.ts`（`"On-screen keys"`）と `ja.ts` に足す。

- [ ] **Step 4: テストが通ることを確かめる**

```sh
npm test --prefix web -- KeyBar
```

Expected: 7 件 PASS

- [ ] **Step 5: `TerminalView` に差し込む**

`TerminalView.tsx:306` の xterm のホスト `<div>` の直後に置く。送り先は
`onData` と同じ stream である。

```tsx
      <div ref={host} className="min-h-0 flex-1 bg-term-bg p-2" />
      <KeyBar onSend={(data) => stream?.send(data)} />
```

`stream` がこのスコープで参照できるかを確認すること。参照できなければ、
`view.input(data)` ではなく **stream へ直接送る** ——`view.input` はローカル
エコーであり、リモートには届かない。

- [ ] **Step 6: 検査して commit**

```sh
npm run typecheck --prefix web && npm test --prefix web && npm run check:i18n --prefix web
git add web/src/terminal/ web/src/i18n/messages/
git commit -m "feat: give the terminal the keys a phone does not have"
```

---

### Task 6: 360×800 で守る

**vitest では見えない。** jsdom にレイアウトが無いので媒体クエリは効かない。
ここまでのタスクが本当に効いているかを言えるのは Playwright だけである。

**Files:**
- Modify: `web/playwright.config.ts`
- Create: `web/e2e/narrow.spec.ts`

- [ ] **Step 1: project を足す**

```ts
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    // 360×800 は、いま売られている最も狭い Android である。**ここで成立
    // すれば、それより広いすべてで成立する。** タッチを有効にするのは、
    // hover に依存した導線がこの幅に残っていないことを同時に見るためである。
    {
      name: "narrow",
      use: { ...devices["Desktop Chrome"], viewport: { width: 360, height: 800 }, hasTouch: true },
    },
  ],
```

- [ ] **Step 2: 既存の e2e からヘルパーの形を読む**

```sh
ls web/e2e && grep -n "test(" web/e2e/*.spec.ts | head
```

**新しい harness を作らない。** 一時 HOME と sshc の起動は既存のものを使う。

- [ ] **Step 3: `narrow.spec.ts` を書く**

守るのは 4 つ。既存の harness に合わせて組み立てること。

```ts
// 1. 横スクロールが無い
const overflows = await page.evaluate(
  () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
);
expect(overflows).toBe(false);

// 2. ドロワーから遷移できる
await page.getByRole("button", { name: "Primary navigation" }).click();
await page.getByRole("link", { name: "Keys" }).click();
await expect(page.getByRole("heading", { name: "Keys" })).toBeVisible();

// 3. 接続を選ぶと詳細が一覧を置き換え、戻れる
//    一覧の見出しが消え、Back を押すと戻ってくることで表明する。

// 4. キーバーの Ctrl + C が 0x03 を送る
//    走らせた sleep が止まることで表明する。**送ったバイトを覗かない** ——
//    見たいのは「止まること」であって、通信路の中身ではない。
```

- [ ] **Step 4: 走らせる**

```sh
make e2e
```

Expected: chromium と narrow の両方が PASS。

- [ ] **Step 5: Commit**

```sh
git add web/playwright.config.ts web/e2e/narrow.spec.ts
git commit -m "test: hold the narrow face to the same promises"
```

---

## 完了後

`android-responsive-ui` を main へマージする。`android-engine` とは触るファイルが
重ならない（Go と `android/` 対 `web/`）ので、順序はどちらでもよい。

両方が入ったら、Android 実機で `docs/manual-acceptance.md` の M6 を通す。
