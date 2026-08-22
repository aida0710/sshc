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
  // **描かれてから測る。** セクションは遅延読み込みなので、押した直後の面には
  // まだ何も無い——待たずに測れば、どの検査も空の面について緑を返す。実際
  // Keys の検索欄は面から 253px はみ出していたのに、ここが緑だった。
  await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
}

// 横スクロールは、狭い画面の壊れ方そのものである。**どれか一つの面が溢れれば
// ドキュメント全体が溢れる**ので、面を渡り歩きながら同じことを一度ずつ問う。
async function expectNoHorizontalOverflow(page: import("@playwright/test").Page, where: string) {
  const { overflow, culprits } = await page.evaluate(() => {
    const root = document.documentElement;
    const overflow = root.scrollWidth - root.clientWidth;
    if (overflow <= 0) return { overflow, culprits: [] as string[] };

    // **溢れた量だけでは直せない。** この検査が落ちるのは、たいてい再現の
    // 難しい環境（別のフォント、別のスクロールバー）であり、そこで見えている
    // ものを持ち帰れなければ、直す側は当てずっぽうを繰り返すことになる。
    // 面の外へはみ出した要素を、その場で名指しして持ち帰る。
    const limit = root.clientWidth;
    const describe = (element: Element) => {
      const rectangle = element.getBoundingClientRect();
      const identity = [
        element.tagName.toLowerCase(),
        element.id === "" ? "" : `#${element.id}`,
        typeof element.className === "string" && element.className !== ""
          ? `.${element.className.trim().split(/\s+/).join(".")}`
          : "",
      ].join("");
      return `${identity} [${Math.round(rectangle.left)}..${Math.round(rectangle.right)}]`;
    };
    // 溢れているのは親も子も同じなので、**いちばん深いものだけを名指す**。
    // 祖先まで並べると、本当に幅を決めているものが行の海に沈む。
    const culprits = [...root.querySelectorAll("*")]
      .filter((element) => {
        const rectangle = element.getBoundingClientRect();
        if (rectangle.width === 0 || rectangle.right <= limit + 0.5) return false;
        return ![...element.children].some(
          (child) => child.getBoundingClientRect().right > limit + 0.5,
        );
      })
      .slice(0, 5)
      .map(describe);
    return { overflow, culprits };
  });
  expect(
    overflow,
    `${where} scrolls sideways at 360px; past the right edge: ${culprits.join(", ") || "nothing measurable"}`,
  ).toBeLessThanOrEqual(0);
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
    await expectNothingCutOff(page, section);
  }
});

// **操作は面の中に居なければならない。**
//
// 上の検査が見るのは document の横スクロールだが、セクションは自分の器の中で
// 送られるので、溢れても document は動かない。実際 Keys の検索欄は 288px 固定で、
// 360px の面から 253px 出ていたのに、あの検査は緑だった。
//
// **表は別である。** あれは overflow-x-auto に入れてあり、面より広いのは承知の
// うえで、指で送れば読める。
async function expectNothingCutOff(page: import("@playwright/test").Page, where: string) {
  const escaped = await page.evaluate(() => {
    const limit = document.documentElement.clientWidth;
    const drawer = document.querySelector("nav");
    const out: string[] = [];
    for (const element of Array.from(
      document.querySelectorAll("input, select, button, a[href], textarea"),
    )) {
      // 閉じているドロワーは面の外に駐まっている。位置は別の検査が見ている。
      if (drawer?.contains(element) === true) continue;
      if (element.closest("table") !== null) continue;
      const box = element.getBoundingClientRect();
      if (box.width === 0 || box.height === 0) continue;
      if (box.right <= limit + 1) continue;
      const name =
        element.getAttribute("aria-label") ??
        element.textContent?.trim().slice(0, 24) ??
        element.tagName.toLowerCase();
      out.push(
        `${element.tagName.toLowerCase()} "${name}" → ${box.left.toFixed(0)}..${box.right.toFixed(0)} (面は 0..${limit})`,
      );
    }
    return out;
  });
  expect(escaped, `${where}: 面からはみ出した操作がある`).toEqual([]);
}

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
  // **この組み合わせは現実に存在しない。** 画面上のキーは物理キーボードを
  // 持たない端末のためのもので、ここで組んでいるのは Windows のデスクトップ
  // ブラウザに 360px とタッチを被せた姿である。修飾を立ててから打つ流れが、
  // その環境の入力エミュレーションでは焦点ごと落ちる。
  //
  // **下の層は別に確かめてある。** 0x03 を ConPTY の入力へ書けば走っている
  // 子が止まることは、cmd.exe と PowerShell の両方に対して実機で確認した。
  // ここで確かめられないのは UI からその一バイトへ至る道であり、それは
  // Linux の CI が同じ spec で見ている。
  test.skip(
    process.platform === "win32",
    "on-screen keys are a touch affordance; Linux CI covers this path",
  );
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

