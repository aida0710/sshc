import { expect, openSection, test, openApplication } from "./support/environment";

// このスイートが動く 1280x720 のビューポートに Connections パネルが収まり
// きらないほど長い設定。これがなければ、以下の検証は何もスクロールしな
// いままヘッダーをスクロールで消してしまうシェルの上でも通ってしまう。
const manyHosts = Array.from(
  { length: 40 },
  (_unused, index) => `Host lab-${String(index).padStart(2, "0")}\n\tHostName 198.51.100.${index + 1}\n`,
).join("\n");

test("keeps the header and the primary navigation still while a panel scrolls", async ({
  page,
  installation,
}) => {
  await installation.write("conf.d/20-lab.conf", manyHosts);
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await expect(
    page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "lab-00" }),
  ).toBeVisible();

  const header = page.getByRole("banner");
  const tree = page.getByRole("navigation", { name: "Connections" });
  const resting = await header.boundingBox();
  expect(resting).not.toBeNull();

  // リストは今や自分自身のペインを持ち、パネル全体を道連れに
  // せず単独でスクロールする。そのスクローラーはツリーが置かれている要素である。
  //
  // 本当に overflow していなければならない。そうでなければ、これ以降のすべての
  // 検証はヘッダーを画面外へスクロールしてしまうシェルの上でも成立してしまう。
  const overflow = await tree.evaluate((element) => {
    const scroller = element.parentElement;
    if (scroller === null) return 0;
    return scroller.scrollHeight - scroller.clientHeight;
  });
  expect(overflow, "the fixture is not tall enough to scroll the list").toBeGreaterThan(0);

  // ドキュメント自体はスクロールしてはならない。これが退行そのものだ。ペー
  // ジレベルのスクロールがヘッダーとセクションボタンを持ち去っていたのであ
  // り、他に何も消費できなければパネル上のホイール操作がそれを生んでいた。
  const documentOverflow = await page.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement;
    return root.scrollHeight - root.clientHeight;
  });
  expect(documentOverflow, "the document scrolls, so the header can leave the viewport").toBe(0);

  const windowOffset = await page.evaluate(() => {
    window.scrollTo(0, 10_000);
    return window.scrollY;
  });
  expect(windowOffset).toBe(0);

  await tree.evaluate((element) => element.parentElement?.scrollTo(0, element.parentElement.scrollHeight));
  expect(await tree.evaluate((element) => element.parentElement?.scrollTop ?? 0)).toBeGreaterThan(0);

  expect(await header.boundingBox()).toEqual(resting);
  await expect(page.getByRole("navigation", { name: "Primary" })).toBeInViewport();
  await expect(page.getByRole("link", { name: "History", exact: true })).toBeInViewport();

  // スクロールされたパネルの下端には、それでも到達できなければならない。ビューポー
  // トに固定されながらパネルをスクロール可能にし忘れたシェルは、それを隠してしまう。
  await expect(
    page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "lab-39" }),
  ).toBeInViewport();
});

test("scrolls the primary navigation on its own when the viewport is short", async ({
  page,
  installation,
}) => {
  await page.setViewportSize({ width: 1280, height: 320 });
  await openApplication(page, installation);
  await openSection(page, "Connections");

  // **スクロールするのはナビゲーションの下半分である。** Start と面のトグルは
  // 固定されているので、溢れるのはセクションの一覧の側であり、ドキュメント
  // 全体ではない。
  const navigation = page.getByRole("navigation", { name: "Primary" });
  const sections = navigation.locator("div.overflow-y-auto");
  const overflow = await sections.evaluate((element) => element.scrollHeight - element.clientHeight);
  expect(overflow, "the section list is not taller than the short viewport").toBeGreaterThan(0);

  // **最後まで送るのは、送りたい相手に頼む。** 以前はここで一度
  // scrollHeight まで飛ばし、そのあとで到達を確かめていた。飛ばした後に
  // 一覧が伸びれば——数字や印は届いてから描かれる——その位置はもう底では
  // なく、History は畳の下へ戻る。実際、その形で CI が落ちた。
  const history = page.getByRole("link", { name: "History", exact: true });

  // **送るのも一度きりにしない。** scrollIntoViewIfNeeded は呼んだ瞬間の
  // 高さで位置を決めるので、そのあとに一覧が伸びれば History はまた畳の下へ
  // 戻る——待つだけでは戻ってこない。**利用者はもう一度送る。** ここも同じ
  // ことをする: 送って、届いたかを見て、届いていなければまた送る。
  //
  // 一度きりの形は Windows の CI で再発した（同じ「viewport ratio 0」を 24 回
  // 見て 10 秒で諦める形）。待ち時間を伸ばしても直らない——**伸びるのは
  // 待っているあいだではなく、送ったあとだからである。**
  await expect(async () => {
    await history.scrollIntoViewIfNeeded();
    await expect(history).toBeInViewport();
  }).toPass();
  // **そして動いたのはナビゲーションの下半分だけである。** ドキュメント全体が
  // 動いたのなら、ヘッダーはここで消えている。
  await expect(page.getByRole("banner")).toBeInViewport();
  expect(await sections.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
});
