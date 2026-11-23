import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { drawnRowFont, screenRect, terminalKeyboard } from "./support/terminal";

const fontSizeOf = async (page: import("@playwright/test").Page) => Number.parseFloat((await drawnRowFont(page)).fontSize);

test("the wheel turned with Ctrl over a terminal changes the size of its text and not the page's, without saving it", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();
  const before = await fontSizeOf(page);
  const saves: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET" && /\/api\/v1\/(settings|metadata)/u.test(new URL(request.url()).pathname)) saves.push(request.url());
  });
  const zoomBefore = await page.evaluate(() => window.visualViewport?.scale ?? 1);

  const screen = await screenRect(page);
  await page.mouse.move(screen.x + screen.width / 2, screen.y + screen.height / 2);
  await page.keyboard.down("Control");
  await page.mouse.wheel(0, -100);
  await page.mouse.wheel(0, -100);
  await page.keyboard.up("Control");

  await expect.poll(() => fontSizeOf(page)).toBe(before + 2);
  expect(await page.evaluate(() => window.visualViewport?.scale ?? 1)).toBe(zoomBefore);

  // The size is this view's: nothing of the settings is saved.
  expect(saves).toEqual([]);
});

test("Ctrl+= and Ctrl+- in a terminal step the size of its text, and Ctrl+0 brings back the settings' size", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();
  const before = await fontSizeOf(page);
  const zoomBefore = await page.evaluate(() => window.visualViewport?.scale ?? 1);
  await terminalKeyboard(page).focus();

  await page.keyboard.press("Control+Equal");
  await page.keyboard.press("Control+Equal");
  await expect.poll(() => fontSizeOf(page)).toBe(before + 2);
  await page.keyboard.press("Control+Minus");
  await expect.poll(() => fontSizeOf(page)).toBe(before + 1);
  await page.keyboard.press("Control+Digit0");
  await expect.poll(() => fontSizeOf(page)).toBe(before);
  expect(await page.evaluate(() => window.visualViewport?.scale ?? 1)).toBe(zoomBefore);
});

test("changing the size of a terminal's text shows the size and a reset to the settings' size, which leaves the keys with the terminal", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openLocalShell(page);
  const region = page.getByRole("region", { name: /^Terminal for / });
  await expect(region).toBeVisible();
  const before = await fontSizeOf(page);
  // Nothing is shown for the size the terminal opens at.
  await expect(region.getByRole("button", { name: "Reset" })).toHaveCount(0);
  await terminalKeyboard(page).focus();

  await page.keyboard.press("Control+Equal");
  await page.keyboard.press("Control+Equal");
  const tip = region.getByRole("status").filter({ hasText: `${before + 2}px` });
  await expect(tip).toBeVisible();

  await tip.getByRole("button", { name: "Reset" }).click();
  await expect.poll(() => fontSizeOf(page)).toBe(before);
  // At the settings' size there is nothing to go back to.
  await expect(region.getByRole("status").filter({ hasText: `${before}px` })).toBeVisible();
  await expect(region.getByRole("button", { name: "Reset" })).toHaveCount(0);
  await expect(terminalKeyboard(page)).toBeFocused();
  // The tip stays while the pointer is on it, and leaves by itself once the pointer has left.
  const screen = await screenRect(page);
  await page.mouse.move(screen.x + screen.width / 2, screen.y + screen.height / 2);
  await expect(region.getByRole("status").filter({ hasText: `${before}px` })).toHaveCount(0, { timeout: 10_000 });
});
