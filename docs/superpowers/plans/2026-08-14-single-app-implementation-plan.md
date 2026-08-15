# ひとつのアプリにする 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 外殻とエンジンを、メニューバーに常駐するひとつのアプリにする。エンジンはアプリの子として生き、終了すれば一緒に終わる。

**Architecture:** Electron の外殻が `sshc` を子プロセスとして起こし、その標準出力から入口の URL を受け取る。二重起動は Electron 側のロックが止め、異常終了は子側の親監視が拾う。ログイン項目は OS に任せて plist を書く仕組みごと消す。端末からはマスターパスワードを答えられるようにし、解錠はエンジンの中に残る。

**Tech Stack:** Go 1.26 / Electron 43 / echo v5 / node:test

**Spec:** `docs/superpowers/specs/2026-08-14-single-app-design.md`

## Global Constraints

- ブランチは `claude/single-app`。**下位互換は取らない**——移行のコードを書かない
- **`~/.ssh/sshc/cli`（handoff）は残す。** CLI がエンジンを見つける唯一の手段である
- **`sshc open`・`sshc list`・`sshc connect` は残す**
- **画面の無い機械でエンジンだけを動かす道は無くなる**（`sshc engine start` を消すため）
- コメントは日本語。**なぜそうしたかを書き、何をしているかは書かない**
- 検査を消してよいのは、検査対象が消えるときだけ
- 各タスクの最後に `go build ./...` と、そのタスクが触った検査を通す

---

### Task 1: 外殻がエンジンを直接起こす

`engine start` を経由せず `spawn` し、標準出力の 1 行目から入口を受け取る。

**Files:**
- Create: `desktop/entrance.js`
- Create: `desktop/entrance.test.js`
- Modify: `desktop/main.js:58-77`（`entrance()`）, `desktop/main.js:163-183`（`whenReady`）
- Modify: `desktop/package.json`（`test` スクリプトに新しい検査を足す）

**Interfaces:**
- Produces: `parseEntrance(text: string): string | null` — 標準出力の断片から `http://127.0.0.1:` で始まる最初の行を返す。無ければ `null`

- [ ] **Step 1: 失敗する検査を書く**

`desktop/entrance.test.js`:

```js
"use strict";

const { test } = require("node:test");
const assert = require("node:assert");
const { parseEntrance } = require("./entrance");

// **エンジンは入口を 1 行で書き出す。** 起こした本人だけがそのパイプを持つ。
test("takes the entrance out of what the engine printed", () => {
  const text = "sshc: listening\nhttp://127.0.0.1:52683/?token=abc\n";
  assert.strictEqual(parseEntrance(text), "http://127.0.0.1:52683/?token=abc");
});

// **ループバック以外は入口ではない。** ここを緩めると、エンジンが答えた
// つもりの何かを窓に読ませることになる。
test("refuses an address that is not loopback", () => {
  assert.strictEqual(parseEntrance("http://example.com/\n"), null);
});

test("answers null until the line has arrived", () => {
  assert.strictEqual(parseEntrance("sshc: starting\n"), null);
});
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `node --test desktop/entrance.test.js`
Expected: FAIL（`Cannot find module './entrance'`）

- [ ] **Step 3: 実装する**

`desktop/entrance.js`:

```js
"use strict";

/**
 * parseEntrance は、エンジンが書き出した入口の URL を取り出す。
 *
 * **起こした本人が、起こした子のパイプから受け取る。** ファイルを経由しない
 * のは、あの 1 行に有効な bootstrap トークンが乗っているからである。
 */
function parseEntrance(text) {
  for (const line of String(text).split("\n")) {
    const candidate = line.trim();
    if (candidate.startsWith("http://127.0.0.1:")) return candidate;
  }
  return null;
}

module.exports = { parseEntrance };
```

- [ ] **Step 4: 通ることを確かめる**

Run: `node --test desktop/entrance.test.js`
Expected: PASS（3 件）

- [ ] **Step 5: 外殻を繋ぎ替える**

`desktop/main.js` の `entrance()` を次に置き換える。`execFile` の `run` は
`sshc open` などで使い続けるので残す。

```js
const { spawn } = require("node:child_process");
const { parseEntrance } = require("./entrance");

// engine は、このアプリが起こしたエンジンひとつである。
//
// **detach しない。** 親と一緒に死ぬことが、このアプリケーションが
// 「終了すれば全部止まる」と言えることの実装そのものである。
let engine = null;

/**
 * entrance は、エンジンを起こし、入口の URL を返す。
 */
