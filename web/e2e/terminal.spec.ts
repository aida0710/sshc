import { expect, openApplication, openSection, test } from "./support/environment";

// 埋め込みターミナルの end-to-end。
//
// ここが駆動するのは実バイナリであり、開かれるのは本物の PTY である。ローカル
// シェルは一時 HOME の中で完結するので、このスイートはリモートホストへ一度も
// 触れない。
//
// CSP は緩めていない。xterm.js の配布物には innerHTML も document.write も
// new Function も無いので、`require-trusted-types-for 'script'` に触れる経路が
// そもそも存在しない。それを毎回確かめるために、どの spec も違反を監視する。
function watchForPolicyViolations(page: import("@playwright/test").Page): string[] {
  const violations: string[] = [];
  page.on("console", (message) => {
    const text = message.text();
    if (/Content Security Policy|Trusted Type/i.test(text)) violations.push(text);
  });
  page.on("pageerror", (error) => {
    if (/Trusted Type|Content Security Policy/i.test(error.message)) violations.push(error.message);
  });
  return violations;
}

// 打鍵は xterm の隠しテキストエリアへ届かなければならない。パネルのボタンを
// 押した直後は焦点がそこにあるので、まず端末へ焦点を移し、シェルがプロンプトを
// 描くのを待ってから打つ。
async function typeIntoConsole(page: import("@playwright/test").Page, line: string) {
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();
  // プロンプトが出るまでは、打った文字はどこにも解釈されない。
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type(line);
  await page.keyboard.press("Enter");
  return screen;
}

// 一覧は一番左のナビゲーションにある。セクションに属さないので、どの画面から
// でも同じ場所にあり、開いてある一本を選べばそこへ連れて行かれる。
async function openConsolePanel(page: import("@playwright/test").Page) {
  const nav = page.getByRole("navigation", { name: "Primary" });
  await nav.getByRole("tab", { name: "Terminals" }).click();
  await expect(nav.getByRole("button", { name: "Local shell" })).toBeVisible();
  return nav;
}

test("opens a local shell, runs a command and shows its output", async ({ page, installation }) => {
  const violations = watchForPolicyViolations(page);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();

  // 端末は主画面に出る。xterm.js が描いた行がそこに現れる。
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();

  // 打鍵はそのまま PTY へ渡る。シェルはプロンプトを描いてから答える。
  await typeIntoConsole(page, "echo embedded-terminal-canary");

  await expect(screen).toContainText("embedded-terminal-canary", { timeout: 20_000 });
  expect(violations).toEqual([]);
});

test("keeps the session and replays its scrollback after a reload", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();

  const screen = await typeIntoConsole(page, "echo survives-a-reload");
  await expect(screen).toContainText("survives-a-reload", { timeout: 20_000 });

  // PTY は常駐プロセス側で存続する。タブを閉じてもリロードしてもセッションは
  // 生きており、繋ぎ直すとスクロールバックが先に再生される。
  await page.reload();
  await expect(page.getByRole("heading", { name: "sshc" })).toBeVisible();
  // 行の名前はログインシェルの basename なので、環境によって違う。開いている
  // のは一本だけなので、名前ではなく一覧の先頭を選ぶ。
  const reopened = await openConsolePanel(page);
  const row = reopened.getByRole("list", { name: "Open consoles" }).getByRole("listitem").first();
  await expect(row).toBeVisible();
  await row.getByRole("button").first().click();

  await expect(page.getByRole("region", { name: /^Console for / }))
    .toContainText("survives-a-reload", { timeout: 20_000 });
});