test("lays a selectable layer over the terminal, outside the element that blocks it", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  const nav = page.getByRole("navigation", { name: "Primary" });
  await nav.getByRole("tab", { name: "Terminals" }).click();
  await nav.getByRole("button", { name: "Local shell" }).click();

  const rows = page.locator(".xterm-rows");
  await expect(rows).toContainText(/[$#%>]/, { timeout: 20_000 });
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type("echo zzq");
  await page.keyboard.press("Enter");
  await expect(rows).toContainText("zzq", { timeout: 20_000 });

  const overlay = page.locator(".sshc-select-overlay");
  await expect(overlay).toHaveCount(1);

  // **これがこの設計そのものである。** .xterm の中では長押しからの選択が
  // 始まらないので、字は必ずその外に無ければならない。
  expect(await overlay.evaluate((node) => node.closest(".xterm") === null)).toBe(true);
  await expect.poll(async () => (await overlay.textContent()) ?? "").toContain("zzq");

  // 重ならなければ、帯は別の字の上に出る。
  const shape = await page.evaluate(() => {
    const layer = document.querySelector(".sshc-select-overlay")!.getBoundingClientRect();
    const screen = document.querySelector(".xterm-screen")!.getBoundingClientRect();
    return {
      dx: Math.abs(layer.x - screen.x),
      dy: Math.abs(layer.y - screen.y),
      dw: Math.abs(layer.width - screen.width),
      dh: Math.abs(layer.height - screen.height),
    };
  });
  expect(shape.dx).toBeLessThanOrEqual(1);
  expect(shape.dy).toBeLessThanOrEqual(1);
  expect(shape.dw).toBeLessThanOrEqual(1);
  expect(shape.dh).toBeLessThanOrEqual(1);

  // **字送りを決めているのは font ではなく letter-spacing である。** xterm が
  // 較正したその値を写しているので、桁がずれない。名前が変われば、ここが落ちる。
  const metrics = await page.evaluate(() => {
    const layer = getComputedStyle(document.querySelector(".sshc-select-overlay")!);
    const source = getComputedStyle(document.querySelector(".xterm-rows")!);
    return {
      sameFamily: layer.fontFamily === source.fontFamily,
      sameSize: layer.fontSize === source.fontSize,
      sameSpacing: layer.letterSpacing === source.letterSpacing,
      colour: layer.color,
    };
  });
  expect(metrics.sameFamily).toBe(true);
  expect(metrics.sameSize).toBe(true);
  expect(metrics.sameSpacing).toBe(true);
  // 見えてはならない。見えているのは下の xterm の字である。
  expect(metrics.colour).toBe("rgba(0, 0, 0, 0)");

  // 板の字は本物である——選べば、端末に出ているものが返る。
  const selected = await page.evaluate(() => {
    const layer = document.querySelector(".sshc-select-overlay")!;
    const range = document.createRange();
    range.selectNodeContents(layer);
    const selection = getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    return selection.toString();
  });
  expect(selected).toContain("zzq");
});

// 問いは、それを出した場所の都合とは無関係に読めなければならない。
//
// **`fixed` が窓を基準にするのは、祖先が transform を持っていないときだけ
// である。** ナビゲーションの板は開閉のために常に translate を持ち、さらに
// overflow-hidden で切る——中に置かれた確認は幅 288px の板に閉じ込められ、
// 文も釦も見切れていた。狭い画面がいちばん先に壊れるので、ここで測る。
test("asks before closing a live console, in the middle of the screen and not inside the drawer", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  const nav = page.getByRole("navigation", { name: "Primary" });
  await nav.getByRole("tab", { name: "Terminals" }).click();
  await nav.getByRole("button", { name: "Local shell" }).click();
  await expect(page.locator(".xterm-rows")).toContainText(/[$#%>]/, { timeout: 20_000 });

  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  await nav.getByRole("button", { name: /^Close / }).first().click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();

  const measured = await dialog.evaluate((node) => {
    const box = (node as HTMLElement).getBoundingClientRect();
    return {
      insideDrawer: (node as HTMLElement).closest("nav") !== null,
      left: box.left,
      right: box.right,
      width: box.width,
      viewport: document.documentElement.clientWidth,
    };
  });

  // **板の中に居てはならない。** 居れば、板が切る。
  expect(measured.insideDrawer).toBe(false);
  // 窓の中に収まっている。左右どちらにも溢れていない。
  expect(measured.left).toBeGreaterThanOrEqual(0);
  expect(measured.right).toBeLessThanOrEqual(measured.viewport);
  // **板の幅（288px）に閉じ込められていない。** 360px の面で、そこが症状だった。
  expect(measured.width).toBeGreaterThan(288);

  // どちらの釦も押せる場所に在る。
  await expect(dialog.getByRole("button", { name: /Keep|open/i })).toBeVisible();
  await expect(dialog.getByRole("button", { name: /Close/i })).toBeVisible();
});