function entrance() {
  return new Promise((resolve, reject) => {
    // **実体を 1 つにする。** 失敗しても続ける——リンクが張れないことは、
    // アプリが開けない理由にはならない。
    relink(binary()).catch(() => {});

    const child = spawn(binary(), [], { stdio: ["ignore", "pipe", "pipe"] });
    engine = child;

    let buffered = "";
    const timer = setTimeout(
      () => reject(new Error("the engine printed no entrance within 20s")),
      20_000,
    );
    const settle = (error, url) => {
      clearTimeout(timer);
      if (error) reject(error);
      else resolve(url);
    };

    child.stdout.on("data", (chunk) => {
      buffered += String(chunk);
      const url = parseEntrance(buffered);
      if (url !== null) settle(null, url);
    });
    child.on("error", (error) => settle(error));
    child.on("exit", (code) => settle(new Error(`the engine exited with ${code}`)));
  });
}
```

`whenReady` の中の `openWindow(await entrance())` はそのままでよい。

- [ ] **Step 6: 終了時に子を畳む**

`desktop/main.js` の `before-quit` を次に置き換える。`sshc engine quit` は
Task 8 で消えるが、ここで先に呼ばなくする。

```js
app.on("before-quit", () => {
  // **持ち主はこのプロセスである。** 設定を読んで決めるものは、もう無い。
  if (engine !== null) engine.kill();
});
```

- [ ] **Step 7: 二重起動を止める**

`desktop/main.js` の `app.setName("sshc");` の直後に置く。

```js
// **エンジンが 2 台になる道を、ここで塞ぐ。** 起こし手がひとつなら、
// 名簿もロックファイルも要らない。
if (!app.requestSingleInstanceLock()) app.exit(0);
```

- [ ] **Step 8: package.json の検査を足す**

`desktop/package.json` の `scripts.test` を
`"node --test link.test.js entrance.test.js"` にする。

- [ ] **Step 9: 通す**

Run: `npm test --prefix desktop`
Expected: PASS

- [ ] **Step 10: commit**

```bash
git add desktop/entrance.js desktop/entrance.test.js desktop/main.js desktop/package.json
git commit -m "feat: 外殻がエンジンを子として起こす"
```

---

### Task 2: 親が居なくなったら畳む

アプリが SIGKILL やクラッシュで消えても、エンジンを残さない。

**Files:**
- Create: `cmd/sshc/watch.go`
- Create: `cmd/sshc/watch_test.go`
- Modify: `cmd/sshc/main.go:205-206`（`signal.NotifyContext` の直後）

**Interfaces:**
- Produces: `watchParent(ctx context.Context, parent func() int, tick time.Duration, stop func())` — `parent()` が 1 を返したら `stop()` を呼んで戻る。`ctx` が終われば `stop` を呼ばずに戻る

- [ ] **Step 1: 失敗する検査を書く**

`cmd/sshc/watch_test.go`:

```go
package main

import (
	"context"
	"testing"
	"time"
)

// **親が死んだ子は init に引き取られる。** それがこの見張りの唯一の手掛かりで
// ある。通常の終了では親が kill するので、ここは異常終了のための最後の網である。
func TestWatchParentStopsWhenTheParentIsGone(t *testing.T) {
	readings := []int{4242, 4242, 1}
	index := 0
	parent := func() int {
		reading := readings[index]
		if index < len(readings)-1 {
			index++
		}
		return reading
	}

	stopped := make(chan struct{})
	go watchParent(context.Background(), parent, time.Millisecond, func() { close(stopped) })

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the watch never noticed the parent was gone")
	}
}

// 見張りは、止めろと言われたら止める。親が居るあいだは何もしない。
func TestWatchParentLetsGoWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchParent(ctx, func() int { return 4242 }, time.Millisecond, func() {
			t.Error("the watch stopped a process whose parent was alive")
		})
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the watch ignored the context")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./cmd/sshc/ -run WatchParent`
Expected: FAIL（`undefined: watchParent`）

- [ ] **Step 3: 実装する**

`cmd/sshc/watch.go`:

