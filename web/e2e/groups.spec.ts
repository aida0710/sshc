import { clickAndAwait, expect, openSection, test, openApplication } from "./support/environment";

// グループはディレクトリであり、この試験はそこから従う 2 つの事実についてのものだ。エ
// ントリファイルはグループごとに明示された順序で 1 行の Include を得ること、そして接続
// をグループ間で移動するとそのファイルも移動すること——両方の検証はディスク上のバイ
// ト列を読む。「移動した」という画面の表示は ~/.ssh について何も証明しないからだ。
test("declares a group in the entry file and moves a connection into it", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Groups");

  await page.getByLabel("New group name").fill("work");
  await page.getByRole("button", { name: "Add group" }).click();
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  // 宣言は 2 つのマーカーコメントに挟まれた普通の Include 行
  // であり、このアプリケーションを知らない読者にも何であるか分かる。
  const entry = await installation.read("config");
  expect(entry).toContain("# >>> sshc groups (generated).");
  expect(entry).toContain("Include connections/work/*.conf\n");
  expect(entry).toContain("Include groups.sshc.conf\n");
  expect(entry).toContain("# <<< sshc groups");
  // この領域はすべての Host 行より上にある。Host 行の下に書かれた
  // Include はそのブロックに属してしまい、OpenSSH はブロックが一致する
  // ときにしか include したファイルのオプションを適用しない。下のほうに
  // 書くと、グループを 1 つのホストにしか宣言できなくなる。
  expect(entry.indexOf("# >>> sshc groups")).toBeLessThan(entry.indexOf("Host "));

  await openSection(page, "Connections");
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "nas" })
    .click();
  await expect(page.getByRole("tablist", { name: "Connection editor" })).toBeVisible();

  await page.getByRole("button", { name: "More connection actions" }).click();
  await page.getByLabel("Primary group").selectOption("work");
  expect(await clickAndAwait(page, "Move to this group", "/api/v1/config/save")).toBe(200);

  // ファイルは移動しても自分の名前を保ち、ブロックはバイト単位でそのまま届いた。
  expect(await installation.read("connections/work/10-home.conf")).toContain("Host nas");
  expect(await installation.read("conf.d/10-home.conf")).toBe("");
});

// connections/work/*.conf は connections/work/eu/lon.conf
// に届かない。'*' は glob(3) でも filepath.Glob でも
// セパレータを越えないからだ。これこそがこの領域がグループ
// ごとに 1 行を出す理由のすべてであり、前提とせずここで検証する。
test("gives a nested group its own Include line, deepest first", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Groups");

  for (const name of ["work", "work/eu"]) {
    await page.getByLabel("New group name").fill(name);
    await page.getByRole("button", { name: "Add group" }).click();
  }
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  const entry = await installation.read("config");
  const child = entry.indexOf("Include connections/work/eu/*.conf");
  const parent = entry.indexOf("Include connections/work/*.conf");
  expect(child).toBeGreaterThan(-1);
  expect(parent).toBeGreaterThan(-1);
  // OpenSSH は最初に読んだ値を保持するため、より深い
  // グループを先に読まなければ、親の設定が子自身の設定に勝ってしまう。
  expect(child).toBeLessThan(parent);
});

test("renames a group and carries its files, its Include line and its keys", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Groups");

  await page.getByLabel("New group name").fill("work");
  await page.getByRole("button", { name: "Add group" }).click();
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Connections");
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "nas" })
    .click();
  await page.getByRole("button", { name: "More connection actions" }).click();
  await page.getByLabel("Primary group").selectOption("work");
  expect(await clickAndAwait(page, "Move to this group", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Groups");
  // リネームはファイルを書き換えるため、一度にすべての
  // グループに対してではなく、選択したグループに対してのみ提供される。
  await page.getByRole("heading", { name: "work", exact: true }).click();
  await page.getByLabel("Rename work to").fill("client-a");
  expect(await clickAndAwait(page, "Rename work", "/api/v1/config/groups/rename")).toBe(200);

  // ファイル、宣言、空になった元のディレクトリ——この 3 つ
  // すべてが、グループがディレクトリであるときのリネームの意味だ。
  expect(await installation.read("connections/client-a/10-home.conf")).toContain("Host nas");
  const entry = await installation.read("config");
  expect(entry).toContain("Include connections/client-a/*.conf\n");
  expect(entry).not.toContain("Include connections/work/*.conf\n");
});