test("refuses to open more consoles than the configured limit", async ({ page, installation }) => {
  // 上限は metadata が運ぶ。2 本まで開ける状態にしてから、3 本目が拒否される
  // ことを見る。黙って古いセッションを閉じることはしない。
  await installation.write(
    "sshc/metadata.json",
    JSON.stringify({ schemaVersion: 3, embeddedTerminal: { maxSessions: 2, scrollbackBytes: 16384 } }),
  );
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  const openShell = panel.getByRole("button", { name: "Local shell" });

  const rows = panel.getByRole("list", { name: "Open consoles" }).getByRole("listitem");
  await openShell.click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();
  await expect(rows).toHaveCount(1);
  await openShell.click();
  // 二本目が一覧に載ってから数える。載る前に上限を問うと、まだ一本しか
  // 無い状態を見て「上限に達していない」と答えてしまう。
  await expect(rows).toHaveCount(2);

  // 二本開いた時点で入口は閉じ、その理由が書かれる。
  await expect(openShell).toBeDisabled();
  await expect(panel).toContainText("limit of 2 open consoles");
});

// リロードのあとも、開いているものが画面に出る。
//
// **どれを見ていたかはこのプロセスの記憶であり、読み込み直せば消える。**
// セッションの方は常駐プロセス側で生きているので、一覧には並んだままになる。
// 選択が空のままだと、Terminal の画面は「開いているコンソールがありません」と
// 言う——**一覧に何本も並んでいる隣で。** 選ばれていないなら、どれかを選ぶ。
test("shows an open console again after a reload instead of claiming there are none", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();

  await page.reload();
  await expect(page.getByRole("heading", { name: "sshc" })).toBeVisible();

  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();
  await expect(page.getByText("No console is open")).toBeHidden();
});

// 上限も画面から変えられる。
//
// **metadata が書けたことではなく、その数字が端末に効くことを見る。** 間に
// 居るのはエンジンであり、そこを通らないと「保存できた」だけで終わる。
test("applies the session limit set from the settings screen", async ({ page, installation }) => {
  await openApplication(page, installation);

  await openSection(page, "Settings");
  const region = page.getByRole("region", { name: "Terminal" });
  await region.getByLabel("Consoles open at once").fill("1");
  await region.getByRole("button", { name: "Save" }).click();
  await expect(region.getByText(/Saved/)).toBeVisible();

  const panel = await openConsolePanel(page);
  const openShell = panel.getByRole("button", { name: "Local shell" });
  await openShell.click();
  await expect(panel.getByRole("list", { name: "Open consoles" }).getByRole("listitem")).toHaveCount(1);

  // 1 本で上限である。入口が閉じ、その理由が書かれる。
  await expect(openShell).toBeDisabled();
  await expect(panel).toContainText("limit of 1 open consoles");
});

// 開始位置は設定が決める。
//
// **`pwd` がその証拠である。** 設定画面が metadata を書けたことではなく、
// 次に開いたシェルがそこに立っていることを見る——間に居るのはエンジンで
// あり、そこを通らないと「保存できた」だけで終わる。
test("starts local shells where the setting says", async ({ page, installation }) => {
  await installation.write("../workspace/marker", "");
  await openApplication(page, installation);

  await openSection(page, "Settings");
  const region = page.getByRole("region", { name: "Terminal" });
  await region.getByLabel("Starting directory").fill("~/workspace");
  await region.getByRole("button", { name: "Save" }).click();
  await expect(region.getByText(/Saved/)).toBeVisible();

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, "pwd");
  await expect(screen).toContainText("/workspace", { timeout: 20_000 });
});

// 選び終えた範囲が、追加のショートカットなしでクリップボードへ入る。
//
// **xterm の選択はブラウザの選択ではない。** 選んだ範囲を知っているのは
// xterm だけなので、Cmd+C をそのまま渡すとブラウザは空の選択を写して何も
// 起きない——実際そうなっていた。ここが見ているのは、写る中身そのものである。
test("copies what was selected in the console as soon as selection finishes", async ({ page, context, installation }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, "echo selectable-canary");
  await expect(screen).toContainText("selectable-canary", { timeout: 20_000 });

  // 端末いっぱいを引きずる。行の高さも桁の幅も環境で違うので、座標では
  // なく端から端まで動かす。
  const rows = page.locator(".xterm-rows");
  const box = await rows.boundingBox();
  expect(box).not.toBeNull();
  if (box === null) return;
  await page.mouse.move(box.x + 2, box.y + 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 2, box.y + box.height - 2, { steps: 12 });
  await page.mouse.up();
  await expect(page.locator(".xterm-selection div").first()).toBeAttached();
  await expect
    .poll(async () => page.evaluate(() => navigator.clipboard.readText()))
    .toContain("selectable-canary");
});