```go
package main

import (
	"context"
	"time"
)

// parentTick は、親を見に行く間隔である。
//
// **これは異常終了のための網であり、通常の終了経路ではない。** 普通に終わる
// ときは親が kill するので、ここが気づくまでの 1 秒は誰も待たない。
const parentTick = time.Second

// watchParent は、親が居なくなったら stop を呼ぶ。
//
// **親が死んだ子は init に引き取られる。** それを見て自分で畳むのは、
// このアプリケーションが「終了すれば全部止まる」と言えるための最後の網である。
// アプリが SIGKILL された日でも、エンジンだけが残ることはない。
func watchParent(ctx context.Context, parent func() int, tick time.Duration, stop func()) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if parent() == 1 {
				stop()
				return
			}
		}
	}
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./cmd/sshc/ -run WatchParent`
Expected: PASS

- [ ] **Step 5: 配線する**

`cmd/sshc/main.go` の `ctx, stop := signal.NotifyContext(...)` と `defer stop()`
の直後に置く。

```go
	// **アプリが消えたらエンジンも消える。** 親を見張るのは、通常の終了
	// 経路（親が kill する）が働かなかったときのためである。
	go watchParent(ctx, os.Getppid, parentTick, stop)
```

- [ ] **Step 6: 通す**

Run: `go build ./... && go test ./cmd/sshc/`
Expected: PASS

- [ ] **Step 7: commit**

```bash
git add cmd/sshc/watch.go cmd/sshc/watch_test.go cmd/sshc/main.go
git commit -m "feat: 親が居なくなったエンジンは自分で畳む"
```

---

### Task 3: `GET /cli/status`

メニューバーと終了時の確認が読む、施錠状態と開いている本数。

**Files:**
- Modify: `internal/httpserver/connect.go`（`ConnectHandlers` に `Sessions` を足し、ルートと handler を足す）
- Modify: `internal/httpserver/server.go:243-262`（`ConnectHandlers` の組み立て）
- Test: `internal/httpserver/connect_test.go`

**Interfaces:**
- Produces: `httpserver.StatusPath = "/cli/status"`、応答は `{"unlocked": bool, "sessions": int}`
- Consumes: `ConnectHandlers.Sessions func() int`（nil なら 0）

- [ ] **Step 1: 失敗する検査を書く**

`internal/httpserver/connect_test.go` の末尾に足す。

```go
// メニューバーと終了時の確認が読む口である。**数えるのは生きている本数だけ**
// で、終了済みは残っていても数に入らない——閉じてよいかを問うための数だからだ。
func TestStatusAnswersWithTheLockAndTheLiveCount(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	engine := connectEngine(t, ConnectHandlers{
		Secret: cliSecret, Passwords: vault, Sessions: func() int { return 3 },
	})

	recorder := send(t, engine, http.MethodGet, StatusPath, "",
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer statusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if !answer.Unlocked || answer.Sessions != 3 {
		t.Fatalf("answer = %+v", answer)
	}
}

// handoff の秘密を持たないものには答えない。
func TestStatusRefusesWithoutTheSecret(t *testing.T) {
	engine := connectEngine(t, ConnectHandlers{Secret: "the secret for this run"})
	if recorder := send(t, engine, http.MethodGet, StatusPath, "", nil); recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/httpserver/ -run Status`
Expected: FAIL（`undefined: StatusPath`）

- [ ] **Step 3: 実装する**

`internal/httpserver/connect.go` に足す。`registerConnectRoutes` の中で
`engine.GET(StatusPath, handlers.Status)` を登録すること。

```go
// StatusPath は、外殻が「いまどうなっているか」を尋ねる場所である。
//
// **これは画面のための口ではない。** 画面は自分の session を持っている。
// ここが答える相手はメニューバーであり、認可は handoff の秘密ひとつである。
const StatusPath = "/cli/status"

type statusResponse struct {
	// Unlocked は vault が開いているか。
	Unlocked bool `json:"unlocked"`
	// Sessions は生きているコンソールの本数。終了済みは数えない——
	// 「閉じてよいか」を問うための数だからである。
	Sessions int `json:"sessions"`
}

func (h ConnectHandlers) Status(c *echo.Context) error {
	if !h.authorised(c.Request()) {
		return c.NoContent(http.StatusForbidden)
	}
	answer := statusResponse{}
	if h.Passwords != nil {
		answer.Unlocked = h.Passwords.Unlocked()
	}
	if h.Sessions != nil {
		answer.Sessions = h.Sessions()
	}
	return c.JSON(http.StatusOK, answer)
}
```

`ConnectHandlers` に足すフィールド:

```go
	// Sessions は、生きているコンソールの本数を返す。nil なら 0。
	Sessions func() int
```

