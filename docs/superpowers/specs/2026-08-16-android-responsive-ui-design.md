# Android 対応 (2) — 画面を狭い幅で成立させる

この画面は幅 1024px 以上を前提に組まれている。360px で開くと、中央のペインに
4rem しか残らない。

壊れている場所は 4 つで、そのうち 1 つは既に直っている。

| 場所 | 現状 |
|---|---|
| `App.tsx:532` | シェルが `grid-cols-[15rem_minmax(0,1fr)_17rem]` 固定 |
| `App.tsx:464` | ヘッダー 1 行に 8 個のコントロール |
| `ConnectionsPage.tsx:834` | `grid-cols-[19rem_minmax(0,1fr)]` 固定 |
| `ConfigExplorer.tsx:257` | 既に `lg:` で分岐済み。**触らない** |

加えて、ナビゲーションのリンクが `px-2 py-1`（高さ約 24px）で、タッチターゲットの
最小 44px を大きく下回る。

## breakpoint

Tailwind の既定値をそのまま使う。設定ファイルに手を入れない。

| 幅 | シェル |
|---|---|
| `< md` (768) | 1 ペイン。ナビはドロワー、Inspector は全画面シート |
| `md` – `lg` | 2 ペイン（ナビ + 本体）。Inspector はシート |
| `>= lg` (1024) | 現状の 3 ペイン |

## JS の媒体クエリは 1 つも要らない

`useIsCompact()` のようなフックを作らない。**幅を JS で読む必要が無い。**

一覧と詳細の出し分けは、幅ではなく「何かが選択されているか」で決まる。
`web/src/routing/connectionRoute.ts` が既にその状態をルートに持っているので、
`className={selected ? "hidden md:block" : "block"}` で足りる。狭い画面では詳細が
一覧を置き換え、広い画面では並ぶ。`matchMedia` も `ResizeObserver` も要らない。

ドロワーの開閉だけが `useState` になるが、これは媒体クエリではない。`md:` 以上で
Tailwind 側が状態を無視する形（`md:relative md:translate-x-0`）にするので、幅が
変わったときに状態をリセットする処理も要らない。

## シェル

**ヘッダー。** `< md` で落とすのは、アプリ名、区切りのスラッシュ、稼働状態、
そして外観と言語のラベル文字である。残すのはハンバーガー、セクション名、
Inspector トグル、ラベルなしの select 2 つ。**`aria-label` は残す**——見える文字を
消すことと、名前を消すことは違う。ラベル文字は `hidden md:inline`、稼働状態は
`hidden sm:flex`。

**ナビ。** `< md` では `fixed inset-y-0 left-0 w-72 -translate-x-full` のドロワーと
オーバーレイ。`md:` 以上で今の `15rem` の列に戻る。開けるのはハンバーガーだけ、
閉じるのはリンクの選択、オーバーレイのタップ、Esc。

リンクのパディングを `px-3 py-2.5` に上げる。**`< md` に限定しない。** 24px の
クリック標的はデスクトップでも狭く、条件付きにする理由がない。

**Inspector。** `< lg` では `fixed inset-0` のシート。`InspectorToggle` は既に
`sm:` を持っているのでトグル自体は動く。`lg:` 以上で今の `17rem` の列。

グリッドは 1 行に収まる。

```
grid-cols-1
md:grid-cols-[15rem_minmax(0,1fr)]
lg:grid-cols-[15rem_minmax(0,1fr)_17rem]   // inspector が開いているときだけ
```

## ターミナル

ここだけはクラスを足すのではなく、コンポーネントを新規に作る。

**キーバー。** 物理キーボードの無い端末に Esc も Ctrl も Tab も無い。xterm の下に
固定のキー列を置く。

```
Esc  Tab  Ctrl  Alt  ↑ ↓ ← →  |  -  ~  /
```

`Ctrl` と `Alt` は sticky——一度タップすると次の 1 打鍵にだけ修飾が乗る。`< md`
でだけ出す。これが無いと Android のターミナルは `ls` しか打てない。

**仮想キーボード。** `web/index.html` の viewport meta に
`interactive-widget=resizes-content` を足す。キーボードが出たときにレイアウト
ビューポートが縮み、既に入っている `@xterm/addon-fit` がそのまま追従する。1 行で
済む。

## CLI 前提の文言

調べた結果、**ここはほとんど作業が無い。** 既にある仕組みが閉じる。

- ssh-agent: `KeysScreen.tsx:664` が `disabled={!inventory.agentAvailable}` で
  閉じている。Android では `KeyAgent` が nil なので、そのまま無効になる。
- ハードウェア鍵: `Toolchain` が ssh-keygen を見つけられるかで一覧に出すかが
  決まる。Android では `Toolchain` が空である。
- 残るのは `en.ts:64` の 1 文だけ——「Use ProxyJump, or connect from a terminal
  with ssh.」。端末を名指ししない文に書き換える。`ja.ts` も同じ。

**プラットフォーム能力を伝える API を追加しない。** `HealthResponse` に
`platform` を足せば、3 箇所のために分岐点を全画面に配ることになる。依存を nil に
すれば機能が消えるという既存の設計が、既にこれを担っている。

ただし「本当に閉じる」ことは今どこにも表明されていない。表明はサブプロジェクト 1
側のテストで足す。

## テスト

**vitest では見えない。** jsdom にレイアウトが無いので媒体クエリは効かず、クラス名
の文字列を表明するテストは、リファクタで即座に壊れる割に何も守らない。

`web/playwright.config.ts` に **360×800 の project を足し**、既存の主要フローを
その幅でもう一周させる。守る対象は 4 つ。

1. 横スクロールが発生しないこと（`document.documentElement.scrollWidth <=
   clientWidth`）
2. ドロワーからの遷移が成立すること
3. 接続を選ぶと詳細が一覧を置き換え、戻れること
4. キーバーの `Ctrl` + `C` が実際に `0x03` を送ること

vitest 側に足すのは 1 つだけ——キーバーの sticky 修飾のロジック。

## スコープ外

ダークモードの見直し（`theme/` が既に持っている）。`ConfigExplorer` の再設計。
アニメーション。スワイプジェスチャ。画面回転時のレイアウト保存。
