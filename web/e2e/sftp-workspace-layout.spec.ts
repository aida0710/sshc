import { join } from "node:path";
import type { Locator, Page } from "@playwright/test";
import type { RemoteEntry } from "../src/sftp/api";
import { changeDisplayLanguage, expect, openApplication, openSection, test } from "./support/environment";
import { chooseEntryOnPhone, connectSFTPHost, openLocalSFTPDirectory, openSecondSFTPPane, sftpTabStrip } from "./support/sftp";

// Mouse and touch controls must align without sacrificing phone tap targets.
for (const hasTouch of [false, true]) {
  test.describe(hasTouch ? "touch controls" : "mouse controls", () => {
    test.use({ hasTouch });

    test("aligns local and remote controls and keeps the file list usable", async ({ page, installation }) => {
      await page.route("**/api/v1/sftp/bastion/entries**", (route) => route.fulfill({ json: {
        path: "/srv", entries: [{ name: "remote.txt", path: "/srv/remote.txt", type: "file", size: 12, mode: "0644", modifiedAt: "2026-10-10T00:00:00Z", revision: "remote-file" }],
      } }));
      await installation.write("layout/alpha.txt", "alpha");
      await installation.write("layout/beta.txt", "beta");
      await installation.write("layout/notes.txt", "notes");
      await page.setViewportSize({ width: 1440, height: 900 });
      await openApplication(page, installation);
      await openSection(page, "SFTP");
      await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory: join(installation.home, ".ssh/layout") });
      await openSecondSFTPPane(page, "New tab", "bastion");
      const remote = page.getByRole("tabpanel").nth(1);
      const local = page.getByRole("tabpanel").first();
      const localFilter = local.getByRole("searchbox", { name: "Filter entries" });
      const remoteFilter = remote.getByRole("searchbox", { name: "Filter entries" });
      await expect(remote.getByRole("button", { name: "remote.txt", exact: true })).toBeVisible();
      await expect.poll(async () => Math.abs((await localFilter.boundingBox())!.y - (await remoteFilter.boundingBox())!.y)).toBeLessThanOrEqual(1);
      await expect.poll(async () => Math.abs((await local.getByTestId("sftp-file-list").boundingBox())!.y - (await remote.getByTestId("sftp-file-list").boundingBox())!.y)).toBeLessThanOrEqual(1);
      await expect(local.getByRole("button", { name: "Host", exact: true })).toHaveCount(0);
      await expect(remote.getByRole("button", { name: "Host", exact: true })).toHaveCount(0);
      await expect(page.getByRole("combobox", { name: "Search mode" })).toHaveCount(0);
      // Split panes prioritize names and use the same columns; full metadata is in Details.
      await expect(local.getByRole("columnheader")).toHaveCount(5);
      await expect(remote.getByRole("columnheader")).toHaveCount(5);
      for (const pane of [local, remote]) {
        await expect.poll(() => pane.getByTestId("sftp-file-list").evaluate((element) => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
      }
      for (const group of await page.locator("[data-sftp-pane-tabs]").all()) {
        const bounds = (await group.boundingBox())!;
        const selected = (await group.getByRole("tab", { selected: true }).boundingBox())!;
        expect(selected.y + selected.height).toBeLessThanOrEqual(bounds.y + bounds.height);
      }
      await remote.getByRole("button", { name: "Search mode", exact: true }).click();
      const options = page.getByRole("dialog", { name: "Search mode", exact: true });
      await options.getByRole("combobox", { name: "Search mode" }).selectOption("content");
      await options.getByRole("button", { name: "Close", exact: true }).click();
      await expect(remote.getByRole("searchbox", { name: "Text to find in files" })).toBeVisible();

      for (const viewport of hasTouch ? [{ width: 390, height: 640 }, { width: 844, height: 390 }] : []) {
        await page.setViewportSize(viewport);
        await sftpTabStrip(page, "File tabs").getByRole("tab", { name: "Local:layout", exact: true }).click();
        const pane = page.getByRole("tabpanel");
        await expect(pane).toHaveCount(1);
        const list = pane.getByTestId("sftp-file-list");
        // Two complete file rows must fit even in landscape.
        await expect.poll(async () => (await list.boundingBox())!.height).toBeGreaterThanOrEqual(112);
        await expect.poll(async () => {
          const bounds = (await list.boundingBox())!;
          const rows = await Promise.all(["alpha.txt", "beta.txt"].map((name) => list.getByRole("button", { name, exact: true }).boundingBox()));
          return rows.every((row) => row !== null && row.y >= bounds.y && row.y + row.height <= bounds.y + bounds.height);
        }).toBe(true);
        const before = (await list.boundingBox())!.height;
        await chooseEntryOnPhone(pane, "alpha.txt");
        await expect(pane.getByRole("button", { name: "Actions for alpha.txt", exact: true })).toBeVisible();
        await expect.poll(async () => Math.abs((await list.boundingBox())!.height - before)).toBeLessThanOrEqual(1);
        await pane.getByRole("button", { name: "Clear selection", exact: true }).click();
      }
    });
  });
}