`authorised(request *http.Request) bool` は `Health` が既に使っている。
**同じ検査を二度書かない。**

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/httpserver/ -run Status`
Expected: PASS

- [ ] **Step 5: 配線する**

`internal/httpserver/server.go` の `ConnectHandlers{...}` に足す。

```go
		Sessions: func() int {
			if options.Terminals == nil {
				return 0
			}
			live := 0
			for _, view := range options.Terminals.Sessions() {
				if view.Exited == nil {
					live++
				}
			}
			return live
		},
```

- [ ] **Step 6: 通す**

Run: `go build ./... && go test ./internal/httpserver/`
Expected: PASS

- [ ] **Step 7: commit**

```bash
git add internal/httpserver/connect.go internal/httpserver/connect_test.go internal/httpserver/server.go
git commit -m "feat: 外殻が施錠状態と本数を尋ねられるようにする"
```

---

### Task 4: メニューバーに常駐する

**Files:**
- Create: `desktop/tray.js`
- Modify: `desktop/main.js`（`window-all-closed`、`whenReady`、`before-quit`）
- Modify: `desktop/icon.mjs`（テンプレート画像を出す）
- Modify: `web/src/i18n/messages/{ja,en}.ts` は**触らない**（メニューは外殻が持つ文字列で、i18n の対象外）

**Interfaces:**
- Consumes: `parseEntrance`（Task 1）、`GET /cli/status`（Task 3）
- Produces: `installTray({ onOpen, onQuit, status }): Tray` — `status()` は
  `Promise<{unlocked: boolean, sessions: number}>` を返す関数

- [ ] **Step 1: テンプレート画像を作る**

`desktop/build/tray.svg` を作る。**アプリのアイコンとは別の図である**——
メニューバーの図は単色の輪郭で、色も影も持たない。黒一色・透過背景で描く。

`desktop/icon.mjs` は `build/icon.svg` を Playwright の chromium で 1024px に
焼いている。同じ関数を使い回して、`build/tray.svg` を 16px と 32px で焼き、
`build/trayTemplate.png` と `build/trayTemplate@2x.png` として書く。**macOS は
`Template` で終わる名前を見て、明暗に合わせて色を反転する**ので、この名前は
飾りではない。既存の焼き込み処理を関数へ括り出し、3 回呼ぶ形にすること。

Run: `npm run icons --prefix desktop`
Expected: `desktop/build/trayTemplate.png` と `@2x` ができている

- [ ] **Step 2: tray を書く**

`desktop/tray.js`:

```js
"use strict";

const { Menu, Tray, nativeImage } = require("electron");
const { join } = require("node:path");

/**
 * installTray は、メニューバーに項目をひとつ置く。
 *
 * **これは飾りではない。** このアプリはウィンドウを閉じても終わらないので、
 * 動いていることの証拠がどこにも無くなる。転送 1 本について「開いていることが
 * 見えないまま開かない」と言っている以上、エンジン自身がその規則の外に居ては
 * ならない。
 */
function installTray({ onOpen, onQuit, status }) {
  const image = nativeImage.createFromPath(join(__dirname, "build", "trayTemplate.png"));
  image.setTemplateImage(true);
  const tray = new Tray(image);
  tray.setToolTip("sshc");

  // **開くたびに数え直す。** 貼りっぱなしのメニューは、閉じたはずの
  // コンソールをいつまでも数えて見せる。
  const show = async () => {
    let line = "sshc";
    try {
      const answer = await status();
      line = `${answer.unlocked ? "解錠中" : "施錠中"} · コンソール ${answer.sessions}`;
    } catch {
      line = "エンジンに繋がりません";
    }
    tray.popUpContextMenu(Menu.buildFromTemplate([
      { label: line, enabled: false },
      { type: "separator" },
      { label: "ウィンドウを開く", click: onOpen },
      { label: "sshc を終了", click: onQuit },
    ]));
  };

  tray.on("click", show);
  tray.on("right-click", show);
  return tray;
}

module.exports = { installTray };
```

- [ ] **Step 3: 外殻に繋ぐ**

`desktop/main.js`:

```js
const { installTray } = require("./tray");

let tray = null;
```

`whenReady` の中、`openWindow(await entrance())` の後に置く。

```js
  tray = installTray({
    onOpen: () => {
      if (BrowserWindow.getAllWindows().length === 0) openWindow(url);
      else BrowserWindow.getAllWindows()[0].show();
    },
    onQuit: () => app.quit(),
    // Task 6 で `sshc status` を足してから本物に差し替える。**handoff の
    // 秘密を持つのは Go 側だけ**なので、外殻は自分では叩けない。
    status: async () => ({ unlocked: false, sessions: 0 }),
  });
