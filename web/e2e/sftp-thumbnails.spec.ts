import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import AxeBuilder from "@axe-core/playwright";
import type { Locator, Page } from "@playwright/test";
import { expect, openApplication, openSection, openSettingsPage, test } from "./support/environment";
import { samplePNG } from "./support/images";
import { readMachineState } from "./support/settings";
import { chooseEntryOnPhone, openLocalSFTPDirectory } from "./support/sftp";

const photoCount = 60;

function photoName(index: number): string {
  return `photo-${String(index).padStart(3, "0")}.png`;
}

async function folderOfPhotos(home: string): Promise<string> {
  const directory = join(home, "photos");
  await mkdir(join(directory, "album"), { recursive: true });
  await Promise.all(Array.from({ length: photoCount }, (_, index) => writeFile(join(directory, photoName(index)), samplePNG())));
  await writeFile(join(directory, "notes.txt"), "not a picture");
  return directory;
}

// asksForThumbnails counts the thumbnails the page asks the engine for.
function asksForThumbnails(page: Page): string[] {
  const asked: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname.endsWith("/thumbnail")) asked.push(url.searchParams.get("path") ?? "");
  });
  return asked;
}

function fileList(page: Page): Locator {
  return page.getByTestId("sftp-file-list");
}

function entry(page: Page, name: string): Locator {
  return fileList(page).getByRole("button", { name, exact: true });
}

test("draws a thumbnail for the rows in view, without moving them, and only asks for those", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  const directory = await folderOfPhotos(installation.home);
  const asked = asksForThumbnails(page);
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory });

  const first = entry(page, photoName(0));
  await expect(first).toBeVisible();
  const rowBefore = await first.locator("xpath=ancestor::tr").boundingBox();
  await expect(first.locator("img")).toBeVisible();
  const rowAfter = await first.locator("xpath=ancestor::tr").boundingBox();
  expect(rowAfter?.height).toBe(rowBefore?.height);

  await expect.poll(() => asked.length).toBeGreaterThan(0);
  await page.waitForTimeout(500);
  const askedAtTheTop = asked.length;
  expect(askedAtTheTop).toBeLessThan(photoCount / 2);
  expect(asked).not.toContain(join(directory, photoName(photoCount - 1)));

  // Jumping to the end passes over the middle of the folder without asking for it.
  await fileList(page).evaluate((list) => { list.scrollTop = list.scrollHeight; });
  await expect(entry(page, photoName(photoCount - 1)).locator("img")).toBeVisible();
  await page.waitForTimeout(500);
  expect(asked.length).toBeLessThan(photoCount);
  expect(asked).not.toContain(join(directory, photoName(Math.floor(photoCount / 2))));
});

test("stops asking for a row that leaves the view", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  const directory = await folderOfPhotos(installation.home);
  // The answers never come, so the rows at the top are still being made when the list moves on.
  const held: string[] = [];
  await page.route("**/api/v1/sftp/local/thumbnail**", (route) => {
    held.push(route.request().url());
    return new Promise<void>(() => undefined);
  });
  const stopped: string[] = [];
  page.on("requestfailed", (request) => {
    if (request.url().includes("/thumbnail")) stopped.push(new URL(request.url()).searchParams.get("path") ?? "");
  });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory });
  await expect.poll(() => held.length).toBe(2);

  await fileList(page).evaluate((list) => { list.scrollTop = list.scrollHeight; });

  await expect.poll(() => stopped.length).toBeGreaterThanOrEqual(2);
  expect(stopped[0]).toContain("photo-");
  await expect.poll(() => held.length).toBeGreaterThan(2);
});

test("keeps the icon of a file with no picture, and says nothing about it", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  const directory = join(installation.home, "mixed");
  await mkdir(directory);
  await writeFile(join(directory, "real.png"), samplePNG());
  await writeFile(join(directory, "drawing.svg"), '<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>');
  await writeFile(join(directory, "script-in-disguise.png"), '<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>');
  await writeFile(join(directory, "broken.jpg"), Buffer.from([0xff, 0xd8, 0xff, 0xe0, 0, 16, 1, 2, 3]));
  const asked = asksForThumbnails(page);
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory });

  await expect(entry(page, "real.png").locator("img")).toBeVisible();
  await expect.poll(() => asked.length).toBe(3);
  await page.waitForTimeout(300);
  await expect(entry(page, "drawing.svg").locator("img")).toHaveCount(0);
  await expect(entry(page, "script-in-disguise.png").locator("img")).toHaveCount(0);
  await expect(entry(page, "broken.jpg").locator("img")).toHaveCount(0);
  expect(asked.some((path) => path.endsWith(".svg"))).toBe(false);
  await expect(page.getByRole("alert")).toHaveCount(0);
});

test("a folder without pictures keeps its small icons and asks for nothing", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  const directory = join(installation.home, "documents");
  await mkdir(directory);
  await writeFile(join(directory, "notes.txt"), "text");
  const asked = asksForThumbnails(page);
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory });
  const icon = entry(page, "notes.txt").locator("svg");
  await expect(icon).toBeVisible();
  expect((await icon.boundingBox())?.width).toBe(15);
  await page.waitForTimeout(300);
  expect(asked).toEqual([]);
});