// 右クリックはブラウザへ生の文字列を流さず、xterm の貼り付け経路を使う。
// シェルへ届いて実行できることが、DOM イベントを消しただけではない証拠になる。
test("pastes the clipboard into the console with right click", async ({ page, context, installation }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await page.evaluate(() => navigator.clipboard.writeText("echo right-click-paste-canary"));

  await page.locator(".xterm-screen").click({ button: "right", position: { x: 20, y: 20 } });
  await page.keyboard.press("Enter");

  await expect(screen).toContainText("right-click-paste-canary", { timeout: 20_000 });
});

// 設定は保存後に、既に開いている端末にも効く。端末を作り直して確認すると
// 「次回だけ効く」退行を見逃すので、同じセッションを隠して戻す。
test("can turn automatic selection copy off for an already open console", async ({
  page,
  context,
  installation,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, "echo copy-setting-canary");
  await expect(screen).toContainText("copy-setting-canary", { timeout: 20_000 });

  await panel.getByRole("tab", { name: "Settings" }).click();
  await openSection(page, "Settings");
  const settings = page.getByRole("region", { name: "Terminal" });
  await settings.getByRole("checkbox", { name: "Copy selected text automatically" }).uncheck();
  await settings.getByRole("button", { name: "Save" }).click();
  await expect(settings.getByText(/Saved/)).toBeVisible();

  await openSection(page, "Terminal");
  await page.evaluate(() => navigator.clipboard.writeText("clipboard-sentinel"));
  const rows = page.locator(".xterm-rows");
  const box = await rows.boundingBox();
  expect(box).not.toBeNull();
  if (box === null) return;
  await page.mouse.move(box.x + 2, box.y + 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 2, box.y + box.height - 2, { steps: 12 });
  await page.mouse.up();
  await expect(page.locator(".xterm-selection div").first()).toBeAttached();

  await expect.poll(async () => page.evaluate(() => navigator.clipboard.readText())).toBe("clipboard-sentinel");
});

// 端末は枠に収まる。
//
// **高さの指定が無い flex の列は内容の高さまで伸びる。** 端末が親を突き抜けると
// はみ出した分は切られて見えなくなり、そこは選ぶことも読むこともできない
// ——1561px の端末が 666px の枠に入っていた。向こうのシェルはその行数を
// 信じるので、全画面を使うプログラムは画面外へ描く。
test("fits the terminal inside the space it was given", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();

  const measured = await page.locator(".xterm-rows").evaluate((node) => {
    const frame = (node as HTMLElement).closest("main");
    return {
      rows: Math.round((node as HTMLElement).getBoundingClientRect().height),
      frame: Math.round(frame?.getBoundingClientRect().height ?? 0),
    };
  });
  expect(measured.frame).toBeGreaterThan(0);
  expect(measured.rows).toBeLessThanOrEqual(measured.frame);
});

// ナビゲーションの上半分は動かない。
//
// **出口の位置が行の数で変わってはいけない。** コンソールが増えると一覧は
// 溢れるが、溢れるのは一覧だけであり、Start と面のトグルはその場に残る。
// ここが見ているのは、一覧をスクロールさせても「接続」が同じ位置に居ること
// である——ナビゲーション全体がスクロールしていたら、これは動く。
test("keeps the top of the navigation still while the console list scrolls", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  const anchor = panel.getByRole("link", { name: "Connections", exact: true });
  const before = await anchor.boundingBox();

  // 一覧が確実に溢れる高さにしてから、行を積む。
  await page.setViewportSize({ width: 1280, height: 400 });
  const rows = panel.getByRole("list", { name: "Open consoles" }).getByRole("listitem");
  for (let opened = 1; opened <= 6; opened += 1) {
    await panel.getByRole("button", { name: "Local shell" }).click();
    await expect(rows).toHaveCount(opened);
  }

  const scroller = page.locator("nav[aria-label='Primary'] div.overflow-y-auto");
  await expect(async () => {
    expect(await scroller.evaluate((node) => node.scrollHeight - node.clientHeight)).toBeGreaterThan(0);
  }).toPass();
  await scroller.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });

  const after = await anchor.boundingBox();
  expect(after?.y).toBe(before?.y);
});

