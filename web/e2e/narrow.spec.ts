import { expect, test, openApplication, sessionStatus } from "./support/environment";

// このファイルだけが 360x800 で走る。**既存の spec を狭い幅でもう一周させない**
// ——17 本すべてをドロワー越しの操作へ書き換えることになり、守るものより
// 書き換える量の方が多い。ここで見るのは、狭い幅でしか壊れない 4 つである。
//
// 360x800 は、いま売られている最も狭い Android である。ここで成立すれば、
// それより広いすべてで成立する。

const hosts = "Host alpha\n\tHostName 198.51.100.10\n\nHost bravo\n\tHostName 198.51.100.11\n";

// ドロワーを開いてから遷移する。狭い画面ではナビが面の外に居るので、
// 広い画面の openSection はそのままでは使えない。
async function openSectionThroughDrawer(page: import("@playwright/test").Page, name: string) {
  await expect(sessionStatus(page)).toContainText("Local session active");
  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  await page
    .getByRole("navigation", { name: "Primary" })
    .getByRole("link", { name, exact: true })
    .click();
}

// 横スクロールは、狭い画面の壊れ方そのものである。**どれか一つの面が溢れれば
// ドキュメント全体が溢れる**ので、面を渡り歩きながら同じことを一度ずつ問う。
async function expectNoHorizontalOverflow(page: import("@playwright/test").Page, where: string) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow, `${where} scrolls sideways at 360px`).toBeLessThanOrEqual(0);
}

test("keeps every section inside 360 pixels", async ({ page, installation }) => {
  await installation.write("conf.d/20-lab.conf", hosts);
  await openApplication(page, installation);
  await expectNoHorizontalOverflow(page, "Home");

  // 表示名で選ぶ。セクションの内部名とは違うものがある——Diagnostics は
  // 画面では "Ad hoc checks" である。
  for (const section of ["Connections", "Keys", "Known Hosts", "Ad hoc checks", "Settings"]) {
    await openSectionThroughDrawer(page, section);
    await expectNoHorizontalOverflow(page, section);
  }
});

test("navigates through the drawer and closes it behind itself", async ({ page, installation }) => {
  await openApplication(page, installation);

  const drawer = page.getByRole("navigation", { name: "Primary" });
  const hamburger = page.getByRole("button", { name: "Navigation", exact: true });

  // 閉じているあいだ、ドロワーは面の外に居る。**hidden ではなく外に居る**ので、
  // 見えるかどうかではなく、どこに居るかを問う。
  const restingLeft = await drawer.evaluate((element) => element.getBoundingClientRect().left);
  expect(restingLeft).toBeLessThan(0);

  await hamburger.click();
  await expect(hamburger).toHaveAttribute("aria-expanded", "true");
  await drawer.getByRole("link", { name: "Keys", exact: true }).click();

  await expect(page.getByRole("heading", { name: "Keys", exact: true })).toBeVisible();
  // **遷移したら畳む。** 開いたままだと、選んだ先が自分の後ろに隠れる。
  await expect(hamburger).toHaveAttribute("aria-expanded", "false");
  await expect
    .poll(() => drawer.evaluate((element) => element.getBoundingClientRect().left))
    .toBeLessThan(0);
});

test("lets the connection detail replace the list and hands back a way out", async ({
  page,
  installation,
}) => {
  await installation.write("conf.d/20-lab.conf", hosts);
  await openApplication(page, installation);
  await openSectionThroughDrawer(page, "Connections");

  const tree = page.getByRole("navigation", { name: "Connections" });
  await expect(tree.getByRole("button", { name: "alpha" })).toBeVisible();

  await tree.getByRole("button", { name: "alpha" }).click();

  // 詳細が一覧を置き換える。**両方は入らないので、一方は消える。**
  await expect(tree).toBeHidden();
  const back = page.getByRole("button", { name: "All connections" });
  await expect(back).toBeVisible();

  await back.click();
  await expect(tree).toBeVisible();
});

test("sends a real control character from the on-screen keys", async ({ page, installation }) => {
  await openApplication(page, installation);

  // コンソールの一覧はナビゲーションの中にある。狭い画面ではドロワー越しになる。
  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  const nav = page.getByRole("navigation", { name: "Primary" });
  await nav.getByRole("tab", { name: "Terminals" }).click();
  await nav.getByRole("button", { name: "Local shell" }).click();

  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });

  // 止められるものを走らせる。**Ctrl+C が 0x03 にならなければ、シェルは 60 秒
  // 戻ってこない。** 送ったバイトを覗くのではなく、シェルがまた打鍵を受ける
  // ようになることで表明する。
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type("sleep 60");
  await page.keyboard.press("Enter");

  // **Ctrl はバーで、c はソフトキーボードで。** バーの上に英字キーは無いので、
  // これが実際に起きる順序である。修飾がここに乗らなければ、触れる画面から
  // 走っているものを止める手段は無い。
  const keys = page.getByLabel("On-screen keys");
  await expect(keys).toBeVisible();
  await keys.getByRole("button", { name: "Ctrl", exact: true }).click();
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type("c");

  // 引用符は、打った行と出た行を別物にするために入れてある。これが無いと、
  // エコーされたコマンド行そのものに一致して、走ったかどうかを何も言わない。
  await page.keyboard.type('echo the-shell-"came"-back');
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("the-shell-came-back", { timeout: 20_000 });

  // **修飾を伴わないキーも、ラベルではなく制御列を送る。** この spec は
  // Ctrl+c しか試しておらず、そちらは打たれた文字の道を通るので、バーのキーが
  // ラベルの文字列をそのまま送っていた不具合を素通りさせた。実機では Esc を
  // 押すと端末に "Esc" と出ていた。
  //
  // **「ラベルが出ていないこと」では表明にならない。** 否定は、エコーが届く
  // 前に評価されればその場で通る——不具合を入れ直しても素通りした。だから
  // 「キーが効いたときにしか起きないこと」を待つ。
  //
  // ↑ は履歴を 1 つ戻す。同じ印が 2 度目に現れたなら、送られたのはラベルの
  // 文字列ではなく制御列である。見るのは端末の行だけ——section はキーバーを
  // 含むので、そこを見るとボタンのラベルに一致してしまう。
  const rows = page.locator(".xterm-rows");
  await page.locator(".xterm-helper-textarea").focus();
  // 印は 1 行に収まる長さでなければならない。**この幅の端末は 20 桁ほどしか
  // 無く**、跨いだ文字列は行の継ぎ目で切れて、どんな部分一致にも当たらない。
  await page.keyboard.type(": zzq");
  await page.keyboard.press("Enter");
  await expect(rows).toContainText("zzq", { timeout: 20_000 });

  await keys.getByRole("button", { name: "↑", exact: true }).click();
  await expect
    .poll(async () => (await rows.innerText()).split("zzq").length - 1, {
      timeout: 20_000,
    })
    .toBeGreaterThanOrEqual(2);
});