test("the grid selects, opens and takes the keyboard like the rows, and is kept with the tab", async ({ page, installation }) => {
  test.setTimeout(60_000);
  await page.setViewportSize({ width: 1280, height: 720 });
  const directory = await folderOfPhotos(installation.home);
  await writeFile(join(directory, "album", "inside.txt"), "inside");
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory });
  await expect(page.getByRole("table")).toBeVisible();

  await page.getByRole("button", { name: "Grid", exact: true }).click();
  await expect(page.getByRole("table")).toHaveCount(0);
  await expect(entry(page, photoName(0)).locator("img")).toBeVisible();

  // A click selects, as a row of the table does.
  const photo = entry(page, photoName(0));
  await photo.click();
  await expect(photo).toHaveAttribute("aria-pressed", "true");

  // The arrow keys go to the neighbour on each side, and down keeps the column.
  await page.keyboard.press("ArrowRight");
  const next = entry(page, photoName(1));
  await expect(next).toBeFocused();
  await expect(next).toHaveAttribute("aria-pressed", "true");
  const columns = await fileList(page).getByRole("list").evaluate((grid) => getComputedStyle(grid).gridTemplateColumns.split(" ").length);
  expect(columns).toBeGreaterThan(2);
  await page.keyboard.press("ArrowDown");
  // The cells run: "..", album, then the photos.
  await expect(entry(page, photoName(1 + columns))).toBeFocused();

  // A right click offers what the row's menu does.
  await entry(page, photoName(2)).click({ button: "right" });
  await expect(page.getByRole("menu", { name: `Actions for ${photoName(2)}`, exact: true })).toBeVisible();
  await page.keyboard.press("Escape");

  // A double click opens a folder, and the parent is the first cell.
  await entry(page, "album").dblclick();
  await expect(entry(page, "inside.txt")).toBeVisible();
  await page.getByRole("button", { name: "Parent directory", exact: true }).click();
  await expect(entry(page, photoName(0))).toBeVisible();

  // The order of the rows can still be changed.
  await fileList(page).getByRole("button", { name: /^Name/u }).click();
  const rowKeys = fileList(page).locator("[data-row-key]");
  await expect(rowKeys.first()).toHaveAttribute("data-row-key", "..");
  await expect(rowKeys.nth(1)).toHaveAttribute("data-row-key", new RegExp(`${photoName(photoCount - 1)}$`, "u"));

  const result = await new AxeBuilder({ page }).include('[data-testid="sftp-file-list"]').withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(result.violations.filter((violation) => violation.impact === "serious" || violation.impact === "critical")).toEqual([]);

  // The view stays with the tab, on this machine.
  await expect.poll(async () => readMachineState(page, "sftpPanes")).toContain('"view":"grid"');
  await page.reload();
  await expect(page.getByRole("button", { name: "Grid", exact: true })).toHaveAttribute("aria-pressed", "true");
});

test("Settings > SFTP turns the thumbnails off, and the list asks for nothing", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  const directory = await folderOfPhotos(installation.home);
  const asked = asksForThumbnails(page);
  await openApplication(page, installation);
  await openSettingsPage(page, "SFTP");
  const thumbnails = page.getByRole("checkbox", { name: "Show thumbnails of images" });
  await expect(thumbnails).toBeChecked();
  await thumbnails.click();
  await expect(thumbnails).not.toBeChecked();

  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory });
  await expect(entry(page, photoName(0))).toBeVisible();
  await page.waitForTimeout(500);
  expect(asked).toEqual([]);
  await expect(entry(page, photoName(0)).locator("img")).toHaveCount(0);
});

test.describe("on a phone", () => {
  test.use({ hasTouch: true, isMobile: true, viewport: { width: 390, height: 844 } });

  test("keeps the two-line row at its height, and a grid cell opens with a tap and is chosen with a long press", async ({ page, installation }) => {
    const directory = await folderOfPhotos(installation.home);
    await openApplication(page, installation);
    await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "SFTP", exact: true }).tap();
    await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory });

    const row = entry(page, photoName(0));
    await expect(row.locator("img")).toBeVisible();
    await expect(row).toContainText(photoName(0));
    await expect(row.locator("time")).toBeVisible();
    // A row without a picture in the same list has the same height as one with it.
    const withPicture = await row.locator("xpath=ancestor::li").boundingBox();
    const withoutPicture = await entry(page, "album").locator("xpath=ancestor::li").boundingBox();
    expect(Math.abs((withPicture?.height ?? 0) - (withoutPicture?.height ?? 0))).toBeLessThanOrEqual(1);
    expect(withPicture?.height).toBeLessThanOrEqual(51);

    await page.getByRole("button", { name: "Grid", exact: true }).tap();
    const cell = entry(page, photoName(0));
    await expect(cell.locator("img")).toBeVisible();
    const box = await cell.locator("img").boundingBox();
    expect(box?.width).toBe(box?.height);

    // A tap opens the file.
    await cell.tap();
    const preview = page.getByRole("dialog").first();
    await expect(preview).toBeVisible();
    await expect(preview).toContainText(photoName(0));
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toHaveCount(0);

    // A long press starts choosing, and a tap then chooses another rather than opening it.
    const pane = page.getByRole("tabpanel");
    await chooseEntryOnPhone(pane, photoName(0));
    await entry(page, photoName(1)).tap();
    await expect(entry(page, photoName(1))).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });
});