```

- [ ] **Step 4: 窓を閉じても終わらないようにする**

`desktop/main.js` の `window-all-closed` を置き換える。

```js
// **窓を閉じてもアプリは残る。** メニューバーの項目がその意味である
// ——ウィンドウが無いのに動き続ける外殻に意味を与えたのは、あの項目である。
app.on("window-all-closed", () => {});
```

macOS では、窓が無いあいだ Dock から消す。

```js
  if (app.dock !== undefined) app.dock.hide();
```

を窓が全部閉じたときに呼び、`openWindow` の先頭で `app.dock?.show()` を呼ぶ。

- [ ] **Step 5: 終了の確認を足す**

`before-quit` を置き換える。

```js
// liveSessions は、生きているコンソールの本数を返す。Task 6 で
// `sshc status` を叩く形に差し替える。
async function liveSessions() {
  return 0;
}

let quitting = false;
app.on("before-quit", async (event) => {
  if (quitting) return;
  event.preventDefault();
  // **SSH のセッションは閉じたら戻らない。** 本数を出して一度だけ問う。
  // Task 6 で `sshc status` を足すまでは 0 を返す。
  const open = await liveSessions();
  if (open > 0) {
    const answer = await dialog.showMessageBox({
      type: "question",
      buttons: ["終了", "やめる"],
      defaultId: 1,
      cancelId: 1,
      message: `開いているコンソールが ${open} 本あります。終了しますか。`,
    });
    if (answer.response !== 0) return;
  }
  quitting = true;
  if (engine !== null) engine.kill();
  app.quit();
});
```

- [ ] **Step 6: 手で確かめる**

Run: `make desktop-run`
Expected:
1. メニューバーに項目が出る
2. ウィンドウを閉じてもアプリが残り、Dock から消える
3. メニューバーの「ウィンドウを開く」で戻る
4. 「sshc を終了」で `pgrep -f "sshc -open" ` が空になる
5. Ctrl-C でも 4 と同じ

- [ ] **Step 7: commit**

```bash
git add desktop/tray.js desktop/main.js desktop/icon.mjs desktop/build/trayTemplate*.png
git commit -m "feat: メニューバーに常駐する"
```

---

### Task 5: `POST /cli/unlock`

端末からマスターパスワードを答えられるようにする。

**Files:**
- Modify: `internal/httpserver/connect.go`（ルートと handler）
- Test: `internal/httpserver/connect_test.go`

**Interfaces:**
- Produces: `httpserver.UnlockPath = "/cli/unlock"`。要求は `{"passphrase": "..."}`、成功は 204、失敗は 403

- [ ] **Step 1: 失敗する検査を書く**

`internal/httpserver/connect_test.go` の末尾に足す。

```go
// **端末でも解錠できる。** ブラウザを開かずに答えられることが、この口の理由で
// ある。解錠はエンジンの中に残るので、あとで窓を開けば解錠済みである。
func TestUnlockOpensTheVaultFromTheCommandLine(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	vault.Lock()
	engine := connectEngine(t, ConnectHandlers{Secret: cliSecret, Passwords: vault})

	body := `{"passphrase":"` + testPassphrase + `"}`
	recorder := send(t, engine, http.MethodPost, UnlockPath, body,
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unlock = %d: %s", recorder.Code, recorder.Body.String())
	}
	if !vault.Unlocked() {
		t.Fatal("the vault stayed locked")
	}
}

