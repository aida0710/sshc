import type { Locator, Page } from "@playwright/test";
import { expect, openApplication, test } from "./support/environment";
import { saveBar, saveButton, saveSettings } from "./support/saveBar";

async function openSettings(page: Page, path: string) {
  await page.goto(new URL(`/settings/${path}`, page.url()).href);
}

// expectAbove は、element が、その下の端に重なる below に隠れていないことを確かめる。
async function expectAbove(element: Locator, below: Locator) {
  const top = await element.boundingBox();
  const bottom = await below.boundingBox();
  if (top === null || bottom === null) throw new Error("要素が画面に無い");
  expect(top.y + top.height).toBeLessThanOrEqual(bottom.y + 1);
}

async function endOfScroll(page: Page) {
  await saveBar(page).evaluate((bar) => {
    let scroller = bar.parentElement;
    while (scroller !== null && !(/auto|scroll/.test(getComputedStyle(scroller).overflowY) && scroller.scrollHeight > scroller.clientHeight)) scroller = scroller.parentElement;
    scroller?.scrollTo({ top: scroller.scrollHeight });
  });
}

test("a theme chosen in Settings applies once it is saved, and the bar stays in sight of a long page", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1280, height: 600 });
  await openApplication(page, installation);
  await openSettings(page, "appearance");
  const root = page.locator("html");
  const before = await root.getAttribute("data-theme");
  await expect(saveBar(page)).toHaveCount(0);

  await page.getByRole("radiogroup", { name: "Theme" }).locator("label").filter({ hasText: /^Light$/ }).click();
  await expect(saveButton(page)).toBeInViewport({ ratio: 1 });
  expect(await root.getAttribute("data-theme")).toBe(before);
  // The last row can be scrolled clear of the bar.
  await endOfScroll(page);
  await expectAbove(page.getByRole("group", { name: "Reduce motion" }), saveBar(page));

  await saveSettings(page);
  await expect(root).toHaveAttribute("data-theme", "light");
});

test("leaving a settings page with unsaved changes asks first, by the list, by another page and by going back", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSettings(page, "general");
  await openSettings(page, "sftp");
  const speed = page.getByRole("spinbutton", { name: "Speed limit" });
  await speed.fill("128");
  const dialog = page.getByRole("dialog");

  await page.getByRole("navigation", { name: "Settings" }).getByRole("link", { name: "General" }).click();
  await dialog.getByRole("button", { name: "Keep editing" }).click();
  await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "Connections", exact: true }).click();
  await dialog.getByRole("button", { name: "Keep editing" }).click();
  await expect(speed).toHaveValue("128");

  await page.getByRole("navigation", { name: "Settings" }).getByRole("link", { name: "General" }).click();
  await dialog.getByRole("button", { name: "Discard", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/general$/);
  await page.getByRole("navigation", { name: "Settings" }).getByRole("link", { name: "SFTP" }).click();
  await expect(speed).toHaveValue("0");
});

test.describe("on a phone", () => {
  test.use({ hasTouch: true, isMobile: true, viewport: { width: 390, height: 640 } });

  test("the bar sits above the bottom bar, follows a typed number, and hides neither the field nor the page's end", async ({ page, installation }) => {
    await openApplication(page, installation);
    await openSettings(page, "sftp");
    const chunk = page.getByRole("spinbutton", { name: "Chunk" });
    await chunk.scrollIntoViewIfNeeded();
    await chunk.tap();
    await chunk.fill("64");

    await expectAbove(saveButton(page), page.getByRole("navigation", { name: "Quick navigation" }));
    await expectAbove(chunk, saveBar(page));

    await chunk.fill("9999");
    await expect(saveButton(page)).toBeDisabled();
    await expectAbove(page.getByRole("main").getByText(/^Chunk: enter a whole number/), saveBar(page));
    await chunk.fill("64");
    await endOfScroll(page);
    await expectAbove(page.getByText("Names match at any depth", { exact: false }), saveBar(page));

    await page.getByRole("link", { name: "Connections", exact: true }).tap();
    await page.getByRole("dialog").getByRole("button", { name: "Keep editing" }).tap();
    await saveSettings(page);
    await expect(page.getByText("Saved your changes.", { exact: true })).toBeVisible();
  });

  test("the connection editor's bar stays above the bottom bar while the form scrolls", async ({ page, installation }) => {
    await openApplication(page, installation);
    await page.goto(new URL("/connections", page.url()).href);
    await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion", exact: true }).tap();
    await page.getByLabel("Port", { exact: true }).fill("2200");

    await expect(saveButton(page)).toBeInViewport({ ratio: 1 });
    await expectAbove(saveButton(page), page.getByRole("navigation", { name: "Quick navigation" }));
    await saveBar(page).getByRole("button", { name: "Discard changes" }).tap();
    await expect(page.getByLabel("Port", { exact: true })).toHaveValue("2222");
    await expect(saveBar(page)).toHaveCount(0);
  });
});