test("refuses to move a connection into a group nothing declares", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "nas" })
    .click();
  await expect(page.getByRole("tablist", { name: "Connection editor" })).toBeVisible();

  // グループが 1 つも宣言されていないため、移動先リストは
  // 空でコントロールは無効化される。失敗すると分かっている移動を、画面は提供しない。
  await page.getByRole("button", { name: "More connection actions" }).click();
  await expect(page.getByRole("button", { name: "Move to this group" })).toBeDisabled();
  expect(await installation.read("conf.d/10-home.conf")).toContain("Host nas");
});

test("quick connect drills into a nested group and promotes it when its container is hidden", async ({
  page,
  installation,
}) => {
  // 一覧の読み取りはナビゲーションがどの画面でも行う。数えるのは、閲覧が
  // 端末を起こしていないことを言うための、起こす側の要求だけである。
  const terminalRequests: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (!path.startsWith("/api/v1/terminal/")) return;
    if (request.method() === "GET") return;
    terminalRequests.push(`${request.method()} ${path}`);
  });
  await openApplication(page, installation);
  await openSection(page, "Groups");
  for (const name of ["work", "work/eu"]) {
    await page.getByLabel("New group name").fill(name);
    await page.getByRole("button", { name: "Add group" }).click();
  }
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Home");
  let browser = page.getByRole("region", { name: "Quick connect" });
  let modes = browser.getByRole("group", { name: "Browse connections by" });
  await expect(modes.getByRole("button", { name: "Servers", exact: true })).toHaveAttribute("aria-pressed", "true");
  await modes.getByRole("button", { name: "Groups", exact: true }).click();
  expect(new URL(page.url()).pathname).toBe("/");

  // 一度に一階層だけを表示する。親を選ぶまでは子を出さず、選ぶと
  // 同じ左ペインを子グループへ置き換える。
  await expect(browser.getByRole("button", { name: "work, 0 servers" })).toBeVisible();
  await expect(browser.getByRole("button", { name: "eu, 0 servers" })).toHaveCount(0);
  await browser.getByRole("button", { name: "work, 0 servers" }).click();
  expect(new URL(page.url()).pathname).toBe("/");
  await expect(browser.getByRole("button", { name: "eu, 0 servers" })).toBeVisible();
  await browser.getByRole("button", { name: "eu, 0 servers" }).click();
  expect(new URL(page.url()).pathname).toBe("/");
  await expect(
    browser.getByRole("navigation", { name: "Group path" }).getByText("eu", { exact: true }),
  ).toHaveAttribute("aria-current", "page");
  expect(terminalRequests).toEqual([]);

  // クイック接続内の閲覧位置は一時的で、再読込後は既定のサーバー一覧へ戻る。
  await page.reload();
  browser = page.getByRole("region", { name: "Quick connect" });
  modes = browser.getByRole("group", { name: "Browse connections by" });
  await expect(modes.getByRole("button", { name: "Servers", exact: true })).toHaveAttribute("aria-pressed", "true");

  // "work" は自分自身のものを何も持たないため、隠す操作が提供される。
  //
  // 非表示は metadata.json にしか存在しない 3 つの設定の
  // 1 つであり——色や表示順とともに——インスペクタに置かれる。
  // グループを選択するとペインが埋まる。ペインは求められるまで閉じている。
  await openSection(page, "Groups");
  await page.getByRole("listitem").filter({ hasText: "work" }).first().click();
  await page.getByRole("button", { name: "Show Group display settings" }).click();
  //
  // `check()` ではなくクリックしてから検証する。ペインの
  // 内容はパネルが組み立ててシェルへ渡すため、新しい状態は
  // クリックから 1 描画分遅れて届く——`check()` は同期的に
  // 検証するため、それを「変化しないチェックボックス」だと判定してしまう。
  const hide = page.getByLabel("Hide work from Connections");
  await hide.click();
  await expect(hide).toBeChecked();
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Home");
  browser = page.getByRole("region", { name: "Quick connect" });
  modes = browser.getByRole("group", { name: "Browse connections by" });
  await modes.getByRole("button", { name: "Groups", exact: true }).click();
  await expect(browser.getByRole("button", { name: "work, 0 servers" })).toHaveCount(0);
  // 子は親が消えてもルートへ昇格する。
  const promoted = browser.getByRole("button", { name: "eu, 0 servers" });
  await expect(promoted).toBeVisible();
  await promoted.click();
  expect(new URL(page.url()).pathname).toBe("/");
});