// 繋ぎっぱなしをまとめて片付ける。
//
// **設定画面から押すと、サーバー側のセッションが本当に終わる。** 一覧が空に
// なることを見るのはそのためであり、画面の状態だけを見ているのではない——
// 一覧はサーバーが返したものである。
test("closes every open connection from the settings screen", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  const rows = panel.getByRole("list", { name: "Open consoles" }).getByRole("listitem");
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(rows).toHaveCount(1);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(rows).toHaveCount(2);

  // ナビゲーションの下半分はいまターミナルの面である。セクションの一覧は
  // もう片方の面にあるので、戻してから行く。
  await panel.getByRole("tab", { name: "Settings" }).click();
  await openSection(page, "Settings");
  const region = page.getByRole("region", { name: "Open connections" });
  await expect(region.getByText("2 open")).toBeVisible();
  await region.getByRole("button", { name: "Close every connection" }).click();

  await expect(region.getByText("0 open")).toBeVisible();
  await openConsolePanel(page);
  await expect(rows).toHaveCount(0);
});

// 端末は一画面である。接続の一覧の隣ではない。
//
// **かつては右のカラムを端末が奪っていた。** そのため接続を開いているあいだ、
// 接続先の詳細を読む場所が無くなっていた。ここが見ているのはその回帰であり、
// 「端末が見える」だけでは足りない——**接続画面へ戻ったときに詳細が居ること**
// までを見る。
test("moves to its own screen and leaves the connection detail alone", async ({ page, installation }) => {
  await installation.write("conf.d/20-detail.conf", ["Host detail-host", "\tHostName 127.0.0.1", ""].join("\n"));
  await openApplication(page, installation);

  await openSection(page, "Connections");
  const tree = page.getByRole("navigation", { name: "Connections" });
  await tree.getByRole("button", { name: "detail-host" }).click();
  const detail = page.getByRole("heading", { name: "detail-host" });
  await expect(detail).toBeVisible();

  await page.getByRole("button", { name: "Connect", exact: true }).click();

  // 端末は自分の画面へ連れて行く。接続の一覧はもうそこに無い。
  await expect(page).toHaveURL(/\/terminal$/);
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();
  await expect(tree).toBeHidden();

  // 戻れば詳細は元のまま居る——**端末はもうそこを覆っていない。**
  // 戻り方が履歴なのは、選ばれているホストが URL に載っているからである。
  // ナビゲーションのリンクは常に一覧の入口を指すので、そちらは選択を持たない。
  await page.goBack();
  await expect(detail).toBeVisible();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeHidden();
});

// 接続できなかった理由は端末に残る。
//
// **このスイートは OpenSSH を一度も起動しない。** それでもこの検査が成り立つ
// のは、SSH をプロセス内で話すようになったからである——接続を試みるのはこの
// バイナリ自身であり、拒否されたのは即座に返る 127.0.0.1 のポートである。
test("shows why a connection failed in the console itself", async ({ page, installation }) => {
  await installation.write(
    "conf.d/20-refused.conf",
    ["Host refused", "\tHostName 127.0.0.1", "\tPort 1", "\tConnectTimeout 2", ""].join("\n"),
  );
  await openApplication(page, installation);

  await openSection(page, "Connections");
  const nav = page.getByRole("navigation", { name: "Connections" });
  await nav.getByRole("button", { name: "refused" }).click();
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  // セッションは作られる。理由が読める場所がそこだけだからである。
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();
  await expect(screen).toContainText(/sshc:.*(refused|connect)/i, { timeout: 20_000 });
});