// 間違いは拒む。どう間違っていたかは言わない。
func TestUnlockRefusesTheWrongPassphrase(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	vault.Lock()
	engine := connectEngine(t, ConnectHandlers{Secret: cliSecret, Passwords: vault})

	recorder := send(t, engine, http.MethodPost, UnlockPath, `{"passphrase":"wrong"}`,
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusForbidden || vault.Unlocked() {
		t.Fatalf("unlock = %d, unlocked = %v", recorder.Code, vault.Unlocked())
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/httpserver/ -run Unlock`
Expected: FAIL（`undefined: UnlockPath`）

- [ ] **Step 3: 実装する**

`internal/httpserver/connect.go` に足し、`registerConnectRoutes` へ
`engine.POST(UnlockPath, handlers.Unlock)` を登録する。

```go
// UnlockPath は、端末からマスターパスワードを答える場所である。
//
// **ブラウザを開かずに答えられることが、この口の理由である。** 解錠はエンジンの
// 中に残るので、あとで窓を開けば解錠済みである。マスターパスワードは、画面から
// も同じ loopback を通っている——この経路が増やすのは「解錠を試せる」ことだけ
// で、それには本人がマスターパスワードを知っている必要がある。
const UnlockPath = "/cli/unlock"

type unlockRequest struct {
	Passphrase string `json:"passphrase"`
}

func (h ConnectHandlers) Unlock(c *echo.Context) error {
	if !h.authorised(c.Request()) {
		return c.NoContent(http.StatusForbidden)
	}
	if h.Passwords == nil {
		return c.NoContent(http.StatusForbidden)
	}
	var decoded unlockRequest
	if err := json.NewDecoder(io.LimitReader(c.Request().Body, maxConnectBody)).Decode(&decoded); err != nil {
		return c.NoContent(http.StatusBadRequest)
	}
	// **どう間違っていたかは言わない。** 施錠されているかどうかも含めて、
	// 答えは「開いた」か「開かない」の二つである。
	if err := h.Passwords.Unlock(decoded.Passphrase); err != nil {
		return c.NoContent(http.StatusForbidden)
	}
	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/httpserver/ -run Unlock`
Expected: PASS

- [ ] **Step 5: commit**

```bash
git add internal/httpserver/connect.go internal/httpserver/connect_test.go
git commit -m "feat: 端末からマスターパスワードを答えられるようにする"
```

---

### Task 6: 端末の経路（アプリを隠しで起こす・解錠を聞く・status サブコマンド）

**Files:**
- Create: `cmd/sshc/launch_darwin.go`
- Create: `cmd/sshc/launch_other.go`
- Modify: `cmd/sshc/connect.go:118-150`（`runConnect`）
- Modify: `cmd/sshc/main.go`（`status` サブコマンドの分岐）
- Modify: `desktop/main.js`（`--hidden` と、Task 4 Step 3 の `status` 差し替え）

**Interfaces:**
- Produces: `launchApp() bool` — アプリを隠しで起こせたら true。darwin 以外は常に false
- Produces: `sshc status` — `{"unlocked":bool,"sessions":int}` を標準出力へ

- [ ] **Step 1: アプリを起こす口を書く**

`cmd/sshc/launch_darwin.go`:

```go
package main

import "os/exec"

// bundleID は、束を LaunchServices が知っている名前である。
// **場所を覚えない。** どこに置かれていても、この名前で起こせる。
const bundleID = "com.github.aida0710.sshc"

// launchApp は、アプリを窓なしで起こす。
//
// **-g は前面に出さないという意味である。** 端末で打ったコマンドが、勝手に
// 画面を奪ってはならない。--hidden は窓を作らないという外殻への指示であり、
// メニューバーの項目は出るので、上がったことは見える。
func launchApp() bool {
	return exec.Command("open", "-g", "-b", bundleID, "--args", "--hidden").Run() == nil
}
```

`cmd/sshc/launch_other.go`:

```go
//go:build !darwin

package main

// launchApp は、darwin 以外では何もしない。
//
// **起こし方が環境で割れるものを、推測して起こさない。** 起こせなければ
// 保存済みを使わずに繋ぐ経路へ退く——接続そのものは常にできる。
func launchApp() bool { return false }
```

`launch_darwin.go` の先頭に `//go:build darwin` は不要（ファイル名で決まる）。

- [ ] **Step 2: 繋ぐ前にアプリを起こす**

`cmd/sshc/connect.go` の `runConnect` で、`askApplication` が失敗したときに
一度だけ `launchApp()` を試し、成功したら `askApplication` をやり直す。

```go
	answer, err := askApplication(ctx, alias, stateDir, client)
	if err != nil && launchApp() {
		// **上がるまで待つ。** 待ち方を知っているのはここだけで、
		// 上限は外殻が入口を書き出すのに掛ける時間と同じにしてある。
		for attempt := 0; attempt < 40 && err != nil; attempt++ {
			time.Sleep(500 * time.Millisecond)
			answer, err = askApplication(ctx, alias, stateDir, client)
		}
	}
```

- [ ] **Step 3: 施錠されていたら聞く**

同じ `runConnect` の中、`answer` を得たあとに置く。`answer` に施錠状態が
無いので、`GET /cli/status` を叩いてから決める。

```go
	// **ブラウザを開かずに答えられる。** 解錠はエンジンの中に残るので、
	// あとで窓を開けば解錠済みである。
	if err == nil && locked(ctx, stateDir, client) {
		fmt.Fprint(stderr, "sshc: master password (leave empty to skip): ")
		typed, readErr := term.ReadPassword(int(stdin.Fd()))
		fmt.Fprintln(stderr)
		if readErr == nil && len(typed) > 0 {
			// **間違えても聞き直さない。** 繋げることの方が強い要求であり、
			// 誤りの遅延を端末で待たせる価値が無い。
			if unlock(ctx, stateDir, client, string(typed)) {
				answer, _ = askApplication(ctx, alias, stateDir, client)
			}
		}
	}
```

`locked` と `unlock` を `cmd/sshc/connect.go` に足す。**`askApplication` と
同じ形で書く**——handoff を読み、秘密を添えて叩く。

```go
// engineStatus は、エンジンがいまどうなっているかを尋ねる。
func engineStatus(ctx context.Context, stateDir string, client *http.Client) (statusAnswer, error) {
	found, err := handoff.Read(stateDir)
	if err != nil {
		return statusAnswer{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		found.URL+httpserver.StatusPath, nil)
	if err != nil {
		return statusAnswer{}, err
	}
	request.Header.Set(handoff.HeaderName, found.Secret)

	response, err := client.Do(request)
	if err != nil {
		return statusAnswer{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return statusAnswer{}, fmt.Errorf("sshc refused the request")
	}
	var answer statusAnswer
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&answer); err != nil {
		return statusAnswer{}, err
	}
	return answer, nil
}

// statusAnswer は、エンジンが答える「いまどうなっているか」である。
type statusAnswer struct {
	Unlocked bool `json:"unlocked"`
	Sessions int  `json:"sessions"`
}

// unlock は、答えられたマスターパスワードをエンジンへ渡す。
//
// **開いたかどうかしか返らない。** どう間違っていたかを、この経路は言わない。
func unlock(ctx context.Context, stateDir string, client *http.Client, passphrase string) bool {
	found, err := handoff.Read(stateDir)
	if err != nil {
		return false
	}
	body, err := json.Marshal(map[string]string{"passphrase": passphrase})
	if err != nil {
		return false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		found.URL+httpserver.UnlockPath, bytes.NewReader(body))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(handoff.HeaderName, found.Secret)

	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode == http.StatusNoContent
}
```

Step 3 の `locked(...)` は
`status, err := engineStatus(ctx, stateDir, client); err == nil && !status.Unlocked`
に読み替えること。

- [ ] **Step 4: `sshc status` を足す**

`cmd/sshc/main.go` の、他のサブコマンド（`open`、`list`）を分岐している所へ
足す。**外殻が読む口である**——handoff の秘密を持つのは Go 側だけなので、
外殻は自分では叩けない。

```go
// runStatus は、エンジンの様子をそのまま JSON で書き出す。
//
// **これは人のための表示ではない。** 読むのはメニューバーであり、だから
// 整形もしないし、翻訳もしない。エンジンが居なければ 1 で終わる。
func runStatus(
	ctx context.Context, stateDir string, client *http.Client, stdout, stderr io.Writer,
) int {
	answer, err := engineStatus(ctx, stateDir, client)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	encoded, err := json.Marshal(answer)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(encoded))
	return 0
}
```

分岐は既存のものに倣って `case "status":` を足す。**usage にも 1 行足す**
——`sshc -h` に出ないサブコマンドを作らない。

- [ ] **Step 5: 外殻を差し替える**

`desktop/main.js` の Task 4 Step 3 に置いた仮の `status` を
`async () => JSON.parse(await run(["status"]))` にする。`liveSessions()` も
同じ経路で数える。

`--hidden` を読む:

```js
// **窓を作らずに上がる。** 端末から起こされたときの姿である。
const hidden = process.argv.includes("--hidden");
```

`whenReady` の中で `if (!hidden) openWindow(url);` とする。

- [ ] **Step 6: 手で確かめる**

Run:
```bash
make desktop-dist   # 束を作る（LaunchServices に登録される）
# アプリを終了してから
sshc <保存済みパスワードのある接続先>
```
Expected: メニューバーに項目が出て、端末がマスターパスワードを聞き、繋がる。
そのあとメニューバーから窓を開くと**解錠済み**である。

- [ ] **Step 7: commit**

```bash
git add cmd/sshc/launch_darwin.go cmd/sshc/launch_other.go cmd/sshc/connect.go cmd/sshc/main.go desktop/main.js
git commit -m "feat: 端末からアプリを起こして解錠できるようにする"
```

---

### Task 7: ログイン項目を消す

**Files:**
- Delete: `internal/platform/macos/loginitem.go`, `internal/platform/macos/loginitem_test.go`
- Delete: `internal/platform/linux/loginitem.go`, `internal/platform/linux/loginitem_test.go`
- Delete: `internal/httpserver/loginitem.go`（と対応する検査）
- Delete: `cmd/sshc/service.go`, `cmd/sshc/service_darwin.go`, `cmd/sshc/service_linux.go`, `cmd/sshc/service_test.go`, `cmd/sshc/service_linux_test.go`
- Modify: `internal/httpserver/server.go`（`LoginItem` の option と登録）
- Modify: `internal/app/run.go`（`LoginItem` の依存）
- Modify: `cmd/sshc/main.go`, `cmd/sshc/wiring*.go`（`parts.LoginItem`）
- Modify: `api/openapi.yaml`（`/api/v1/login-item` を消す）
- Modify: `web/src/settings/SettingsPanel.tsx`（と `.test.tsx`）
- Modify: `web/src/api/integrations.ts`, `web/src/i18n/messages/{ja,en}.ts`
- Modify: `Makefile`（`install-binary` の `service refresh`）

- [ ] **Step 1: Go 側を消す**

上の Delete を消し、Modify から参照を外す。`go build ./...` が通るまで。

- [ ] **Step 2: 生成物を作り直す**

Run: `make generate`
Expected: `internal/api/models.gen.go` と `web/src/api/schema.d.ts` から
login-item が消える

- [ ] **Step 3: 画面から消す**

`SettingsPanel.tsx` からトグルを消す。i18n から `settings.loginItem*` と
`desktop.loginItemWins` を消す——**あの文言は二人の持ち主が居たことの証拠**
であり、持ち主がひとつになったら意味が無い。

- [ ] **Step 4: 通す**

Run: `go build ./... && go test ./... && npm test --prefix web && npx tsc --noEmit --project web`
Expected: PASS

- [ ] **Step 5: commit**

```bash
git add -A
git commit -m "refactor: ログイン項目を OS に任せて、書く仕組みを消す"
```

---

### Task 8: engine サブコマンド・detach・KeepRunning を消し、後始末する

**Files:**
- Delete: `cmd/sshc/engine.go`, `cmd/sshc/engine_test.go`, `cmd/sshc/detach_unix.go`, `cmd/sshc/detach_other.go`
- Delete: `internal/application/desktop.go`（と対応する検査）
- Modify: `internal/application/metadata.go`（`Desktop` と `KeepEngineRunning`）
- Modify: `internal/httpserver/config_handlers.go`（`SetKeepEngineRunning` の handler）
- Modify: `api/openapi.yaml`（desktop 設定の schema と route）
- Modify: `web/src/settings/SettingsPanel.tsx`, `web/src/api/integrations.ts`, `web/src/i18n/messages/{ja,en}.ts`
- Modify: `Makefile`（`desktop-run` の `engine stop`）
- Modify: `README.md`
- Modify: `desktop/main.js`（`run(["engine", ...])` の残りが無いこと）

- [ ] **Step 1: 消す**

`go build ./...` が通るまで参照を外す。**`sshc open` は残す。**

- [ ] **Step 2: 生成物を作り直す**

Run: `make generate`

- [ ] **Step 3: README を書き換える**

書き換える節：ログイン時起動（丸ごと消える）、`sshc <接続先>`（アプリを隠しで
起こす・端末で解錠できる）、埋め込みターミナル（「閉じても動かし続ける」設定が
消える）、そして**メニューバーに常駐すること**を新しく書く。

- [ ] **Step 4: Makefile を整える**

`desktop-run` から `engine stop` を消す。**止める物がもう無い。**

- [ ] **Step 5: 開発機の後始末**

```bash
launchctl bootout gui/$(id -u)/com.github.aida0710.sshc 2>/dev/null || true
rm -f ~/Library/LaunchAgents/com.github.aida0710.sshc.plist
pkill -f "sshc -open=false" || true
```

**移行のコードは書かない。** 配布前なので、置き去りになる plist はこの 1 台
だけである。

- [ ] **Step 6: 通す**

Run: `make test`
Expected: PASS

- [ ] **Step 7: 手で確かめる**

Run: `make desktop-run` → 窓を閉じる → メニューバーから開き直す → 終了
Expected: `pgrep -f "sshc -open"` が空。ログインし直してもエンジンは上がらない

- [ ] **Step 8: commit**

```bash
git add -A
git commit -m "refactor: エンジンはアプリの子だけになる"
```