// 左右に並べたペインが狭くても、列を消さず名前も潰さない。入りきらない分は一覧を横にスクロールする。
test("keeps file names readable in narrow side-by-side panes and scrolls the list sideways instead", async ({ page, installation }) => {
  await installation.write("narrow/report-001.csv", "a,b");
  const directory = join(installation.home, ".ssh/narrow");
  await page.setViewportSize({ width: 1024, height: 768 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory });
  await page.getByRole("button", { name: "Side by side", exact: true }).click();
  const left = page.getByRole("tabpanel").first();
  const name = left.getByRole("button", { name: "report-001.csv", exact: true }).locator("[title]").first();
  await expect(name).toBeVisible();
  await expect.poll(() => name.evaluate((element) => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(0);
  await expect.poll(() => left.getByTestId("sftp-file-list").evaluate((element) => element.scrollWidth - element.clientWidth)).toBeGreaterThan(0);
  await expect(left.getByRole("columnheader")).toHaveCount(5);
});

const narrowPaneEntries: RemoteEntry[] = [
  { name: "current", path: "/work/current", type: "symlink", size: 0, mode: "0777", modifiedAt: "2026-10-07T06:00:00Z", revision: "a", linkTarget: "old", targetType: "directory" },
  { name: "mirage-backup.tar.gz", path: "/work/mirage-backup.tar.gz", type: "file", size: 4096, mode: "0644", modifiedAt: "2026-10-07T06:00:00Z", revision: "b" },
  { name: "notes.txt", path: "/work/notes.txt", type: "file", size: 2048, mode: "0644", modifiedAt: "2026-10-07T06:00:00Z", revision: "c" },
];

// 2つ目のペインも bastion にして、日本語の画面で左右に並べる。
async function openRemotePanesInJapanese(page: Page) {
  await page.route("**/api/v1/sftp/bastion/entries**", (route) => route.fulfill({ json: { path: "/work", entries: narrowPaneEntries } }));
  await openSection(page, "SFTP");
  await changeDisplayLanguage(page, "ja");
  await connectSFTPHost(page, "bastion");
  await openSecondSFTPPane(page, "新しいタブ", "bastion");
}

// 文字が要素の幅からはみ出して切れているか。
function isCut(element: Locator) {
  return element.evaluate((cell) => cell.scrollWidth > cell.clientWidth);
}

// 狭い列で種別の文字が切れても、ポインタを当てれば全体が読める。切れないものはそのまま見せる。
test("shows the whole type name on hover when the narrow column cuts it in Japanese", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1024, height: 768 });
  await openApplication(page, installation);
  await openRemotePanesInJapanese(page);
  const left = page.getByRole("tabpanel").first();
  const symlinkType = left.getByRole("row", { name: /current/u }).getByRole("cell", { name: "シンボリックリンク", exact: true });
  const archiveType = left.getByRole("row", { name: /mirage-backup/u }).getByRole("cell", { name: "圧縮ファイル", exact: true });
  await expect(symlinkType).toBeVisible();
  await expect.poll(() => isCut(symlinkType)).toBe(true);
  await expect(symlinkType).toHaveAttribute("title", "シンボリックリンク");
  await expect.poll(() => isCut(archiveType)).toBe(false);
});

// 絞り込みの欄の幅が、プレースホルダーを切らずに出せるだけ残っているか。
function showsWholePlaceholder(filter: Locator) {
  return filter.evaluate((input: HTMLInputElement) => {
    const style = getComputedStyle(input);
    const context = document.createElement("canvas").getContext("2d")!;
    context.font = `${style.fontSize} ${style.fontFamily}`;
    const room = input.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight);
    return context.measureText(input.placeholder).width <= room;
  });
}

// 仕切りを端まで動かしても、そのあと窓を狭めても、ペインの操作の部品はペインの中に収まり、絞り込みの欄も入力できる幅を保つ。
// 接続先のペインは［検索対象］と［このフォルダ配下を検索する］の分だけ絞り込みの行が長く、日本語では［リスト｜グリッド］も長い。
const narrowedPaneCases = [
  { language: "en", connection: "Local", separator: "Resize the panes", filter: "Filter entries", controls: [
    { role: "group", name: "How the list is shown" }, { role: "button", name: "Refresh directory" },
  ] },
  { language: "ja", connection: "bastion", separator: "ペインの幅を変更", filter: "項目を絞り込み", controls: [
    { role: "group", name: "一覧の表示形式" }, { role: "button", name: "フォルダを更新" },
    { role: "button", name: "検索対象" }, { role: "button", name: "このフォルダ配下を検索する" },
  ] },
] as const;