// 端末の大きさは、繋いだ直後に PTY へ届く。
//
// **その機会は一度きりである。** WebSocket は new した直後まだ CONNECTING で
// あり、そこで送ろうとしたフレームは落ちる。落ちれば PTY は既定の 80×24 の
// まま残り、次に人が窓の大きさを変えるまで直らない——折り返しも、全画面を使う
// プログラムも、その幅を信じて描く。ここが見ているのは、xterm が描いている
// 行数と、向こうの `stty size` が答える行数が同じであることである。
test("tells the pseudo-terminal how big it is as soon as it attaches", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  // 区切りを "-" にするのは、打った行そのものと、その出力とを見分けるためである。
  const screen = await typeIntoConsole(page, 'stty size | tr " " "-"');
  await expect(screen).toContainText(/\d+-\d+/, { timeout: 20_000 });

  const reported = (await screen.innerText()).match(/(\d+)-(\d+)/);
  expect(reported).not.toBeNull();
  const drawn = await page.locator(".xterm-rows").evaluate((node) => node.children.length);
  expect(Number(reported?.[1])).toBe(drawn);
  // 既定の 80 桁のままではない。この窓はそれより広い。
  expect(Number(reported?.[2])).toBeGreaterThan(80);
});

// 端末は、別の画面を見ているあいだも生きている。
//
// **外せば xterm ごと捨てることになる。** 戻ったときに読めるのはサーバー側の
// リングバッファの再生だけであり、あれは途中から始まるバイト列なので、
// alt-screen を使っているもの（vim、top）は崩れた姿で戻ってくる。ここが見て
// いるのは、戻ってきた端末が同じ端末であることそのものである。
test("keeps the same terminal alive while another screen is shown", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, "echo stays-mounted");
  await expect(screen).toContainText("stays-mounted", { timeout: 20_000 });

  // 作り直されたら消える印を、いまの xterm に付ける。
  await page.locator(".xterm").first().evaluate((node) => node.setAttribute("data-e2e-mark", "1"));
  // 隠れているあいだにも出力は届く。受け取るのは同じ端末である。
  //
  // **打った行そのものには現れない文字列でなければならない。** 打鍵は
  // そのまま画面へ写るので、`echo late-canary` と書けば「late-canary」は
  // 出力を待たずにそこにある——それを待っても何も確かめたことにならない。
  await typeIntoConsole(page, "(sleep 2; echo late-$((6*7))) &");

  await page.getByRole("navigation", { name: "Primary" }).getByRole("tab", { name: "Settings" }).click();
  await openSection(page, "Settings");
  // 隠れてはいるが、居なくなってはいない。
  await expect(screen).toBeHidden();

  const reopened = await openConsolePanel(page);
  await reopened
    .getByRole("list", { name: "Open consoles" })
    .getByRole("listitem")
    .first()
    .getByRole("button")
    .first()
    .click();

  await expect(screen).toBeVisible();
  await expect(page.locator(".xterm[data-e2e-mark='1']")).toBeAttached();
  await expect(screen).toContainText("stays-mounted");
  await expect(screen).toContainText("late-42", { timeout: 20_000 });
});

// 端末は、それを起こしたものの事情を継がない。
//
// **常駐プロセスの環境は、それを起こしたものがたまたま持っていたものである。**
// npm run から起こせば npm の設定がそこに入る——`npm_config_prefix` は npm へ
// 渡した `--prefix` の写しであり、それを継いだシェルの中で nvm は「知らない
// prefix だ」と警告する。開始ディレクトリと同じ話であり、利用者はそのどれも
// 選んでいない。
test("does not hand npm's own environment to the shell", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  // 打った行そのものには "prefix=[]" は現れない。継いでいれば括弧の中に出る。
  const screen = await typeIntoConsole(page, 'echo "prefix=[${npm_config_prefix}]"');

  await expect(screen).toContainText("prefix=[]", { timeout: 20_000 });
});