for (const { language, connection, separator, filter, controls } of narrowedPaneCases) {
  test(`keeps a side-by-side pane wide enough for its controls when the divider or the window narrows it (${language}, ${connection})`, async ({ page, installation }) => {
    await installation.write("narrow/notes.txt", "notes");
    await page.setViewportSize({ width: 1440, height: 800 });
    await openApplication(page, installation);
    if (connection === "bastion") {
      await openRemotePanesInJapanese(page);
    } else {
      await openSection(page, "SFTP");
      await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory: join(installation.home, ".ssh/narrow") });
      await page.getByRole("button", { name: "Side by side", exact: true }).click();
    }
    const leftPane = page.locator("[data-sftp-pane]").first();
    await expect(leftPane.getByRole("button", { name: "notes.txt", exact: true })).toBeVisible();
    const controlsInside = async () => {
      const pane = (await leftPane.boundingBox())!;
      const boxes = await Promise.all([
        leftPane.getByRole("searchbox", { name: filter, exact: true }).boundingBox(),
        ...controls.map((control) => leftPane.getByRole(control.role, { name: control.name, exact: true }).boundingBox()),
      ]);
      return boxes.every((box) => box !== null && box.x >= pane.x && box.x + box.width <= pane.x + pane.width);
    };
    const filterField = leftPane.getByRole("searchbox", { name: filter, exact: true });
    const handle = page.getByRole("separator", { name: separator, exact: true });
    await handle.focus();
    for (let step = 0; step < 12; step++) await page.keyboard.press("ArrowLeft");
    await expect.poll(controlsInside).toBe(true);
    await expect.poll(() => showsWholePlaceholder(filterField)).toBe(true);
    await page.setViewportSize({ width: 1024, height: 800 });
    await expect.poll(controlsInside).toBe(true);
    await expect.poll(() => showsWholePlaceholder(filterField)).toBe(true);
  });
}

// 窓を狭めてペインが最小の幅まで広がったら、仕切りは見えている位置を値にし、キーはそこから動かす。
// 動かせないキーでは保存した比率を変えないので、窓を広げ直すと元の幅に戻る。
test("moves the divider from where a narrowed window shows it and keeps the split a key cannot move", async ({ page, installation }) => {
  await installation.write("narrow/notes.txt", "notes");
  await page.setViewportSize({ width: 1440, height: 800 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory: join(installation.home, ".ssh/narrow") });
  await page.getByRole("button", { name: "Side by side", exact: true }).click();
  const leftPane = page.locator("[data-sftp-pane]").first();
  const leftWidth = async () => (await leftPane.boundingBox())!.width;
  const handle = page.getByRole("separator", { name: "Resize the panes", exact: true });
  // 仕切りの値と、見えている左のペインの割合（2つのペインを並べた行に対する %）の差。
  const valueOffShown = () => leftPane.evaluate((pane) => {
    const row = pane.parentElement!.getBoundingClientRect().width;
    const value = Number(pane.parentElement!.querySelector('[role="separator"]')!.getAttribute("aria-valuenow"));
    return Math.abs(value - pane.getBoundingClientRect().width / row * 100);
  });
  await expect(leftPane.getByRole("button", { name: "notes.txt", exact: true })).toBeVisible();
  const evenWidth = await leftWidth();
  await handle.focus();
  for (let step = 0; step < 12; step++) await page.keyboard.press("ArrowLeft");
  await expect.poll(leftWidth).toBeLessThan(evenWidth);
  const narrowestAtWide = await leftWidth();

  await page.setViewportSize({ width: 1024, height: 800 });
  await expect.poll(leftWidth).toBeLessThanOrEqual(narrowestAtWide);
  await expect.poll(valueOffShown).toBeLessThanOrEqual(1);
  await page.keyboard.press("ArrowLeft");
  await page.setViewportSize({ width: 1440, height: 800 });
  await expect.poll(async () => Math.abs(await leftWidth() - narrowestAtWide)).toBeLessThanOrEqual(1);

  await page.setViewportSize({ width: 1024, height: 800 });
  await expect.poll(valueOffShown).toBeLessThanOrEqual(1);
  const narrowestAtNarrow = await leftWidth();
  await page.keyboard.press("ArrowRight");
  await expect.poll(leftWidth).toBeGreaterThan(narrowestAtNarrow + 1);
});

test("opens a local shell in the displayed directory from the SFTP terminal button", async ({ page, installation }) => {
  await installation.write("shell-start/notes.txt", "notes");
  const directory = join(installation.home, ".ssh/shell-start");
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  const pane = page.getByRole("tabpanel");
  await openLocalSFTPDirectory({ page, pane, directory });
  const creation = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/terminal/sessions" && response.request().method() === "POST");
  await pane.getByRole("button", { name: "Open Terminal here", exact: true }).click();
  const response = await creation;
  expect(response.ok()).toBe(true);
  expect(response.request().postDataJSON()).toMatchObject({ kind: "shell", cwd: directory });
  expect(response.request().postDataJSON()).not.toHaveProperty("alias");
  await expect(page).toHaveURL(/\/terminal$/);
});
